package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexandruC0909/playground/templates"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
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
		//`\bgo\s+func\b`,        // Preventing goroutines
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

	log.Println("Starting HTTP server...")
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/run", handleRun)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/save", handleSave)
	http.HandleFunc("/robots.txt", robotsHandler)
	http.HandleFunc("/run-with-input", handleRunWithInput)
	http.HandleFunc("/program-output", handleProgramOutput)
	http.HandleFunc("/send-input", handleSendInput)

	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "/static"))
	http.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(filesDir)))

	log.Fatal(http.ListenAndServe(":8088", nil))
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
	if err != nil {
		if client.IsErrNotFound(err) {
			log.Println("Image not found locally, pulling...")
			if _, err := localClient.ImagePull(ctx, dockerImage, image.PullOptions{}); err != nil {
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
	}

	log.Printf("Starting container %s\n", resp.ID[:12])
	if err := localClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	containerID = resp.ID
	return nil
}

func runCode(code string) (string, error) {
	start := time.Now()
	defer func() {
		log.Printf("Code execution took: %v\n", time.Since(start))
	}()
	var ErrInvalidGoCode = errors.New("invalid or potentially unsafe Go code")
	if !validateGoCode(code) {
		return "", fmt.Errorf(`%w`, ErrInvalidGoCode)
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
	if err := localClient.CopyToContainer(ctx, containerID, "/code", tar, container.CopyToContainerOptions{}); err != nil {
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
	response, err := localClient.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
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

	var requestData struct {
		Code string `json:"code"`
	}

	err := json.NewDecoder(r.Body).Decode(&requestData)
	if err != nil {
		http.Error(w, "Error decoding JSON", http.StatusBadRequest)
		return
	}

	if len(requestData.Code) > maxCodeSize {
		http.Error(w, fmt.Sprintf("Code size exceeds maximum limit of %d bytes", maxCodeSize), http.StatusBadRequest)
		return
	}

	output, err := runCode(requestData.Code)
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

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var requestData struct {
		Code string `json:"code"`
	}

	err := json.NewDecoder(r.Body).Decode(&requestData)
	if err != nil {
		http.Error(w, "Error decoding JSON", http.StatusBadRequest)
		return
	}

	formatted, err := format.Source([]byte(requestData.Code))
	if err != nil {
		http.Error(w, "Error formatting code", http.StatusInternalServerError)
		return
	}

	responseData := struct {
		Code string `json:"code"`
	}{
		Code: string(formatted),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responseData)
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

	file, err := os.Open(filePath)
	if err != nil {
		return &buf
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return &buf
	}

	header := &tar.Header{
		Name:    "main.go",
		Size:    info.Size(),
		Mode:    0600,
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return &buf
	}

	if _, err := io.Copy(tw, file); err != nil {
		return &buf
	}

	return &buf
}

func validateGoCode(code string) bool {
	for _, pattern := range disallowedPatterns {
		match, _ := regexp.MatchString(pattern, code)
		if match {
			return false
		}
	}

	if strings.Count(code, "func") > 50 {
		return false // Prevent too many functions
	}

	if strings.Count(code, "for") > 30 {
		return false // Limit number of loops
	}
	/*
		if strings.Count(code, "go ") > 0 {
			return false // Prevent goroutines
		} */

	return true
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	robotsTxt := []byte("User-agent: *\nDisallow: /private/")
	w.Header().Set("Content-Type", "text/plain")
	w.Write(robotsTxt)
}

func cacheControlWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		h.ServeHTTP(w, r)
	})
}

type ProgramOutput struct {
	Output          string `json:"output,omitempty"`
	Error           string `json:"error,omitempty"`
	WaitingForInput bool   `json:"waitingForInput"`
	Done            bool   `json:"done"`
}

type InputRequest struct {
	Input string `json:"input"`
}

type ProgramSession struct {
	inputChan  chan string
	outputChan chan ProgramOutput
	done       chan struct{}
	cleanup    sync.Once
}

