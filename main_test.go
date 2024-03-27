package main

import (
	"testing"
)

func TestRunCode_ValidGoCode(t *testing.T) {
	code := `package main

import "fmt"

func main() {
	fmt.Println("Hello, world!")
}`
	_, err := runCode(code)
	if err != nil {
		t.Errorf("runCode failed with valid Go code: %v", err)
	}
}

func TestRunCode_InvalidGoCode(t *testing.T) {
	code := `package main

import "os/exec"

func main() {
	// This should fail validation
}`
	_, err := runCode(code)
	if err == nil {
		t.Error("runCode should fail with invalid Go code using os/exec import")
	}
}

func TestRunCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "Valid Go Code",
			code:    `package main; import "fmt"; func main() { fmt.Println("Hello, World!") }`,
			wantErr: false,
		},
		{
			name:    "Invalid Go Code",
			code:    `package main; import "fmt"; func main() { fmt.Println("Hello, World!")`,
			wantErr: true,
		},
		{
			name:    "Empty Code",
			code:    ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCode(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("runCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
