//go:build windows

package agent

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func prepareCommandForCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	// CREATE_NEW_PROCESS_GROUP allows softer console-control attempts later.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func interruptCommandTree(cmd *exec.Cmd) error {
	pid := commandPID(cmd)
	if pid <= 0 {
		return nil
	}
	// Best-effort graceful tree termination first (without /F).
	taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T")
	if out, err := taskkill.CombinedOutput(); err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("taskkill /PID %d /T: %w (%s)", pid, err, text)
	}
	return nil
}

func killCommandTree(cmd *exec.Cmd) error {
	pid := commandPID(cmd)
	if pid <= 0 {
		return nil
	}
	taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	_, err := taskkill.CombinedOutput()
	return err
}

func commandPID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	return cmd.Process.Pid
}
