package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentWorkDir_DefaultIsAbsolute(t *testing.T) {
	tmp := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("CIWI_AGENT_WORKDIR", "")

	got := agentWorkDir()
	want, err := filepath.Abs(filepath.Join(".ciwi-agent", "work"))
	if err != nil {
		t.Fatalf("abs want: %v", err)
	}
	if got != want {
		t.Fatalf("agentWorkDir default=%q want=%q", got, want)
	}
}

func TestAgentWorkDir_RelativeEnvIsAbsolute(t *testing.T) {
	tmp := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("CIWI_AGENT_WORKDIR", "workdir/subdir")

	got := agentWorkDir()
	want, err := filepath.Abs(filepath.Join("workdir", "subdir"))
	if err != nil {
		t.Fatalf("abs want: %v", err)
	}
	if got != want {
		t.Fatalf("agentWorkDir env=%q want=%q", got, want)
	}
}
