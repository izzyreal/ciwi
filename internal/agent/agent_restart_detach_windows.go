//go:build windows

package agent

import (
	"os/exec"
	"syscall"
)

const windowsCreationFlagDetachedProcess = 0x00000008

func prepareDetachedWindowsRestartCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windowsCreationFlagDetachedProcess,
	}
}
