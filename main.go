package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlexandruC0909/playground/templates"
)

func main() {
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

	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		code := r.FormValue("code")
		output, err := runCode(code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, output)
	})
	http.HandleFunc("/robots.txt", robotsHandler)

	http.ListenAndServe(":8080", nil)
}

func runCode(code string) (string, error) {
	if !validateGoCode(code) {
		return "", fmt.Errorf("invalid Go code")
	}

	tempDir, err := os.MkdirTemp("", "goplayground")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		return "", err
	}
	if len(files) != 0 {
		return "", fmt.Errorf("temporary directory is not empty")
	}

	tempFile := filepath.Join(tempDir, "main.go")
	err = os.WriteFile(tempFile, []byte(code), 0600)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "run", tempFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		errorMessage := parseGoError(stderr.String())
		if errorMessage != "" {
			return "", fmt.Errorf("%s\nError: error running code: %v", errorMessage, err)
		}
		return "", nil
	} else {
		return stdout.String(), nil
	}
}

func validateGoCode(code string) bool {
	disallowed := []string{
		`import\s+"os/exec"`,
		`import\s+"net/http"`,
		`\bos\.Exec\b`,
		`\bos\.Setenv\b`,
		`\bfile\.Open\b`,
	}

	for _, pattern := range disallowed {
		match, _ := regexp.MatchString(pattern, code)
		if match {
			return false
		}
	}

	return true
}

func parseGoError(stderr string) string {
	re := regexp.MustCompile(`(?m)^\S+:(\d+):(\d+):\s+(.*)$`)
	lines := strings.Split(stderr, "\n")
	for _, line := range lines {
		match := re.FindStringSubmatch(line)
		if len(match) > 0 {
			return strings.TrimSpace(match[len(match)-1])
		}
	}
	return ""
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	robotsTxt := []byte("User-agent: *\nDisallow: /private/")
	w.Header().Set("Content-Type", "text/plain")
	w.Write(robotsTxt)
}
