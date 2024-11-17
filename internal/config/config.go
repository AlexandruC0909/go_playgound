package config

const (
	// Server configuration
	ServerPort = "8088"

	// Docker configuration
	DockerImage    = "golang:1.22-alpine"
	ContainerName  = "go-playground"
	TimeoutSeconds = 100
	MemoryLimit    = 150 * 1024 * 1024

	// Program limits
	MaxCodeSize   = 1024 * 1024
	MaxOutputSize = 1024 * 1024

	// Rate limiting
	RequestsPerHour   = 1000
	RequestsPerMinute = 500
)

var (
	// Security configuration
	DisallowedPatterns = []string{
		`import\s+"os/exec"`,
		`import\s+"net/http"`,
		`import\s+"syscall"`,
		`import\s+"unsafe"`,
		`import\s+"debug/.*"`,
		`import\s+"plugin"`,
		`import\s+"runtime/debug"`,
		`\bos\.Exec\b`,
		`\bos\.Setenv\b`,
		`\bos\.Remove\b`,
		`\bos\.Chmod\b`,
		`\bfile\.\w+\b`,
		`\bsyscall\.\w+\b`,
		`\bunsafe\.\w+\b`,
		`\bexec\.\w+\b`,
		`\bnet\.\w+\b`,
		`\bdebug\.\w+\b`,
		`\bplugin\.\w+\b`,
		`\bmake\(\w+,\s*\d+\)`,
	}
)
