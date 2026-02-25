//go:build !windows

package agent

import "os/exec"

func prepareDetachedWindowsRestartCommand(_ *exec.Cmd) {}
