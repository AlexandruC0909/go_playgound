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
        // Dangerous imports
        `import\s+"os/exec"`,
        `import\s+"net/http"`,
        `import\s+"syscall"`,
        `import\s+"unsafe"`,
        `import\s+"debug/.*"`,
        `import\s+"plugin"`,
        `import\s+"runtime/debug"`,
        `import\s+"path/filepath"`,
        `import\s+"io/ioutil"`,
        
        // Dangerous package usage
        `\bos\.(?:Create|Remove|Chmod|OpenFile|Setenv|Exit|MkdirAll|RemoveAll|Executable)\b`,
        `\bsyscall\.\w+\b`,
        `\bunsafe\.\w+\b`,
        `\bexec\.\w+\b`,
        `\bnet\.\w+\b`,
        `\bdebug\.\w+\b`,
        `\bplugin\.\w+\b`,
        
        // Potentially dangerous operations
        `\bmake\(\w+,\s*\d+[^)]*\)`,  // Catch potentially large slice allocations
        `\bnew\([^)]+\)\s*\[\d+\]`,   // Catch potentially large array allocations
        `for\s*\{(?:[^}]*\n){20,}\}`, // Detect potentially infinite or very long loops
        
        // File operations
        `\bos\.(Open|Create|OpenFile|Write|Remove)\b`,
        `\bioutil\.\w+\b`,
        
        // Command execution
        `\bexec\.Command\b`,
        `\bgo\s+func\b`,              // Prevent goroutine spawning
        
        // Memory operations
        `\(*\[\]byte\)\(`,
        `\buintptr\b`,
        
        // Network operations
        `\bdial\b`,
        `\blisten\b`,
    }
)
