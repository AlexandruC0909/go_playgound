package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"html/template"
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

	"github.com/AlexandruC0909/playground/internal/config"
	"github.com/AlexandruC0909/playground/internal/docker"
	"github.com/AlexandruC0909/playground/internal/models"
	"github.com/AlexandruC0909/playground/templates"
	"github.com/docker/docker/client"
	"golang.org/x/time/rate"
)

var (
	rateLimiter    = NewRateLimiter()
	containerID    string
	localClient    *client.Client
	container      *docker.Container
	executor       *docker.Executor
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
		limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(config.RequestsPerMinute)), 1)
		rl.visitors[ip] = limiter
	}

	return limiter
}

func main() {
	log.Println("Starting Go Playground...")

	containerConfig := docker.ContainerConfig{
		Name:        config.ContainerName,
		Image:       config.DockerImage,
		MemoryLimit: config.MemoryLimit,
		WorkDir:     "/code",
	}

	container, err := docker.NewContainer(containerConfig)
	if err != nil {
		log.Fatalf("Failed to create Docker container: %v", err)
	}
	defer container.Close()

	if err := container.Ensure(); err != nil {
		log.Fatalf("Failed to ensure container: %v", err)
	}
	executor = docker.NewExecutor(container, "/code")

	log.Println("Starting HTTP server...")
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/run", handleRun)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/save", handleSave)
	http.HandleFunc("/robots.txt", handleRobots)
	http.HandleFunc("/program-output", handleProgramOutput)
	http.HandleFunc("/send-input", handleSendInput)

	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "../../static"))
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
	session := sessionInterface.(*models.ProgramSession)

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
		case output, ok := <-session.OutputChan:
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
		case <-session.Done:
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
	session := sessionInterface.(*models.ProgramSession)

	var inputReq models.InputRequest
	if err := json.NewDecoder(r.Body).Decode(&inputReq); err != nil {
		http.Error(w, "Error decoding JSON", http.StatusBadRequest)
		return
	}

	select {
	case session.InputChan <- inputReq.Input:
		w.WriteHeader(http.StatusOK)
	case <-session.Done:
		http.Error(w, "Program execution completed", http.StatusGone)
	case <-time.After(5 * time.Second):
		http.Error(w, "Timeout waiting for program to accept input", http.StatusRequestTimeout)
	}
}

func validateGoCode(code string) bool {
	for _, pattern := range config.DisallowedPatterns {
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

func cacheControlWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		h.ServeHTTP(w, r)
	})
}

func detectInputOperations(code string) ([]models.InputOperation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", code, parser.AllErrors)
	if err != nil {
		return nil, err
	}

	var operations []models.InputOperation

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
								operations = append(operations, models.InputOperation{
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

func handleRun(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer logTiming("Total request handling", start)

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
	session := models.NewSession()

	activeSessions.Store(sessionID, session)

	go executeCode(requestData.Code, session, sessionID)

	return sessionID, nil
}

func executeCode(code string, session *models.ProgramSession, sessionID uint64) {
	start := time.Now()

	defer logTiming("Code execution", start)
	defer func() {
		session.Close()
		activeSessions.Delete(sessionID)
	}()

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
				oldSession.(*models.ProgramSession).Close()
				activeSessions.Delete(oldSessionID)
			}
		}
	}
}

func validateAndPrepare(code string, session *models.ProgramSession) error {
	inputOps, err := detectInputOperations(code)
	if err != nil {
		return fmt.Errorf("failed to analyze code for input operations: %v", err)
	}
	session.DetectedInputOps = inputOps

	if !validateGoCode(code) {
		return fmt.Errorf("invalid or potentially unsafe Go code")
	}
	return nil
}

func logTiming(operation string, start time.Time) {
	log.Printf("%s took: %v\n", operation, time.Since(start))
}

func sendError(session *models.ProgramSession, errMsg string) {
	session.OutputChan <- models.ProgramOutput{
		Error: errMsg,
		Done:  true,
	}
}
