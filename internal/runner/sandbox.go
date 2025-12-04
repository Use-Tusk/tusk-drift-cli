package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// SandboxConfig mirrors the sandbox-runtime configuration structure
type SandboxConfig struct {
	Network    NetworkConfig    `json:"network"`
	Filesystem FilesystemConfig `json:"filesystem"`
}

// NetworkConfig mirrors the sandbox-runtime network configuration structure
type NetworkConfig struct {
	AllowedDomains    []string `json:"allowedDomains"`
	DeniedDomains     []string `json:"deniedDomains"`
	AllowUnixSockets  []string `json:"allowUnixSockets,omitempty"`
	AllowLocalBinding bool     `json:"allowLocalBinding,omitempty"`
}

// FilesystemConfig mirrors the sandbox-runtime filesystem configuration structure
type FilesystemConfig struct {
	DenyRead   []string `json:"denyRead"`
	AllowWrite []string `json:"allowWrite"`
	DenyWrite  []string `json:"denyWrite"`
}

// SandboxOptions configures the sandbox wrapper
type SandboxOptions struct {
	Enabled        bool
	SocketPath     string   // Unix socket for CLI<->SDK communication
	ServicePort    int      // Port the service binds to
	WorkDir        string   // Working directory for the service
	AllowedDomains []string // Optional: domains to allow (empty = block all external)
}

// WrapCommandWithSandbox wraps a command with sandbox-runtime restrictions
func WrapCommandWithSandbox(command string, opts SandboxOptions) (string, string, error) {
	if !opts.Enabled {
		return command, "", nil
	}

	if _, err := exec.LookPath("srt"); err != nil {
		return "", "", fmt.Errorf("sandbox-runtime (srt) not found in PATH: %w", err)
	}

	config := SandboxConfig{
		Network: NetworkConfig{
			// undefined -> all allowed
			// empty array -> all block, localhost via allowLocalBinding (SeatBelt level), doesn't start proxy
			// e.g. ["github.com"] -> spins up proxy, allows github.com
			AllowedDomains:    []string{},
			DeniedDomains:     []string{},
			AllowLocalBinding: true, // Allow service to bind to port
		},
		Filesystem: FilesystemConfig{
			DenyRead: []string{},
			AllowWrite: []string{
				opts.WorkDir,   // Working directory (absolute path)
				"/tmp",         // Temp directory
				"/private/tmp", // macOS actual temp path
				"/var/folders", // macOS per-user temp
				".tusk",        // Tusk config/logs directory
			},
			DenyWrite: []string{
				".env",
			},
		},
	}

	// Add Unix socket if using socket communication
	if opts.SocketPath != "" {
		config.Network.AllowUnixSockets = []string{opts.SocketPath}
	}

	// Write config to temp file
	configFile, err := os.CreateTemp("", "srt-config-*.json")
	if err != nil {
		return "", "", fmt.Errorf("failed to create sandbox config file: %w", err)
	}

	if err := json.NewEncoder(configFile).Encode(config); err != nil {
		_ = configFile.Close()
		_ = os.Remove(configFile.Name())
		return "", "", fmt.Errorf("failed to write sandbox config: %w", err)
	}
	_ = configFile.Close()

	wrappedCommand := fmt.Sprintf("srt --settings %s %s", configFile.Name(), command)

	return wrappedCommand, configFile.Name(), nil
}

// IsSandboxAvailable checks if sandbox-runtime is installed and platform is supported
func IsSandboxAvailable() bool {
	if _, err := exec.LookPath("srt"); err != nil {
		return false
	}

	switch os.Getenv("GOOS") {
	case "darwin", "linux", "":
		return true
	default:
		return false
	}
}
