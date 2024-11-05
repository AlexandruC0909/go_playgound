package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
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
	memoryLimit       = 150 * 1024 * 1024 // 50MB
	dockerImage       = "golang:1.22-alpine"
	maxCodeSize       = 1024 * 1024 // 1MB
	maxOutputSize     = 1024 * 1024 // 1MB
	requestsPerHour   = 1000
	requestsPerMinute = 500
	containerName     = "go-playground"
)

// RateLimiter manages rate limiting per IP address
type RateLimiter struct {
	visitors map[string]*rate.Limiter
	mu       sync.Mutex
}

var containerID string

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
)

func main() {
	// Create the Docker container when the server starts
	createContainer()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFS(templates.Templates, "form.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = tmpl.Execute(w, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/run", handleRun)
	http.HandleFunc("/robots.txt", robotsHandler)
	http.ListenAndServe(":8080", nil)
}

func createContainer() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(fmt.Errorf("failed to create Docker client: %v", err))
	}
	defer cli.Close()

	// Check if the container already exists
	_, err = cli.ContainerInspect(ctx, containerName)
	if err == nil {
		return // Container already exists, no need to create a new one
	}

	// Create container config
	config := &container.Config{
		Image:      dockerImage,
		Cmd:        []string{"sh", "-c", "while true; do sleep 1; done"}, // Keep the container running
		WorkingDir: "/code",
		Env: []string{
			"GOMEMLIMIT=50MiB",
			"GOGC=50",
		},
	}
	pidsLimit := int64(100)
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:     memoryLimit,
			MemorySwap: memoryLimit, // Disable swap
			NanoCPUs:   1000000000,  // 1 CPU
			PidsLimit:  &pidsLimit,  // Limit number of processes
		},
		NetworkMode: "none",
		AutoRemove:  false, // Do not remove the container when it stops
		SecurityOpt: []string{
			"no-new-privileges",
		},
	}

	// Create the container
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		panic(fmt.Errorf("failed to create container: %v", err))
	}

	// Start the container
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		panic(fmt.Errorf("failed to start container: %v", err))
	}

	containerID = resp.ID

}

func handleRun(w http.ResponseWriter, r *http.Request) {
	// Get IP address for rate limiting
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

	// Check code size
	if len(code) > maxCodeSize {
		http.Error(w, fmt.Sprintf("Code size exceeds maximum limit of %d bytes", maxCodeSize), http.StatusBadRequest)
		return
	}

	output, err := runCode(code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check output size
	if len(output) > maxOutputSize {
		http.Error(w, fmt.Sprintf("Output size exceeds maximum limit of %d bytes", maxOutputSize), http.StatusBadRequest)
		return
	}

	fmt.Fprintln(w, output)
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

func runCode(code string) (string, error) {
	if !validateGoCode(code) {
		return "", fmt.Errorf("invalid or potentially unsafe Go code")
	}
	tempDir, err := os.MkdirTemp("", "goplayground")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	// Write code to file
	tempFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(tempFile, []byte(code), 0600); err != nil {
		return "", err
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Copy code into the existing container

	tar := createTarFromFile(tempFile)
	if err := cli.CopyToContainer(ctx, containerName, "/code", tar, container.CopyToContainerOptions{}); err != nil {
		return "", fmt.Errorf("failed to copy code to container: %v", err)
	}

	// Run the code multiple times in the same container
	// Execute the command in the running container
	execConfig := container.ExecOptions{
		Cmd:          []string{"go", "run", "/code/main.go"},
		WorkingDir:   "/code",
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %v", err)
	}

	// Start the exec instance
	response, err := cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec: %v", err)
	}
	defer response.Close()
	out, err := cli.ContainerLogs(ctx, containerID.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %v", err)
	}
	defer out.Close()
	if _, err := stdcopy.StdCopy(&stdout, &stderr, out); err != nil {
		return "", fmt.Errorf("failed to read container output: %v", err)
	}

	// Read the output
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, response.Reader); err != nil {
		return "", fmt.Errorf("failed to read exec output: %v", err)
	}

	if stderr.Len() > 0 {
		return "", fmt.Errorf("execution error: %s", stderr.String())
	}

	return stdout.String(), nil
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	robotsTxt := []byte("User-agent: *\nDisallow: /private/")
	w.Header().Set("Content-Type", "text/plain")
	w.Write(robotsTxt)
}
