package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWipeAgentCache(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := filepath.Join(workDir, "cache")
	if err := os.MkdirAll(filepath.Join(cacheDir, "ccache"), 0o755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "ccache", "entry"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	msg, err := wipeAgentCache(workDir)
	if err != nil {
		t.Fatalf("wipeAgentCache: %v", err)
	}
	if !strings.Contains(msg, "cache wipe completed") {
		t.Fatalf("unexpected completion message: %q", msg)
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("read recreated cache dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty recreated cache dir, got %d entries", len(entries))
	}
}

func TestWipeAgentJobHistory(t *testing.T) {
	workDir := t.TempDir()
	workspacesDir := filepath.Join(workDir, "workspaces")
	if err := os.MkdirAll(filepath.Join(workspacesDir, "old-workspace-a"), 0o755); err != nil {
		t.Fatalf("create workspace a: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacesDir, "old-workspace-b"), 0o755); err != nil {
		t.Fatalf("create workspace b: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "job-legacy-1"), 0o755); err != nil {
		t.Fatalf("create legacy job dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "keep-me"), 0o755); err != nil {
		t.Fatalf("create non-job dir: %v", err)
	}

	msg, err := wipeAgentJobHistory(workDir)
	if err != nil {
		t.Fatalf("wipeAgentJobHistory: %v", err)
	}
	if !strings.Contains(msg, "removed=3 workspaces") {
		t.Fatalf("unexpected completion message: %q", msg)
	}

	if _, err := os.Stat(filepath.Join(workDir, "job-legacy-1")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy job dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspacesDir, "old-workspace-a")); !os.IsNotExist(err) {
		t.Fatalf("expected old workspace a removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspacesDir, "old-workspace-b")); !os.IsNotExist(err) {
		t.Fatalf("expected old workspace b removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "keep-me")); err != nil {
		t.Fatalf("expected non-job dir to remain, stat err=%v", err)
	}
}
