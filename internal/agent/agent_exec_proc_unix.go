//go:build !windows

package agent

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
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

// commandDescendantPIDs snapshots the other members of the command's process
// group. Taking this snapshot before the initial interrupt lets cancellation
// distinguish the original workload from cleanup commands a shell trap starts.
func commandDescendantPIDs(cmd *exec.Cmd) ([]int, error) {
	pid := commandPID(cmd)
	if pid <= 0 {
		return nil, nil
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil, nil
		}
		return nil, err
	}
	out, err := exec.Command("ps", "-A", "-o", "pid=", "-o", "pgid=").Output()
	if err != nil {
		return nil, err
	}

	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		memberPID, pidErr := strconv.Atoi(fields[0])
		memberPGID, pgidErr := strconv.Atoi(fields[1])
		if pidErr != nil || pgidErr != nil || memberPID == pid || memberPGID != pgid {
			continue
		}
		pids = append(pids, memberPID)
	}
	return pids, nil
}

// killCommandDescendants forcibly stops the snapshotted workload while leaving
// the process-group leader and any newly started cleanup commands alive.
func killCommandDescendants(cmd *exec.Cmd, pids []int) (bool, error) {
	pid := commandPID(cmd)
	if pid <= 0 || len(pids) == 0 {
		return false, nil
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		return false, err
	}

	killed := false
	var killErrs []error
	for _, memberPID := range pids {
		if memberPID <= 0 || memberPID == pid {
			continue
		}
		memberPGID, err := syscall.Getpgid(memberPID)
		if err != nil {
			if !errors.Is(err, syscall.ESRCH) {
				killErrs = append(killErrs, err)
			}
			continue
		}
		if memberPGID != pgid {
			continue
		}
		if err := syscall.Kill(memberPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			killErrs = append(killErrs, err)
			continue
		}
		killed = true
	}
	return killed, errors.Join(killErrs...)
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
