//go:build !windows

package agent

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestPrepareCommandForCancellationSetsProcessGroup(t *testing.T) {
	prepareCommandForCancellation(nil)

	cmd := exec.Command("sleep", "30")
	prepareCommandForCancellation(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("expected command to run in its own process group")
	}
}

func TestCommandPIDAndTreeSignals(t *testing.T) {
	if got := commandPID(nil); got != 0 {
		t.Fatalf("nil command pid=%d want 0", got)
	}
	unstarted := exec.Command("sleep", "30")
	if got := commandPID(unstarted); got != 0 {
		t.Fatalf("unstarted command pid=%d want 0", got)
	}
	if err := interruptCommandTree(nil); err != nil {
		t.Fatalf("interrupt nil command: %v", err)
	}
	if err := killCommandTree(nil); err != nil {
		t.Fatalf("kill nil command: %v", err)
	}

	t.Run("interrupts running process group", func(t *testing.T) {
		cmd := exec.Command("sleep", "30")
		prepareCommandForCancellation(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start sleep: %v", err)
		}
		defer func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}
		}()

		if got := commandPID(cmd); got <= 0 {
			t.Fatalf("expected running pid, got %d", got)
		}
		if err := interruptCommandTree(cmd); err != nil {
			t.Fatalf("interrupt running process tree: %v", err)
		}
		waitDone := make(chan error, 1)
		go func() { waitDone <- cmd.Wait() }()
		select {
		case err := <-waitDone:
			var exitErr *exec.ExitError
			if err == nil {
				t.Fatalf("expected SIGINT termination")
			}
			if !errors.As(err, &exitErr) {
				t.Fatalf("unexpected wait error: %v", err)
			}
			if status, ok := exitErr.Sys().(syscall.WaitStatus); !ok || status.Signal() != syscall.SIGINT {
				t.Fatalf("expected SIGINT exit, got %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for SIGINT termination")
		}
	})

	t.Run("kills running process group", func(t *testing.T) {
		cmd := exec.Command("sleep", "30")
		prepareCommandForCancellation(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start sleep: %v", err)
		}
		defer func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}
		}()

		if err := killCommandTree(cmd); err != nil {
			t.Fatalf("kill running process tree: %v", err)
		}
		waitDone := make(chan error, 1)
		go func() { waitDone <- cmd.Wait() }()
		select {
		case err := <-waitDone:
			var exitErr *exec.ExitError
			if err == nil {
				t.Fatalf("expected SIGKILL termination")
			}
			if !errors.As(err, &exitErr) {
				t.Fatalf("unexpected wait error: %v", err)
			}
			if status, ok := exitErr.Sys().(syscall.WaitStatus); !ok || status.Signal() != syscall.SIGKILL {
				t.Fatalf("expected SIGKILL exit, got %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for SIGKILL termination")
		}
	})
}
