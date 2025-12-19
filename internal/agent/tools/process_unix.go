//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
	"time"
)

// setSysProcAttr sets Unix-specific process attributes to create a new process group
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills all processes in the process group
func killProcessGroup(mp *ManagedProcess) {
	if mp.cmd.Process == nil {
		return
	}
	pgid := mp.cmd.Process.Pid
	// Send SIGTERM to the process group, then SIGKILL if it doesn't exit
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	select {
	case <-mp.done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-mp.done
	}
}

// killProcessGroupImmediate kills the process group with a shorter timeout (used in StopAll)
func killProcessGroupImmediate(mp *ManagedProcess) {
	if mp.cmd.Process == nil {
		return
	}
	pgid := mp.cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	select {
	case <-mp.done:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}
