package utils

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexandruC0909/playground/internal/config"
	"github.com/AlexandruC0909/playground/internal/models"
	"golang.org/x/time/rate"
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

func CheckRateLimit(rateLimiter *RateLimiter, ip string) error {
	limiter := rateLimiter.getLimiter(ip)
	if !limiter.Allow() {
		return fmt.Errorf("rate limit exceeded")
	}
	return nil
}

func ValidateAndPrepare(code string, session *models.ProgramSession) error {
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

func validateGoCode(code string) bool {
	for _, pattern := range config.DisallowedPatterns {
		match, _ := regexp.MatchString(pattern, code)
		if match {
			return false
		}
	}

	if strings.Count(code, "func") > 50 {
		return false
	}

	if strings.Count(code, "for") > 30 {
		return false
	}

	return true
}

func ExtractIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		return strings.Split(forwardedFor, ",")[0]
	}
	return r.RemoteAddr
}

func ParseRequestBody(r *http.Request) (struct {
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

func CleanupPreviousSession(activeSessions *sync.Map, r *http.Request) {
	if oldSessionIDStr := r.Header.Get("X-Previous-Session"); oldSessionIDStr != "" {
		if oldSessionID, err := strconv.ParseUint(oldSessionIDStr, 10, 64); err == nil {
			if oldSession, ok := activeSessions.Load(oldSessionID); ok {
				oldSession.(*models.ProgramSession).Close()
				activeSessions.Delete(oldSessionID)
			}
		}
	}
}

func LogTiming(operation string, start time.Time) {
	log.Printf("%s took: %v\n", operation, time.Since(start))
}

func SendError(session *models.ProgramSession, errMsg string) {
	session.OutputChan <- models.ProgramOutput{
		Error: errMsg,
		Done:  true,
	}
}
