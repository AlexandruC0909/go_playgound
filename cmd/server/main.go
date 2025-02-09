package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/AlexandruC0909/playground/internal/config"
	"github.com/AlexandruC0909/playground/internal/docker"
	"github.com/AlexandruC0909/playground/internal/handlers"
	"github.com/AlexandruC0909/playground/internal/utils"
	"github.com/docker/docker/client"
)

var (
	rateLimiter    = utils.NewRateLimiter()
	containerID    string
	localClient    *client.Client
	container      *docker.Container
	executor       *docker.Executor
	activeSessions = sync.Map{}
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

	http.HandleFunc("/", handlers.HandleHome)
	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleRun(w, r, rateLimiter, &activeSessions, executor)
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleHealth(w, r, containerID, localClient)
	})
	http.HandleFunc("/save", handlers.HandleSave)
	http.HandleFunc("/robots.txt", handlers.HandleRobots)
	http.HandleFunc("/program-output", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleProgramOutput(w, r, &activeSessions)
	})
	http.HandleFunc("/send-input", func(w http.ResponseWriter, r *http.Request) {
		handlers.HandleSendInput(w, r, &activeSessions)
	})

	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "static"))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(filesDir)))

	log.Fatal(http.ListenAndServe(":8088", nil))
}