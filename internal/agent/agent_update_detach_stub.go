//go:build !darwin

package agent

import "os/exec"

func prepareDetachedDarwinUpdaterCommand(_ *exec.Cmd) {}
