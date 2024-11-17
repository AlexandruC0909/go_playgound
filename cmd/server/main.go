package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go/format"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexandruC0909/playground/internal/config"
	"github.com/AlexandruC0909/playground/internal/docker"
	"github.com/AlexandruC0909/playground/internal/models"
	"github.com/AlexandruC0909/playground/internal/utils"
	"github.com/AlexandruC0909/playground/templates"
	"github.com/docker/docker/client"
)

var (
	rateLimiter    = utils.NewRateLimiter()
	containerID    string
	localClient    *client.Client
	container      *docker.Container
	executor       *docker.Executor
	activeSessions = sync.Map{}
	sessionCounter uint64
)

func main() {
	log.Println("Starting Go Playground...")
	var err error
	containerConfig := docker.ContainerConfig{
		Name:        config.ContainerName,
		Image:       config.DockerImage,
		MemoryLimit: config.MemoryLimit,
		WorkDir:     "/code",
	}

	container, err = docker.NewContainer(containerConfig)
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

func cacheControlWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		h.ServeHTTP(w, r)
	})
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer utils.LogTiming("Total request handling", start)

	sessionID, err := handleRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(map[string]uint64{"sessionId": sessionID})

}

func handleRequest(r *http.Request) (uint64, error) {
	ip := utils.ExtractIP(r)

	if err := utils.CheckRateLimit(rateLimiter, ip); err != nil {
		return 0, err
	}

	requestData, err := utils.ParseRequestBody(r)
	if err != nil {
		return 0, err
	}

	utils.CleanupPreviousSession(&activeSessions, r)

	sessionID := atomic.AddUint64(&sessionCounter, 1)
	session := models.NewSession()

	activeSessions.Store(sessionID, session)

	go executeCode(requestData.Code, session, sessionID)

	return sessionID, nil
}

func executeCode(code string, session *models.ProgramSession, sessionID uint64) {
	start := time.Now()

	defer utils.LogTiming("Code execution", start)
	defer func() {
		session.Close()
		activeSessions.Delete(sessionID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := utils.ValidateAndPrepare(code, session); err != nil {
		utils.SendError(session, err.Error())
		return
	}

	if err := executor.Compile(ctx, code); err != nil {
		utils.SendError(session, err.Error())
		return
	}

	if err := executor.Run(ctx, session); err != nil {
		utils.SendError(session, err.Error())
		return
	}
}
