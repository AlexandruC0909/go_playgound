package main

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/AlexandruC0909/playground/internal/config"
	"github.com/AlexandruC0909/playground/internal/docker"
	"github.com/AlexandruC0909/playground/internal/handlers"
	"github.com/AlexandruC0909/playground/internal/utils"
	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var (
	rateLimiter    = utils.NewRateLimiter()
	containerID    string
	localClient    *client.Client
	container      *docker.Container
	executor       *docker.Executor
	activeSessions = sync.Map{}
)

func init() {
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".html", "text/html")
}

func fileServer(root http.FileSystem) http.Handler {
	fs := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ext := filepath.Ext(r.URL.Path)
		
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			w.Header().Set("Content-Type", mimeType)
		}
		
		fs.ServeHTTP(w, r)
	})
}

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

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", handlers.HandleHome)
	r.Post("/run", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleRun(w, r, rateLimiter, &activeSessions, executor)
	})
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleHealth(w, r, containerID, localClient)
	})
	r.Post("/save", handlers.HandleSave)
	r.Get("/robots.txt", handlers.HandleRobots)
	r.Get("/program-output", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleProgramOutput(w, r, &activeSessions)
	})
	r.Post("/send-input", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleSendInput(w, r, &activeSessions)
	})

	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer(filesDir)))

	log.Println("Server starting on :8088")
	log.Fatal(http.ListenAndServe(":8088", r))
}