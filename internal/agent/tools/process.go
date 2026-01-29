package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ProcessManager manages background processes
type ProcessManager struct {
	mu        sync.RWMutex
	processes map[string]*ManagedProcess
	workDir   string
}

// ManagedProcess represents a background process
type ManagedProcess struct {
	handle    string
	cmd       *exec.Cmd
	stdout    *RingBuffer
	stderr    *RingBuffer
	startTime time.Time
	done      chan struct{}
	err       error
}

// RingBuffer keeps the last N lines
type RingBuffer struct {
	mu    sync.Mutex
	lines []string
	size  int
}

// NewRingBuffer creates a new ring buffer
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{lines: make([]string, 0, size), size: size}
}

// Write adds a line to the buffer
func (rb *RingBuffer) Write(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if len(rb.lines) >= rb.size {
		rb.lines = rb.lines[1:]
	}
	rb.lines = append(rb.lines, line)
}

// Lines returns the last n lines
func (rb *RingBuffer) Lines(n int) []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if n > len(rb.lines) {
		n = len(rb.lines)
	}
	if n <= 0 {
		return []string{}
	}
	result := make([]string, n)
	copy(result, rb.lines[len(rb.lines)-n:])
	return result
}

// All returns all lines
func (rb *RingBuffer) All() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	result := make([]string, len(rb.lines))
	copy(result, rb.lines)
	return result
}

// NewProcessManager creates a new ProcessManager.
// Agent commands are not sandboxed - we rely on validateCommandSafety() for protection.
// Replay sandboxing (to detect uninstrumented packages) is handled by runner.createReplayFenceConfig().
func NewProcessManager(workDir string) *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*ManagedProcess),
		workDir:   workDir,
	}
}

// ProcessTools provides process management operations
type ProcessTools struct {
	pm      *ProcessManager
	workDir string
}

// NewProcessTools creates a new ProcessTools instance
func NewProcessTools(pm *ProcessManager, workDir string) *ProcessTools {
	return &ProcessTools{pm: pm, workDir: workDir}
}

// validateCommandSafety checks if a command is safe to execute
// Returns an error if the command is potentially dangerous
func validateCommandSafety(command string) error {
	cmdLower := strings.ToLower(command)

	// Dangerous command patterns - these could cause data loss or system damage
	dangerousPatterns := []struct {
		pattern string
		reason  string
	}{
		// verifySetupPhase() need to run `rm -rf .tusk/traces/*` to delete existing traces
		// {"rm -rf", "recursive forced deletion"},
		{"rm -fr", "recursive forced deletion"},
		{"rmdir", "directory deletion"},
		{"> /dev/", "writing to device files"},
		{"dd if=", "low-level disk operations"},
		{"mkfs", "filesystem formatting"},
		{"fdisk", "disk partitioning"},
		{"format ", "disk formatting"},
		{":(){:|:&};:", "fork bomb"},
		{"chmod -r 777", "recursive permission change"},
		{"chmod -r 000", "recursive permission change"},
		{"chown -r", "recursive ownership change"},
		{"wget ", "downloading files (use http_request instead)"},
		{"curl ", "downloading files (use http_request instead)"},
		{"sudo ", "elevated privileges not allowed"},
		{"su ", "user switching not allowed"},
		{"shutdown", "system shutdown"},
		{"reboot", "system reboot"},
		{"init ", "init system commands"},
		{"systemctl", "systemd commands"},
		// {"kill -9", "force killing processes"},
		{"killall", "killing multiple processes"},
		// {"pkill", "killing processes by name"},
		{"mv /", "moving root files"},
		{"cp /etc/passwd", "copying sensitive files"},
		{"cat /etc/shadow", "reading sensitive files"},
		{"> /etc/", "writing to system config"},
		{"export path=", "modifying PATH"},
		{"unset path", "unsetting PATH"},
		{"npm publish", "publishing packages"},
		{"yarn publish", "publishing packages"},
		{"git push", "pushing to remote"},
		{"git commit", "committing changes (agent shouldn't modify git history)"},
		{"docker rm", "removing containers"},
		{"docker rmi", "removing images"},
		{"docker system prune", "pruning docker"},
	}

	for _, dp := range dangerousPatterns {
		if strings.Contains(cmdLower, dp.pattern) {
			return fmt.Errorf("command blocked for safety: '%s' contains '%s' (%s). This command is not allowed during setup", command, dp.pattern, dp.reason)
		}
	}

	// Check for redirection that overwrites important files
	if strings.Contains(command, ">") && !strings.Contains(command, ">>") {
		// Allow redirecting to .tusk directory
		if !strings.Contains(command, ".tusk/") && !strings.Contains(command, "/tmp/") {
			// Check if it's overwriting something outside project
			if strings.Contains(command, "> /") || strings.Contains(command, "> ~/") {
				return fmt.Errorf("command blocked: redirecting output to system paths is not allowed")
			}
		}
	}

	return nil
}