func (s *ProgramSession) Close() {
	s.cleanup.Do(func() {
		close(s.done)
		close(s.inputChan)
	})
}

var (
	activeSessions = sync.Map{}
	sessionCounter uint64
)

func newSession() *ProgramSession {
	return &ProgramSession{
		inputChan:  make(chan string),
		outputChan: make(chan ProgramOutput),
		done:       make(chan struct{}),
	}
}

func handleRunWithInput(w http.ResponseWriter, r *http.Request) {
	if oldSessionIDStr := r.Header.Get("X-Previous-Session"); oldSessionIDStr != "" {
		if oldSessionID, err := strconv.ParseUint(oldSessionIDStr, 10, 64); err == nil {
			if oldSession, ok := activeSessions.Load(oldSessionID); ok {
				oldSession.(*ProgramSession).Close()
				activeSessions.Delete(oldSessionID)
			}
		}
	}

	var requestData struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, "Error decoding JSON", http.StatusBadRequest)
		return
	}

	sessionID := atomic.AddUint64(&sessionCounter, 1)
	session := newSession()

	activeSessions.Store(sessionID, session)

	go func() {
		defer session.Close()
		defer activeSessions.Delete(sessionID)
		runCodeInteractive(requestData.Code, session)

	}()

	json.NewEncoder(w).Encode(map[string]uint64{"sessionId": sessionID})
}

