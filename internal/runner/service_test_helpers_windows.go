//go:build windows

package runner

import (
	"context"
	"os/exec"
	"syscall"
)

// createTestCommand creates a test command that can be gracefully killed
func createTestCommand(ctx context.Context, duration string) *exec.Cmd {
	// Windows: use ping for delay (timeout command fails in non-interactive CI)
	// ping -n <duration+1> gives roughly <duration> seconds of delay
	cmd := exec.CommandContext(ctx, "cmd.exe", "/c", "ping -n "+duration+" 127.0.0.1 >nul")
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
	// Windows: infinite loop with ping delay (timeout command fails in non-interactive CI)
	return "cmd.exe /c :loop & ping -n 2 127.0.0.1 >nul & goto loop"
}

// getSimpleSleepCommand returns a simple sleep command for testing
func getSimpleSleepCommand() string {
	// Windows: use ping for ~1 second delay (timeout command fails in non-interactive CI)
	return "cmd.exe /c ping -n 2 127.0.0.1 >nul"
}

// getMediumSleepCommand returns a medium duration sleep for integration tests
func getMediumSleepCommand() string {
	// Windows: use ping for ~2 second delay (timeout command fails in non-interactive CI)
	return "cmd.exe /c ping -n 3 127.0.0.1 >nul"
}
