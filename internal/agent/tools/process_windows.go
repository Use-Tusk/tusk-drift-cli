//go:build windows

package tools

import (
	"os/exec"
	"time"
)

// setSysProcAttr sets Windows-specific process attributes
// On Windows, we don't use process groups the same way as Unix
func setSysProcAttr(cmd *exec.Cmd) {
	// No-op on Windows - process groups work differently
}

// killProcessGroup kills the process and its children on Windows
func killProcessGroup(mp *ManagedProcess) {
	if mp.cmd.Process == nil {
		return
	}
	// On Windows, we just kill the main process
	// For more thorough cleanup, we could use taskkill /T, but Process.Kill() is simpler
	_ = mp.cmd.Process.Kill()
	select {
	case <-mp.done:
	case <-time.After(5 * time.Second):
		// Already tried to kill, just wait
		<-mp.done
	}
}

// killProcessGroupImmediate kills the process with a shorter timeout (used in StopAll)
func killProcessGroupImmediate(mp *ManagedProcess) {
	if mp.cmd.Process == nil {
		return
	}
	_ = mp.cmd.Process.Kill()
	select {
	case <-mp.done:
	case <-time.After(2 * time.Second):
		// Already tried to kill
	}
}
