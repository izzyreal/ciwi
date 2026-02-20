//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

func prepareCommandForCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interruptCommandTree(cmd *exec.Cmd) error {
	pid := commandPID(cmd)
	if pid <= 0 {
		return nil
	}
	// Negative pid signals the whole process group.
	return syscall.Kill(-pid, syscall.SIGINT)
}

func killCommandTree(cmd *exec.Cmd) error {
	pid := commandPID(cmd)
	if pid <= 0 {
		return nil
	}
	// Negative pid signals the whole process group.
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func commandPID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	return cmd.Process.Pid
}
