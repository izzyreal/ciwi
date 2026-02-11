package updatehelper

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type multiArg []string

func (m *multiArg) String() string { return strings.Join(*m, ",") }
func (m *multiArg) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func Run(args []string) error {
	fs := flag.NewFlagSet("update-helper", flag.ContinueOnError)
	var target string
	var newPath string
	var pid int
	var restartArgs multiArg
	fs.StringVar(&target, "target", "", "target executable path")
	fs.StringVar(&newPath, "new", "", "new binary path")
	fs.IntVar(&pid, "pid", 0, "parent pid")
	fs.Var(&restartArgs, "arg", "restart arg")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(target) == "" || strings.TrimSpace(newPath) == "" || pid <= 0 {
		return fmt.Errorf("update-helper requires --target, --new and --pid")
	}

	if err := waitForParentExit(pid, 45*time.Second); err != nil {
		return fmt.Errorf("wait for parent exit: %w", err)
	}

	bak := target + ".bak"
	_ = os.Remove(bak)

	if err := os.Rename(target, bak); err != nil {
		return fmt.Errorf("move current binary to backup: %w", err)
	}
	if err := os.Rename(newPath, target); err != nil {
		_ = os.Rename(bak, target)
		return fmt.Errorf("move new binary into place: %w", err)
	}
	_ = os.Chmod(target, 0o755)

	if err := startRelaunched(target, restartArgs); err != nil {
		_ = os.Rename(target, newPath)
		_ = os.Rename(bak, target)
		return fmt.Errorf("restart updated binary: %w", err)
	}
	return nil
}

func waitForParentExit(pid int, timeout time.Duration) error {
	if runtime.GOOS == "windows" {
		time.Sleep(2 * time.Second)
		return nil
	}
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for pid %d", pid)
		}
		running, err := processRunning(pid)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func processRunning(pid int) (bool, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "process already finished") {
		return false, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such process") {
		return false, nil
	}
	return false, nil
}

func startRelaunched(target string, args []string) error {
	absTarget, _ := filepath.Abs(target)
	cmd := exec.Command(absTarget, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
