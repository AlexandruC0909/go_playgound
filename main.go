package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/AlexandruC0909/playground/templates"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/time/rate"
)

const (
	timeoutSeconds    = 100
	memoryLimit       = 150 * 1024 * 1024
	dockerImage       = "golang:1.22-alpine"
	maxCodeSize       = 1024 * 1024
	maxOutputSize     = 1024 * 1024
	requestsPerHour   = 1000
	requestsPerMinute = 500
	containerName     = "go-playground"
)

type RateLimiter struct {
	visitors map[string]*rate.Limiter
	mu       sync.Mutex
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*rate.Limiter),
	}
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.visitors[ip]
	if !exists {
		// Create both per-minute and per-hour limiters
		limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), 1)
		rl.visitors[ip] = limiter
	}

	return limiter
}

var (
	rateLimiter = NewRateLimiter()
	// Extended list of dangerous imports and functions
	disallowedPatterns = []string{
		`import\s+"os/exec"`,
		`import\s+"net/http"`,
		`import\s+"syscall"`,
		`import\s+"unsafe"`,
		`import\s+"debug/.*"`,
		`import\s+"plugin"`,
		`import\s+"runtime/debug"`,
		`\bos\.Exec\b`,
		`\bos\.Setenv\b`,
		`\bos\.Remove\b`,
		`\bos\.Chmod\b`,
		`\bfile\.\w+\b`,
		`\bsyscall\.\w+\b`,
		`\bunsafe\.\w+\b`,
		`\bexec\.\w+\b`,
		`\bnet\.\w+\b`,
		`\bdebug\.\w+\b`,
		`\bplugin\.\w+\b`,
		`\bgo\s+func\b`,        // Preventing goroutines
		`\bmake\(\w+,\s*\d+\)`, // Preventing large slice allocation
	}
	containerID string
	localClient *client.Client
)

func main() {
	log.Println("Starting Go Playground...")

	var err error
	log.Println("Initializing Docker client...")
	localClient, err = client.NewClientWithOpts(client.FromEnv, client.WithTimeout(time.Second*30))
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer localClient.Close()

	// Ensure container is ready before starting server
	if err := ensureContainer(); err != nil {
		log.Fatalf("Failed to ensure container: %v", err)
	}

	// Pre-warm the container
	if err := prewarmContainer(); err != nil {
		log.Printf("Warning: Failed to pre-warm container: %v", err)
	}

	log.Println("Starting HTTP server...")
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/run", handleRun)
	http.HandleFunc("/health", handleHealth)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func ensureContainer() error {
	ctx := context.Background()
	log.Println("Checking for existing container...")

	// Check if container exists and is running
	containers, err := localClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to list containers: %v", err)
	}

	for _, cont := range containers {
		for _, name := range cont.Names {
			if name == "/"+containerName {
				log.Printf("Found existing container %s with state %s\n", cont.ID[:12], cont.State)

				if cont.State == "running" {
					containerID = cont.ID
					return nil
				}

				log.Printf("Removing stopped container %s\n", cont.ID[:12])
				err := localClient.ContainerRemove(ctx, cont.ID, container.RemoveOptions{Force: true})
				if err != nil {
					return fmt.Errorf("failed to remove stopped container: %v", err)
				}
			}
		}
	}

	log.Println("Creating new container...")

	config := &container.Config{
		Image:      dockerImage,
		Cmd:        []string{"sh", "-c", "while true; do sleep 1; done"},
		WorkingDir: "/code",
		Env: []string{
			"GOMEMLIMIT=50MiB",
			"GOGC=50",
			"CGO_ENABLED=0",
		},
	}

	pidsLimit := int64(100)
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:     memoryLimit,
			MemorySwap: memoryLimit,
			NanoCPUs:   1000000000,
			PidsLimit:  &pidsLimit,
		},
		NetworkMode: "none",
		AutoRemove:  false,
		SecurityOpt: []string{"no-new-privileges"},
	}

	resp, err := localClient.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	/* 	if err != nil {
		if client.IsErrNotFound(err) {
			log.Println("Image not found locally, pulling...")
			if _, err := localClient.ImagePull(ctx, dockerImage, types.ImagePullOptions{}); err != nil {
				return fmt.Errorf("failed to pull image: %v", err)
			}
			// Try creating container again after pulling image
			resp, err = localClient.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
			if err != nil {
				return fmt.Errorf("failed to create container after pulling image: %v", err)
			}
		} else {
			return fmt.Errorf("failed to create container: %v", err)
		}
	} */

	log.Printf("Starting container %s\n", resp.ID[:12])
	if err := localClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	containerID = resp.ID
	return nil
}

