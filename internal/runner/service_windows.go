//go:build windows

package runner

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
)

// createServiceCommand creates a shell command for Windows systems
func createServiceCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/c", command) // #nosec G204
}

// createReadinessCommand creates a shell command for Windows systems
func createReadinessCommand(command string) *exec.Cmd {
	return exec.Command("cmd.exe", "/c", command) // #nosec G204
}

// setupProcessGroup configures the command for Windows process management
func setupProcessGroup(cmd *exec.Cmd) {
	// On Windows, we set CREATE_NEW_PROCESS_GROUP flag
	// This allows us to send Ctrl+Break signals to the process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup attempts to kill the process gracefully, then forcefully
func killProcessGroup(cmd *exec.Cmd, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	log.Debug("Stopping service", "pid", pid)

	// On Windows, try to interrupt the process first
	// Note: On Windows, Process.Signal doesn't work the same way as Unix
	// We need to use taskkill or Process.Kill()

	// First, try graceful termination using taskkill
	killCmd := exec.Command("taskkill", "/T", "/PID", fmt.Sprintf("%d", pid))
	if err := killCmd.Run(); err != nil {
		log.Debug("Failed to gracefully terminate process tree", "pid", pid, "error", err)
	}

	// Wait for the process to exit gracefully
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		log.Debug("Service stopped gracefully")
		return nil
	case <-time.After(timeout):
		log.Debug("Service didn't stop gracefully, force killing")
		// Force kill the entire process tree
		forceKillCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
		if err := forceKillCmd.Run(); err != nil {
			log.Debug("Failed to force kill process tree", "pid", pid, "error", err)
			// Last resort: use Process.Kill()
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return fmt.Errorf("service was force killed after timeout")
	}
}
