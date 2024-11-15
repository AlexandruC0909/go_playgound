package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
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
	"github.com/docker/docker/api/types"
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
	containerID    string
	localClient    *client.Client
	activeSessions = sync.Map{}
	sessionCounter uint64
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
	inputChan        chan string
	outputChan       chan ProgramOutput
	done             chan struct{}
	cleanup          sync.Once
	detectedInputOps []InputOperation
}

type InputOperation struct {
	Line    int
	Type    string // Type of input operation (e.g., "fmt.Scan", "bufio.Scanner", etc.)
	Package string
}

func newSession() *ProgramSession {
	return &ProgramSession{
		inputChan:  make(chan string),
		outputChan: make(chan ProgramOutput),
		done:       make(chan struct{}),
	}
}

func (s *ProgramSession) Close() {
	s.cleanup.Do(func() {
		close(s.done)
		close(s.inputChan)
	})
}

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
	http.HandleFunc("/robots.txt", handleRobots)
	http.HandleFunc("/program-output", handleProgramOutput)
	http.HandleFunc("/send-input", handleSendInput)

	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "/static"))
	http.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(filesDir)))

	log.Fatal(http.ListenAndServe(":8088", nil))
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

func handleRobots(w http.ResponseWriter, r *http.Request) {
	robotsTxt := []byte("User-agent: *\nDisallow: /private/")
	w.Header().Set("Content-Type", "text/plain")
	w.Write(robotsTxt)
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

func cacheControlWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		h.ServeHTTP(w, r)
	})
}

func detectInputOperations(code string) ([]InputOperation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", code, parser.AllErrors)
	if err != nil {
		return nil, err
	}

	var operations []InputOperation

	inputFuncs := map[string][]string{
		"fmt": {
			"Scan", "Scanf", "Scanln",
			"Fscan", "Fscanf", "Fscanln",
			"Sscan", "Sscanf", "Sscanln",
		},
		"bufio": {
			"NewScanner",
		},
		"os": {
			"Stdin",
		},
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok {
					// Check if it's a known input function
					if funcs, exists := inputFuncs[pkg.Name]; exists {
						for _, funcName := range funcs {
							if sel.Sel.Name == funcName {
								operations = append(operations, InputOperation{
									Line:    fset.Position(x.Pos()).Line,
									Type:    pkg.Name + "." + sel.Sel.Name,
									Package: pkg.Name,
								})
							}
						}
					}
				}
			}
		case *ast.ImportSpec:
			return true
		}
		return true
	})

	return operations, nil
}

func isWaitingForInput(output string, detectedOps []InputOperation) bool {
	if len(detectedOps) > 0 {
		inputPatterns := []string{
			"input", "enter", "type", "?", ">", ":",
		}

		outputLower := strings.ToLower(strings.TrimSpace(output))

		for _, pattern := range inputPatterns {
			if strings.HasSuffix(outputLower, pattern) {
				return true
			}
		}

		if len(output) > 0 {
			lastChar := output[len(output)-1]
			if lastChar != '\n' && lastChar != '\r' {
				return true
			}
		}
	}

	return false
}

// Types and interfaces to improve structure
type ExecutionResult struct {
	Error           string
	Output          string
	Done            bool
	WaitingForInput bool
}

type CodeExecutor interface {
	Compile(ctx context.Context, code string) error
	Run(ctx context.Context, session *ProgramSession) error
}

type DockerExecutor struct {
	client      *client.Client
	containerID string
	workDir     string
}

// NewDockerExecutor creates a new DockerExecutor instance
func NewDockerExecutor(client *client.Client, containerID string) *DockerExecutor {
	return &DockerExecutor{
		client:      client,
		containerID: containerID,
		workDir:     "/code",
	}
}

func (e *DockerExecutor) Compile(ctx context.Context, code string) error {
	tempDir, err := os.MkdirTemp("", "goplayground")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(tempFile, []byte(code), 0600); err != nil {
		return fmt.Errorf("failed to write code to file: %v", err)
	}

	tar := createTarFromFile(tempFile)
	if err := e.client.CopyToContainer(ctx, e.containerID, e.workDir, tar, types.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("failed to copy code to container: %v", err)
	}

	execConfig := types.ExecConfig{
		Cmd:          []string{"go", "build", "-o", "/dev/null", filepath.Join(e.workDir, "main.go")},
		WorkingDir:   e.workDir,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := e.client.ContainerExecCreate(ctx, e.containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create compile exec: %v", err)
	}

	response, err := e.client.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return fmt.Errorf("failed to attach to compile exec: %v", err)
	}
	defer response.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, response.Reader); err != nil {
		return fmt.Errorf("failed to read compile output: %v", err)
	}

	inspect, err := e.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return fmt.Errorf("failed to inspect compile exec: %v", err)
	}

	if inspect.ExitCode != 0 {
		return fmt.Errorf("compilation failed: %s", stderr.String())
	}

	return nil
}

