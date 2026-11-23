package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func autoBumpTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func autoBumpTestRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	autoBumpTestGit(t, root, "init", "--bare", bare)
	autoBumpTestGit(t, root, "clone", bare, work)
	autoBumpTestGit(t, work, "checkout", "-b", "main")
	autoBumpTestGit(t, work, "config", "user.name", "CIWI Test")
	autoBumpTestGit(t, work, "config", "user.email", "ciwi-test@example.invalid")
	if err := os.WriteFile(filepath.Join(work, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	autoBumpTestGit(t, work, "add", "VERSION")
	autoBumpTestGit(t, work, "commit", "-m", "initial version")
	autoBumpTestGit(t, work, "push", "-u", "origin", "main")
	return bare, work
}

func autoBumpTestSpec(bare string) *protocol.AutoBumpStepSpec {
	return &protocol.AutoBumpStepSpec{
		Mode: "patch", VersionRaw: "1.2.3", VersionFile: "VERSION",
		SourceRefRaw: "refs/heads/main", SourceRepo: bare,
	}
}

func TestRunAutoBumpStepPushesVersionCommit(t *testing.T) {
	bare, work := autoBumpTestRepo(t)
	var output bytes.Buffer
	if err := runAutoBumpStep(context.Background(), work, autoBumpTestSpec(bare), os.Environ(), &output); err != nil {
		t.Fatalf("auto bump: %v\n%s", err, output.String())
	}
	if got := autoBumpTestGit(t, work, "--git-dir="+bare, "show", "main:VERSION"); got != "1.2.4" {
		t.Fatalf("remote version = %q, want 1.2.4", got)
	}
	if got := autoBumpTestGit(t, work, "log", "-1", "--format=%s"); got != "chore: bump VERSION to 1.2.4 [skip ci]" {
		t.Fatalf("commit message = %q", got)
	}
}

func TestRunAutoBumpStepRejectsStaleVersion(t *testing.T) {
	bare, work := autoBumpTestRepo(t)
	if err := os.WriteFile(filepath.Join(work, "VERSION"), []byte("1.2.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	autoBumpTestGit(t, work, "add", "VERSION")
	autoBumpTestGit(t, work, "commit", "-m", "version advanced")
	autoBumpTestGit(t, work, "push", "origin", "main")
	var output bytes.Buffer
	err := runAutoBumpStep(context.Background(), work, autoBumpTestSpec(bare), os.Environ(), &output)
	if err == nil || !strings.Contains(err.Error(), "branch main moved from 1.2.3 to 1.2.4") {
		t.Fatalf("expected stale version error, got %v", err)
	}
	if got := autoBumpTestGit(t, work, "--git-dir="+bare, "show", "main:VERSION"); got != "1.2.4" {
		t.Fatalf("remote version changed to %q", got)
	}
}

func TestRunAutoBumpStepReportsRejectedPush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bare repository hook uses a POSIX shell")
	}
	bare, work := autoBumpTestRepo(t)
	hook := filepath.Join(bare, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := runAutoBumpStep(context.Background(), work, autoBumpTestSpec(bare), os.Environ(), &output)
	if err == nil || !strings.Contains(err.Error(), "auto bump push failed") {
		t.Fatalf("expected push failure, got %v", err)
	}
	if got := autoBumpTestGit(t, work, "--git-dir="+bare, "show", "main:VERSION"); got != "1.2.3" {
		t.Fatalf("remote version changed to %q", got)
	}
}

func TestRunAutoBumpStepHonorsCancellation(t *testing.T) {
	bare, work := autoBumpTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runAutoBumpStep(ctx, work, autoBumpTestSpec(bare), os.Environ(), &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if got := autoBumpTestGit(t, work, "--git-dir="+bare, "show", "main:VERSION"); got != "1.2.3" {
		t.Fatalf("remote version changed to %q", got)
	}
}

func TestStepPlanIncludesTypedAutoBumpWithoutScript(t *testing.T) {
	spec := &protocol.AutoBumpStepSpec{Mode: "patch", VersionRaw: "1.2.3"}
	steps := stepPlanToScriptSteps([]protocol.JobStepPlanItem{{
		Index: 1, Total: 1, Kind: protocol.JobStepKindAutoBump, AutoBump: spec,
	}})
	if len(steps) != 1 || steps[0].autoBump == nil || steps[0].autoBump.VersionRaw != "1.2.3" {
		t.Fatalf("typed step was omitted: %+v", steps)
	}
	steps[0].autoBump.VersionRaw = "changed"
	if spec.VersionRaw != "1.2.3" {
		t.Fatal("step payload was not copied")
	}
}
