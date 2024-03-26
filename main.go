package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Render the code editor and output area
		fmt.Fprintln(w, `<!DOCTYPE html>
		<html lang="en">
		<head>
			<meta charset="UTF-8">
			<meta name="viewport" content="width=device-width, initial-scale=1.0">
			<title>Go Playground</title>
		</head>
		<body>
			<textarea id="code" rows="10" cols="50">package main
		
		import "fmt"
		
		func main() {
			fmt.Println("Hello, World!")
		}</textarea>
			<button onclick="runCode()">Run</button>
			<pre id="output"></pre>
		
			<script>
				function runCode() {
					var code = document.getElementById("code").value;
		
					var xhr = new XMLHttpRequest();
					xhr.open("POST", "/run", true);
					xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
					xhr.onreadystatechange = function() {
						if (xhr.readyState === XMLHttpRequest.DONE) {
							if (xhr.status === 200) {
								document.getElementById("output").textContent = xhr.responseText;
							} else {
								document.getElementById("output").textContent = "Error: " + xhr.responseText;
							}
						}
					};
					xhr.send("code=" + encodeURIComponent(code));
				}
			</script>
		</body>
		</html>
		`)
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

	http.ListenAndServe(":8080", nil)
}

func runCode(code string) (string, error) {
	tempDir, err := ioutil.TempDir("", "goplayground")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "main.go")
	err = ioutil.WriteFile(tempFile, []byte(code), 0644)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "run", tempFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running code: %v\n%s", err, stderr.String())
	}

	return stdout.String(), nil
}
