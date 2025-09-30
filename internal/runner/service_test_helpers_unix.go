//go:build darwin || linux || freebsd

package runner

import (
	"context"
	"os/exec"
	"syscall"
)

// createTestCommand creates a test command that can be gracefully killed
func createTestCommand(ctx context.Context, duration string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep "+duration) // #nosec G204
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// createUnkillableTestCommand creates a test command that ignores SIGTERM
func createUnkillableTestCommand(ctx context.Context, duration string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "trap '' TERM; sleep "+duration) // #nosec G204
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// getLongRunningCommand returns a shell command that runs indefinitely
func getLongRunningCommand() string {
	return "sh -c 'while true; do sleep 1; done'"
}

// getSimpleSleepCommand returns a simple sleep command for testing
func getSimpleSleepCommand() string {
	return "sleep 0.1"
}
