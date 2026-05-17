//go:build darwin

package agent

import (
	"os/exec"
	"syscall"
)

func prepareDetachedDarwinUpdaterCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
