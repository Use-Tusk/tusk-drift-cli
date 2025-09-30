//go:build windows

package runner

import (
	"context"
	"os/exec"
	"syscall"
)

// createTestCommand creates a test command that can be gracefully killed
func createTestCommand(ctx context.Context, duration string) *exec.Cmd {
	// Windows: use timeout command (similar to sleep, but in seconds)
	cmd := exec.CommandContext(ctx, "cmd.exe", "/c", "timeout /t "+duration+" /nobreak >nul")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd
}

// createUnkillableTestCommand creates a test command that's harder to kill
func createUnkillableTestCommand(ctx context.Context, duration string) *exec.Cmd {
	// On Windows, we'll use a PowerShell loop that's harder to interrupt
	cmd := exec.CommandContext(ctx, "powershell.exe", "-Command",
		"$timeout = "+duration+"; Start-Sleep -Seconds $timeout")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd
}

// getLongRunningCommand returns a shell command that runs indefinitely
func getLongRunningCommand() string {
	// Windows: infinite loop with timeout
	return "cmd.exe /c :loop & timeout /t 1 /nobreak >nul & goto loop"
}

// getSimpleSleepCommand returns a simple sleep command for testing
func getSimpleSleepCommand() string {
	// Windows: 1 second timeout (timeout is in seconds, minimum is 1)
	return "cmd.exe /c timeout /t 1 /nobreak >nul"
}

// getMediumSleepCommand returns a medium duration sleep for integration tests
func getMediumSleepCommand() string {
	// Windows: 1 second is minimum, but this is acceptable for medium duration
	return "cmd.exe /c timeout /t 2 /nobreak >nul"
}
