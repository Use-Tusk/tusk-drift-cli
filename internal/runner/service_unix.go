//go:build darwin || linux || freebsd

package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"
	"time"
)

// createServiceCommand creates a shell command for Unix systems
func createServiceCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", command) // #nosec G204
}

// createReadinessCommand creates a shell command for Unix systems
func createReadinessCommand(command string) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", command) // #nosec G204
}

// setupProcessGroup configures the command to run in its own process group (Unix)
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessGroup attempts to kill the process group gracefully, then forcefully
func killProcessGroup(cmd *exec.Cmd, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	slog.Debug("Stopping service", "pid", pid)

	// Try to get the process group ID
	pgid, err := syscall.Getpgid(pid)
	if err == nil {
		// Send SIGTERM to the entire process group
		slog.Debug("Killing process group", "pgid", pgid)
		if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
			slog.Debug("Failed to send SIGTERM to process group", "pgid", pgid, "error", err)
			// Fallback to interrupting the main process
			if err := cmd.Process.Signal(syscall.Signal(syscall.SIGINT)); err != nil {
				slog.Debug("Failed to send interrupt signal", "pid", pid, "error", err)
			}
		}
	} else {
		// If we can't get the process group, just interrupt the main process
		slog.Debug("Failed to get process group", "pid", pid, "error", err)
		if err := cmd.Process.Signal(syscall.Signal(syscall.SIGINT)); err != nil {
			slog.Debug("Failed to send interrupt signal", "pid", pid, "error", err)
		}
	}

	// Wait for the process to exit gracefully
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		slog.Debug("Service stopped gracefully")
		return nil
	case <-time.After(timeout):
		slog.Debug("Service didn't stop gracefully, force killing")
		// Force kill the process group
		if pgid, err := syscall.Getpgid(pid); err == nil {
			slog.Debug("Force killing process group", "pgid", pgid)
			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
				slog.Debug("Failed to SIGKILL process group", "pgid", pgid, "error", err)
				// Last resort: kill the main process
				_ = cmd.Process.Kill()
			}
		} else {
			slog.Debug("Final fallback: killing main process", "pid", pid)
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return fmt.Errorf("service was force killed after timeout")
	}
}
