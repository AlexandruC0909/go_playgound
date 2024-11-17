package docker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type Container struct {
	client *client.Client
	ID     string
	config ContainerConfig
}

type ContainerConfig struct {
	Name        string
	Image       string
	MemoryLimit int64
	WorkDir     string
}

func NewContainer(config ContainerConfig) (*Container, error) {
	client, err := client.NewClientWithOpts(client.FromEnv, client.WithTimeout(time.Second*30))
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	return &Container{
		client: client,
		config: config,
	}, nil
}

func (c *Container) Close() error {
	return c.client.Close()
}

func (c *Container) Ensure() error {
	ctx := context.Background()
	log.Println("Checking for existing container...")

	// Check if container exists and is running
	containers, err := c.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to list containers: %v", err)
	}

	for _, cont := range containers {
		for _, name := range cont.Names {
			if name == "/"+c.config.Name {
				log.Printf("Found existing container %s with state %s\n", cont.ID[:12], cont.State)

				if cont.State == "running" {
					c.ID = cont.ID
					return nil
				}

				log.Printf("Removing stopped container %s\n", cont.ID[:12])
				err := c.client.ContainerRemove(ctx, cont.ID, container.RemoveOptions{Force: true})
				if err != nil {
					return fmt.Errorf("failed to remove stopped container: %v", err)
				}
			}
		}
	}

	log.Println("Creating new container...")

	containerConfig := &container.Config{
		Image:      c.config.Image,
		Cmd:        []string{"sh", "-c", "while true; do sleep 1; done"},
		WorkingDir: c.config.WorkDir,
		Env: []string{
			"GOMEMLIMIT=50MiB",
			"GOGC=50",
			"CGO_ENABLED=0",
		},
	}

	pidsLimit := int64(100)
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:     c.config.MemoryLimit,
			MemorySwap: c.config.MemoryLimit,
			NanoCPUs:   1000000000,
			PidsLimit:  &pidsLimit,
		},
		NetworkMode: "none",
		AutoRemove:  false,
		SecurityOpt: []string{"no-new-privileges"},
	}

	resp, err := c.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, c.config.Name)
	if err != nil {
		if client.IsErrNotFound(err) {
			log.Println("Image not found locally, pulling...")
			if _, err := c.client.ImagePull(ctx, c.config.Image, image.PullOptions{}); err != nil {
				return fmt.Errorf("failed to pull image: %v", err)
			}
			// Try creating container again after pulling image
			resp, err = c.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, c.config.Name)
			if err != nil {
				return fmt.Errorf("failed to create container after pulling image: %v", err)
			}
		} else {
			return fmt.Errorf("failed to create container: %v", err)
		}
	}

	log.Printf("Starting container %s\n", resp.ID[:12])
	if err := c.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	c.ID = resp.ID
	return nil
}

func (c *Container) IsHealthy(ctx context.Context) error {
	_, err := c.client.ContainerInspect(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("container not healthy: %v", err)
	}
	return nil
}