// RunCommand executes a command and waits for completion
func (pt *ProcessTools) RunCommand(input json.RawMessage) (string, error) {
	var params struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if err := validateCommandSafety(params.Command); err != nil {
		return "", err
	}

	// Check if this looks like a server start command - should use start_background_process instead
	serverPatterns := []string{"npm run dev", "npm start", "yarn dev", "yarn start", "pnpm dev", "pnpm start", "node server", "nodemon"}
	cmdLower := strings.ToLower(params.Command)
	for _, pattern := range serverPatterns {
		if strings.Contains(cmdLower, pattern) {
			return "", fmt.Errorf("command '%s' looks like a server start command. Use start_background_process instead of run_command for long-running processes", params.Command)
		}
	}

	timeout := 120 * time.Second
	if params.TimeoutSeconds > 0 {
		timeout = time.Duration(params.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", params.Command) //nolint:gosec // Command is validated by validateCommandSafety
	cmd.Dir = pt.workDir
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %v. If this is a long-running process like a server, use start_background_process instead", timeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return "", fmt.Errorf("failed to run command: %w", err)
		}
	}

	result := string(output)
	// Truncate if too long
	maxLen := 50000
	if len(result) > maxLen {
		result = result[:maxLen] + "\n\n... (output truncated)"
	}

	return fmt.Sprintf("Exit code: %d\n\nOutput:\n%s", exitCode, result), nil
}