func handleProgramOutput(w http.ResponseWriter, r *http.Request) {
	sessionID, err := strconv.ParseUint(r.URL.Query().Get("sessionId"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	sessionInterface, ok := activeSessions.Load(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	session := sessionInterface.(*ProgramSession)

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Create done channel for this connection
	done := make(chan struct{})
	defer close(done)

	/* 	// Close connection if client disconnects
	   	go func() {
	   		<-r.Context().Done()
	   		close(done)
	   	}() */

	for {
		select {
		case output, ok := <-session.outputChan:
			if !ok {
				return
			}
			data, _ := json.Marshal(output)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			if output.Done || output.Error != "" {
				return
			}
		case <-done:
			return
		case <-session.done:
			return
		}
	}
}

func handleSendInput(w http.ResponseWriter, r *http.Request) {
	sessionID, err := strconv.ParseUint(r.URL.Query().Get("sessionId"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	sessionInterface, ok := activeSessions.Load(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	session := sessionInterface.(*ProgramSession)

	var inputReq InputRequest
	if err := json.NewDecoder(r.Body).Decode(&inputReq); err != nil {
		http.Error(w, "Error decoding JSON", http.StatusBadRequest)
		return
	}

	select {
	case session.inputChan <- inputReq.Input:
		w.WriteHeader(http.StatusOK)
	case <-session.done:
		http.Error(w, "Program execution completed", http.StatusGone)
	case <-time.After(5 * time.Second):
		http.Error(w, "Timeout waiting for program to accept input", http.StatusRequestTimeout)
	}
}

func runCodeInteractive(code string, session *ProgramSession) {
	defer close(session.outputChan)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if !validateGoCode(code) {
		session.outputChan <- ProgramOutput{
			Error: "invalid or potentially unsafe Go code",
			Done:  true,
		}
		return
	}

	tempDir, err := os.MkdirTemp("", "goplayground")
	if err != nil {
		session.outputChan <- ProgramOutput{
			Error: err.Error(),
			Done:  true,
		}
		return
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(tempFile, []byte(code), 0600); err != nil {
		session.outputChan <- ProgramOutput{
			Error: err.Error(),
			Done:  true,
		}
		return
	}

	tar := createTarFromFile(tempFile)
	if err := localClient.CopyToContainer(ctx, containerID, "/code", tar, container.CopyToContainerOptions{}); err != nil {
		session.outputChan <- ProgramOutput{
			Error: fmt.Sprintf("failed to copy code to container: %v", err),
			Done:  true,
		}
		return
	}

	// First, try to compile the code
	compileConfig := container.ExecOptions{
		Cmd:          []string{"go", "build", "-o", "/dev/null", "/code/main.go"},
		WorkingDir:   "/code",
		AttachStdout: true,
		AttachStderr: true,
	}

	compileResp, err := localClient.ContainerExecCreate(ctx, containerID, compileConfig)
	if err != nil {
		session.outputChan <- ProgramOutput{
			Error: fmt.Sprintf("failed to create compile exec: %v", err),
			Done:  true,
		}
		return
	}

	compileAttach, err := localClient.ContainerExecAttach(ctx, compileResp.ID, container.ExecStartOptions{})
	if err != nil {
		session.outputChan <- ProgramOutput{
			Error: fmt.Sprintf("failed to attach to compile exec: %v", err),
			Done:  true,
		}
		return
	}
	defer compileAttach.Close()

	var compileStdout, compileStderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&compileStdout, &compileStderr, compileAttach.Reader); err != nil {
		session.outputChan <- ProgramOutput{
			Error: fmt.Sprintf("failed to read compile output: %v", err),
			Done:  true,
		}
		return
	}

	compileResult, err := localClient.ContainerExecInspect(ctx, compileResp.ID)
	if err != nil {
		session.outputChan <- ProgramOutput{
			Error: fmt.Sprintf("failed to inspect compile exec: %v", err),
			Done:  true,
		}
		return
	}

	if compileResult.ExitCode != 0 {
		session.outputChan <- ProgramOutput{
			Error:           compileStderr.String(),
			Done:            true,
			WaitingForInput: false,
		}
		return
	}

	execConfig := container.ExecOptions{
		Cmd:          []string{"go", "run", "/code/main.go"},
		WorkingDir:   "/code",
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}

	execResp, err := localClient.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		session.outputChan <- ProgramOutput{
			Error: fmt.Sprintf("failed to create exec: %v", err),
			Done:  true,
		}
		return
	}

	response, err := localClient.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		session.outputChan <- ProgramOutput{
			Error: fmt.Sprintf("failed to attach to exec: %v", err),
			Done:  true,
		}
		return
	}
	defer response.Close()

	reader := bufio.NewReader(response.Reader)
	outputDone := make(chan struct{})

	// Handle program output and errors
	go func() {
		defer close(outputDone)
		for {
			header := make([]byte, 8)
			_, err := reader.Read(header)
			if err != nil {
				if err != io.EOF {
					session.outputChan <- ProgramOutput{
						Error:           fmt.Sprintf("error reading output: %v", err),
						Done:            true,
						WaitingForInput: false,
					}
				} else {
					// Send final output with Done: true on EOF
					session.outputChan <- ProgramOutput{
						Done:            true,
						WaitingForInput: false,
					}
				}
				return
			}

			streamType := header[0]
			size := int64(binary.BigEndian.Uint32(header[4:]))

			content := make([]byte, size)
			_, err = io.ReadFull(reader, content)
			if err != nil {
				session.outputChan <- ProgramOutput{
					Error:           fmt.Sprintf("error reading content: %v", err),
					Done:            true,
					WaitingForInput: false,
				}
				return
			}

			var output ProgramOutput
			if streamType == 2 { // stderr
				output = ProgramOutput{
					Error:           string(content),
					WaitingForInput: false,
				}
			} else { // stdout
				output = ProgramOutput{
					Output:          string(content),
					WaitingForInput: true,
				}
			}

			select {
			case <-session.done:
				return
			case session.outputChan <- output:
			}
		}
	}()

	// Handle program input
	for {
		select {
		case input, ok := <-session.inputChan:
			if !ok {
				return
			}
			fmt.Fprintln(response.Conn, input)
		case <-session.done:
			return
		case <-outputDone:
			// Program has finished executing
			return
		}
	}
}