func (e *DockerExecutor) Run(ctx context.Context, session *ProgramSession) error {
	execConfig := container.ExecOptions{
		Cmd:          []string{"go", "run", filepath.Join(e.workDir, "main.go")},
		WorkingDir:   e.workDir,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}

	execID, err := e.client.ContainerExecCreate(ctx, e.containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create run exec: %v", err)
	}

	response, err := e.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to attach to run exec: %v", err)
	}
	defer response.Close()

	return e.handleExecIO(ctx, response, session)
}

func (e *DockerExecutor) handleExecIO(ctx context.Context, response types.HijackedResponse, session *ProgramSession) error {
	reader := bufio.NewReader(response.Reader)
	outputDone := make(chan struct{})

	go e.processOutput(reader, session, outputDone)

	return e.processInput(response, session, outputDone)
}

func (e *DockerExecutor) processOutput(reader *bufio.Reader, session *ProgramSession, outputDone chan struct{}) {
	defer close(outputDone)
	defer close(session.outputChan)

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
		if _, err = io.ReadFull(reader, content); err != nil {
			session.outputChan <- ProgramOutput{
				Error:           fmt.Sprintf("error reading content: %v", err),
				Done:            true,
				WaitingForInput: false,
			}
			return
		}

		outputStr := string(content)
		isInput := isWaitingForInput(outputStr, session.detectedInputOps)

		output := ProgramOutput{
			Output:          outputStr,
			WaitingForInput: isInput,
		}
		if streamType == 2 { // stderr
			output.Error = outputStr
			output.Output = ""
			output.WaitingForInput = false
		}

		select {
		case <-session.done:
			return
		case session.outputChan <- output:
		}
	}
}

func (e *DockerExecutor) processInput(response types.HijackedResponse, session *ProgramSession, outputDone chan struct{}) error {
	for {
		select {
		case input, ok := <-session.inputChan:
			if !ok {
				return nil
			}
			if _, err := fmt.Fprintln(response.Conn, input); err != nil {
				return fmt.Errorf("failed to write input: %v", err)
			}
		case <-session.done:
			return nil
		case <-outputDone:
			return nil
		}
	}
}

func handleRun(w http.ResponseWriter, r *http.Request) {

	start := time.Now()

	defer logTiming("Total request handling", start)

	// Extract request handling to separate function
	sessionID, err := handleRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(map[string]uint64{"sessionId": sessionID})

}

func handleRequest(w http.ResponseWriter, r *http.Request) (uint64, error) {
	ip := extractIP(r)

	if err := checkRateLimit(ip); err != nil {
		return 0, err
	}

	requestData, err := parseRequestBody(r)
	if err != nil {
		return 0, err
	}

	cleanupPreviousSession(r)

	sessionID := atomic.AddUint64(&sessionCounter, 1)
	session := newSession()

	activeSessions.Store(sessionID, session)

	go executeCode(requestData.Code, session, sessionID)

	return sessionID, nil
}

func executeCode(code string, session *ProgramSession, sessionID uint64) {
	start := time.Now()

	defer logTiming("Code execution took:", start)
	defer func() {
		session.Close()
		activeSessions.Delete(sessionID)
	}()

	executor := &DockerExecutor{
		client:      localClient,
		containerID: containerID,
		workDir:     "/code",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := validateAndPrepare(code, session); err != nil {
		sendError(session, err.Error())
		return
	}

	if err := executor.Compile(ctx, code); err != nil {
		sendError(session, err.Error())
		return
	}

	if err := executor.Run(ctx, session); err != nil {
		sendError(session, err.Error())
		return
	}
}

// Helper functions
func extractIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		return strings.Split(forwardedFor, ",")[0]
	}
	return r.RemoteAddr
}

func checkRateLimit(ip string) error {
	limiter := rateLimiter.getLimiter(ip)
	if !limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	return nil
}

func parseRequestBody(r *http.Request) (struct {
	Code string `json:"code"`
}, error) {
	var requestData struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		return requestData, fmt.Errorf("error decoding JSON: %v", err)
	}
	return requestData, nil
}

func cleanupPreviousSession(r *http.Request) {
	if oldSessionIDStr := r.Header.Get("X-Previous-Session"); oldSessionIDStr != "" {
		if oldSessionID, err := strconv.ParseUint(oldSessionIDStr, 10, 64); err == nil {
			if oldSession, ok := activeSessions.Load(oldSessionID); ok {
				oldSession.(*ProgramSession).Close()
				activeSessions.Delete(oldSessionID)
			}
		}
	}
}

func validateAndPrepare(code string, session *ProgramSession) error {
	inputOps, err := detectInputOperations(code)
	if err != nil {
		return fmt.Errorf("failed to analyze code for input operations: %v", err)
	}
	session.detectedInputOps = inputOps

	if !validateGoCode(code) {
		return fmt.Errorf("invalid or potentially unsafe Go code")
	}
	return nil
}

func logTiming(operation string, start time.Time) {
	log.Printf("%s took: %v\n", operation, time.Since(start))
}

func sendError(session *ProgramSession, errMsg string) {
	session.outputChan <- ProgramOutput{
		Error: errMsg,
		Done:  true,
	}
}
