package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"go/format"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/AlexandruC0909/playground/internal/docker"
	"github.com/AlexandruC0909/playground/internal/models"
	"github.com/AlexandruC0909/playground/internal/utils"
	"github.com/AlexandruC0909/playground/templates"
	"github.com/docker/docker/client"
)

var (
	sessionCounter uint64
)

func HandleHome(w http.ResponseWriter, r *http.Request) {
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

func HandleSave(w http.ResponseWriter, r *http.Request) {
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

func HandleHealth(w http.ResponseWriter, r *http.Request, containerID string, localClient *client.Client) {
	ctx := context.Background()
	_, err := localClient.ContainerInspect(ctx, containerID)
	if err != nil {
		http.Error(w, "Container not healthy", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintln(w, "OK")
}

func HandleRobots(w http.ResponseWriter, r *http.Request) {
	robotsTxt := []byte("User-agent: *\nDisallow: /private/")
	w.Header().Set("Content-Type", "text/plain")
	w.Write(robotsTxt)
}

func HandleProgramOutput(w http.ResponseWriter, r *http.Request, activeSessions *sync.Map) {
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

func HandleSendInput(w http.ResponseWriter, r *http.Request, activeSessions *sync.Map) {
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

func HandleRun(w http.ResponseWriter, r *http.Request, rateLimiter *utils.RateLimiter, activeSessions *sync.Map, executor *docker.Executor) {
	start := time.Now()
	defer utils.LogTiming("Total request handling", start)

	sessionID, err := handleRequest(r, rateLimiter, activeSessions, executor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(map[string]uint64{"sessionId": sessionID})
}

func handleRequest(r *http.Request, rateLimiter *utils.RateLimiter, activeSessions *sync.Map, executor *docker.Executor) (uint64, error) {
	ip := utils.ExtractIP(r)

	if err := utils.CheckRateLimit(rateLimiter, ip); err != nil {
		return 0, err
	}

	requestData, err := utils.ParseRequestBody(r)
	if err != nil {
		return 0, err
	}

	utils.CleanupPreviousSession(activeSessions, r)

	sessionID := atomic.AddUint64(&sessionCounter, 1)
	session := models.NewSession()

	activeSessions.Store(sessionID, session)

	go executeCode(requestData.Code, session, sessionID, executor, activeSessions)

	return sessionID, nil
}

func executeCode(code string, session *models.ProgramSession, sessionID uint64, executor *docker.Executor, activeSessions *sync.Map) {
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
