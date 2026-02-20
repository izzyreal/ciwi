package updatehelper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRollsBackWhenRelaunchFails(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "ciwi")
	newPath := filepath.Join(tmp, "ciwi.new")
	oldContent := []byte("#!/bin/sh\necho old\n")
	newContent := []byte("not-an-executable-binary")
	if err := os.WriteFile(target, oldContent, 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(newPath, newContent, 0o644); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	// Use a definitely-missing PID so waitForParentExit returns quickly.
	err := Run([]string{
		"--target", target,
		"--new", newPath,
		"--pid", "1073741824",
	})
	if err == nil || !strings.Contains(err.Error(), "restart updated binary") {
		t.Fatalf("expected relaunch failure, got %v", err)
	}

	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after rollback: %v", err)
	}
	if string(gotTarget) != string(oldContent) {
		t.Fatalf("expected target to roll back to old content, got %q", string(gotTarget))
	}
	gotNew, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new path after rollback: %v", err)
	}
	if string(gotNew) != string(newContent) {
		t.Fatalf("expected new path to be restored, got %q", string(gotNew))
	}
}

func TestRunReplacesBinaryWhenRelaunchSucceeds(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "ciwi")
	newPath := filepath.Join(tmp, "ciwi.new")
	oldContent := []byte("#!/bin/sh\necho old\n")
	newContent := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(target, oldContent, 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(newPath, newContent, 0o755); err != nil {
		t.Fatalf("write new binary: %v", err)
	}

	err := Run([]string{
		"--target", target,
		"--new", newPath,
		"--pid", "1073741824",
	})
	if err != nil {
		t.Fatalf("expected successful update helper run, got %v", err)
	}
	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if string(gotTarget) != string(newContent) {
		t.Fatalf("expected target to contain new binary content, got %q", string(gotTarget))
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("expected new binary path to be moved away, err=%v", err)
	}
}
