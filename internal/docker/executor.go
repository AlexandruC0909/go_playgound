package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexandruC0909/playground/internal/models"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

type Executor struct {
	container *Container
	workDir   string
}

func NewExecutor(container *Container, workDir string) *Executor {
	return &Executor{
		container: container,
		workDir:   workDir,
	}
}

func (e *Executor) Compile(ctx context.Context, code string) error {
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
	if err := e.container.client.CopyToContainer(ctx, e.container.ID, e.workDir, tar, types.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("failed to copy code to container: %v", err)
	}

	execConfig := container.ExecOptions{
		Cmd:          []string{"go", "build", "-o", "/dev/null", filepath.Join(e.workDir, "main.go")},
		WorkingDir:   e.workDir,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := e.container.client.ContainerExecCreate(ctx, e.container.ID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create compile exec: %v", err)
	}

	response, err := e.container.client.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return fmt.Errorf("failed to attach to compile exec: %v", err)
	}
	defer response.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, response.Reader); err != nil {
		return fmt.Errorf("failed to read compile output: %v", err)
	}

	inspect, err := e.container.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return fmt.Errorf("failed to inspect compile exec: %v", err)
	}

	if inspect.ExitCode != 0 {
		return fmt.Errorf("compilation failed: %s", stderr.String())
	}

	return nil
}

func (e *Executor) Run(ctx context.Context, session *models.ProgramSession) error {
	execConfig := container.ExecOptions{
		Cmd:          []string{"go", "run", filepath.Join(e.workDir, "main.go")},
		WorkingDir:   e.workDir,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}

	execID, err := e.container.client.ContainerExecCreate(ctx, e.container.ID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create run exec: %v", err)
	}

	response, err := e.container.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("failed to attach to run exec: %v", err)
	}
	defer response.Close()

	return e.handleExecIO(ctx, response, session)
}

func (e *Executor) handleExecIO(ctx context.Context, response types.HijackedResponse, session *models.ProgramSession) error {
	reader := bufio.NewReader(response.Reader)
	outputDone := make(chan struct{})

	go e.processOutput(reader, session, outputDone)

	return e.processInput(response, session, outputDone)
}

func (e *Executor) processOutput(reader *bufio.Reader, session *models.ProgramSession, outputDone chan struct{}) {
	defer close(outputDone)
	defer close(session.OutputChan)

	for {
		header := make([]byte, 8)
		_, err := reader.Read(header)
		if err != nil {
			if err != io.EOF {
				session.OutputChan <- models.ProgramOutput{
					Error:           fmt.Sprintf("error reading output: %v", err),
					Done:            true,
					WaitingForInput: false,
				}
			} else {
				session.OutputChan <- models.ProgramOutput{
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
			session.OutputChan <- models.ProgramOutput{
				Error:           fmt.Sprintf("error reading content: %v", err),
				Done:            true,
				WaitingForInput: false,
			}
			return
		}

		outputStr := string(content)
		isInput := isWaitingForInput(outputStr, session.DetectedInputOps)

		output := models.ProgramOutput{
			Output:          outputStr,
			WaitingForInput: isInput,
		}
		if streamType == 2 { // stderr
			output.Error = outputStr
			output.Output = ""
			output.WaitingForInput = false
		}

		select {
		case <-session.Done:
			return
		case session.OutputChan <- output:
		}
	}
}

func (e *Executor) processInput(response types.HijackedResponse, session *models.ProgramSession, outputDone chan struct{}) error {
	for {
		select {
		case input, ok := <-session.InputChan:
			if !ok {
				return nil
			}
			if _, err := fmt.Fprintln(response.Conn, input); err != nil {
				return fmt.Errorf("failed to write input: %v", err)
			}
		case <-session.Done:
			return nil
		case <-outputDone:
			return nil
		}
	}
}

// Helper functions
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

func isWaitingForInput(output string, detectedOps []models.InputOperation) bool {
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