// StartBackground starts a process in the background.
// Note: This is NOT sandboxed because it's used for:
// - "Confirm App Starts" phase (needs real DB connections)
// - "Record" mode (needs real outbound calls to capture behavior)
// Replay mode sandboxing is handled by the runner package, not here.
func (pt *ProcessTools) StartBackground(input json.RawMessage) (string, error) {
	var params struct {
		Command string            `json:"command"`
		Env     map[string]string `json:"env"`
		Port    int               `json:"port"` // Optional: port the server will listen on
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if err := validateCommandSafety(params.Command); err != nil {
		return "", err
	}

	handle := "proc_" + uuid.New().String()[:8]

	cmd := exec.Command("sh", "-c", params.Command) //nolint:gosec // Command is validated by validateCommandSafety
	cmd.Dir = pt.workDir

	env := os.Environ()
	for k, v := range params.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Start process in its own process group so we can kill all children
	setSysProcAttr(cmd)

	// Capture output
	stdout := NewRingBuffer(1000)
	stderr := NewRingBuffer(1000)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	mp := &ManagedProcess{
		handle:    handle,
		cmd:       cmd,
		stdout:    stdout,
		stderr:    stderr,
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start process: %w", err)
	}

	// Stream output to ring buffers
	go streamToBuffer(stdoutPipe, stdout)
	go streamToBuffer(stderrPipe, stderr)

	// Monitor process
	go func() {
		mp.err = cmd.Wait()
		close(mp.done)
	}()

	pt.pm.mu.Lock()
	pt.pm.processes[handle] = mp
	pt.pm.mu.Unlock()

	// Wait a moment to check for immediate failure
	select {
	case <-mp.done:
		logs := strings.Join(stderr.All(), "\n")
		stdoutLogs := strings.Join(stdout.All(), "\n")
		return "", fmt.Errorf("process exited immediately: %v\nStderr:\n%s\nStdout:\n%s", mp.err, logs, stdoutLogs)
	case <-time.After(1 * time.Second):
		return fmt.Sprintf("Started background process with handle: %s (PID: %d)\nCommand: %s",
			handle, cmd.Process.Pid, params.Command), nil
	}
}

// StopBackground stops a background process
func (pt *ProcessTools) StopBackground(input json.RawMessage) (string, error) {
	var params struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	pt.pm.mu.Lock()
	mp, ok := pt.pm.processes[params.Handle]
	if !ok {
		pt.pm.mu.Unlock()
		return "", fmt.Errorf("no process with handle: %s", params.Handle)
	}
	delete(pt.pm.processes, params.Handle)
	pt.pm.mu.Unlock()

	killProcessGroup(mp)

	return fmt.Sprintf("Stopped process %s", params.Handle), nil
}

// GetLogs returns recent logs from a background process
func (pt *ProcessTools) GetLogs(input json.RawMessage) (string, error) {
	var params struct {
		Handle string `json:"handle"`
		Lines  int    `json:"lines"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if params.Lines == 0 {
		params.Lines = 100
	}

	pt.pm.mu.RLock()
	mp, ok := pt.pm.processes[params.Handle]
	pt.pm.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("no process with handle: %s", params.Handle)
	}

	stdoutLines := mp.stdout.Lines(params.Lines)
	stderrLines := mp.stderr.Lines(params.Lines)

	// Check if still running
	status := "running"
	select {
	case <-mp.done:
		if mp.err != nil {
			status = fmt.Sprintf("exited with error: %v", mp.err)
		} else {
			status = "exited successfully"
		}
	default:
	}

	return fmt.Sprintf("Status: %s\nRunning for: %s\n\n=== STDOUT (last %d lines) ===\n%s\n\n=== STDERR (last %d lines) ===\n%s",
		status,
		time.Since(mp.startTime).Round(time.Second),
		len(stdoutLines),
		strings.Join(stdoutLines, "\n"),
		len(stderrLines),
		strings.Join(stderrLines, "\n"),
	), nil
}

// WaitForReady polls a URL until it responds successfully
func (pt *ProcessTools) WaitForReady(input json.RawMessage) (string, error) {
	var params struct {
		URL             string `json:"url"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
		IntervalSeconds int    `json:"interval_seconds"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	timeout := 30 * time.Second
	interval := 1 * time.Second
	if params.TimeoutSeconds > 0 {
		timeout = time.Duration(params.TimeoutSeconds) * time.Second
	}
	if params.IntervalSeconds > 0 {
		interval = time.Duration(params.IntervalSeconds) * time.Second
	}

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	deadline := start.Add(timeout)
	attempts := 0

	for time.Now().Before(deadline) {
		attempts++
		resp, err := client.Get(params.URL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return fmt.Sprintf("Service ready after %d attempts (%.1fs). Status: %d",
					attempts, time.Since(start).Seconds(), resp.StatusCode), nil
			}
		}
		time.Sleep(interval)
	}

	return "", fmt.Errorf("service not ready after %d attempts over %.0fs", attempts, timeout.Seconds())
}

// StopAll stops all managed processes and cleans up resources
func (pm *ProcessManager) StopAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for handle, mp := range pm.processes {
		killProcessGroupImmediate(mp)
		delete(pm.processes, handle)
	}
}

func streamToBuffer(r io.Reader, buf *RingBuffer) {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024
	scanBuf := make([]byte, maxCapacity)
	scanner.Buffer(scanBuf, maxCapacity)

	for scanner.Scan() {
		buf.Write(scanner.Text())
	}
}
