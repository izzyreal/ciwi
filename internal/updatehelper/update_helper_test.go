package updatehelper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMultiArg(t *testing.T) {
	var m multiArg
	if err := m.Set("a"); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if err := m.Set("b"); err != nil {
		t.Fatalf("set b: %v", err)
	}
	if got := m.String(); got != "a,b" {
		t.Fatalf("unexpected multiArg string: %q", got)
	}
}

func TestRunRequiresMandatoryFlags(t *testing.T) {
	err := Run([]string{})
	if err == nil || !strings.Contains(err.Error(), "requires --target, --new and --pid") {
		t.Fatalf("expected missing mandatory flag error, got %v", err)
	}
}

func TestRunFailsWhenNewBinaryMissing(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "ciwi")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	missingNew := filepath.Join(tmp, "does-not-exist")
	err := Run([]string{
		"--target", target,
		"--new", missingNew,
		"--pid", "1",
	})
	if err == nil || !strings.Contains(err.Error(), "move new binary into place") {
		t.Fatalf("expected move-new error, got %v", err)
	}
}

func TestProcessRunningAndWaitForParentExit(t *testing.T) {
	// An extremely large PID should not exist.
	const impossiblePID = int(1 << 30)
	running, err := processRunning(impossiblePID)
	if err != nil {
		t.Fatalf("processRunning error: %v", err)
	}
	if running {
		t.Fatalf("impossible PID must not be running")
	}

	if err := waitForParentExit(impossiblePID, 100*time.Millisecond); err != nil {
		t.Fatalf("waitForParentExit should return quickly for missing pid: %v", err)
	}
}

func TestWaitForParentExitTimeoutForRunningProcess(t *testing.T) {
	// Current process is running; with a short timeout we should hit timeout.
	err := waitForParentExit(os.Getpid(), 50*time.Millisecond)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout waiting for pid") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestStartRelaunchedFailsForMissingTarget(t *testing.T) {
	err := startRelaunched("/path/does/not/exist/ciwi", nil)
	if err == nil {
		t.Fatalf("expected startRelaunched to fail for missing target")
	}
}