func prewarmContainer() error {
	log.Println("Pre-warming container...")
	// Simple program to compile
	warmupCode := `package main
	
	func main() {
		println("warm")
	}
	`
	_, err := runCode(warmupCode)
	if err != nil {
		return fmt.Errorf("failed to pre-warm container: %v", err)
	}
	log.Println("Container pre-warmed successfully")
	return nil
}

func runCode(code string) (string, error) {
	start := time.Now()
	defer func() {
		log.Printf("Code execution took: %v\n", time.Since(start))
	}()

	if !validateGoCode(code) {
		return "", fmt.Errorf("invalid or potentially unsafe Go code")
	}

	log.Println("Creating temporary directory...")
	tempDir, err := os.MkdirTemp("", "goplayground")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(tempFile, []byte(code), 0600); err != nil {
		return "", err
	}

	ctx := context.Background()
	log.Println("Copying code to container...")
	tar := createTarFromFile(tempFile)
	if err := localClient.CopyToContainer(ctx, containerID, "/code", tar, types.CopyToContainerOptions{}); err != nil {
		return "", fmt.Errorf("failed to copy code to container: %v", err)
	}

	log.Println("Creating exec instance...")
	execConfig := container.ExecOptions{
		Cmd:          []string{"go", "run", "/code/main.go"},
		WorkingDir:   "/code",
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := localClient.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %v", err)
	}

	log.Println("Starting exec instance...")
	response, err := localClient.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec: %v", err)
	}
	defer response.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, response.Reader); err != nil {
		return "", fmt.Errorf("failed to read exec output: %v", err)
	}

	if stderr.Len() > 0 {
		return "", fmt.Errorf("execution error: %s", stderr.String())
	}

	return stdout.String(), nil
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(templates.Templates, "form.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		log.Printf("Total request handling took: %v\n", time.Since(start))
	}()

	ip := r.RemoteAddr
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ip = strings.Split(forwardedFor, ",")[0]
	}

	// Apply rate limiting
	limiter := rateLimiter.getLimiter(ip)
	if !limiter.Allow() {
		http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
		return
	}

	code := r.FormValue("code")
	if len(code) > maxCodeSize {
		http.Error(w, fmt.Sprintf("Code size exceeds maximum limit of %d bytes", maxCodeSize), http.StatusBadRequest)
		return
	}

	output, err := runCode(code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(output) > maxOutputSize {
		http.Error(w, fmt.Sprintf("Output size exceeds maximum limit of %d bytes", maxOutputSize), http.StatusBadRequest)
		return
	}

	fmt.Fprintln(w, output)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	_, err := localClient.ContainerInspect(ctx, containerID)
	if err != nil {
		http.Error(w, "Container not healthy", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintln(w, "OK")
}
func createTarFromFile(filePath string) io.Reader {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()

	// Read the file
	file, err := os.Open(filePath)
	if err != nil {
		return &buf
	}
	defer file.Close()

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return &buf
	}

	// Create tar header
	header := &tar.Header{
		Name:    "main.go",
		Size:    info.Size(),
		Mode:    0600,
		ModTime: time.Now(),
	}

	// Write header
	if err := tw.WriteHeader(header); err != nil {
		return &buf
	}

	// Copy file content to tar
	if _, err := io.Copy(tw, file); err != nil {
		return &buf
	}

	return &buf
}

func validateGoCode(code string) bool {
	// Check for dangerous patterns
	for _, pattern := range disallowedPatterns {
		match, _ := regexp.MatchString(pattern, code)
		if match {
			return false
		}
	}

	// Additional code validation
	if strings.Count(code, "func") > 50 {
		return false // Prevent too many functions
	}

	if strings.Count(code, "for") > 10 {
		return false // Limit number of loops
	}

	if strings.Count(code, "go ") > 0 {
		return false // Prevent goroutines
	}

	return true
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	robotsTxt := []byte("User-agent: *\nDisallow: /private/")
	w.Header().Set("Content-Type", "text/plain")
	w.Write(robotsTxt)
}
