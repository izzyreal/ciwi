package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestAgentCacheHelpers(t *testing.T) {
	if got := sanitizeCacheSegment(" FetchContent V1 "); got != "fetchcontent-v1" {
		t.Fatalf("unexpected sanitizeCacheSegment value: %q", got)
	}
	if got := sanitizeCacheSegment("..."); got != "cache" {
		t.Fatalf("expected fallback cache segment, got %q", got)
	}

	root := t.TempDir()
	spec := protocol.JobCacheSpec{ID: "ccache", Env: "CCACHE_DIR"}
	dir, source, err := resolveSingleJobCacheDir(root, spec)
	if err != nil {
		t.Fatalf("resolveSingleJobCacheDir first: %v", err)
	}
	if source != "miss" {
		t.Fatalf("expected initial cache source miss, got %q", source)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ciwi-cache-touch")); err != nil {
		t.Fatalf("expected touch marker file: %v", err)
	}
	dir2, source2, err := resolveSingleJobCacheDir(root, spec)
	if err != nil {
		t.Fatalf("resolveSingleJobCacheDir second: %v", err)
	}
	if dir2 != dir || source2 != "hit" {
		t.Fatalf("expected cache hit on second resolve, dir2=%q source2=%q", dir2, source2)
	}

	if samePath("/a/b", "/a/c") {
		t.Fatalf("expected different paths to not match")
	}
	if runtime.GOOS == "windows" {
		if !samePath(`C:\Temp\A`, `c:\temp\a`) {
			t.Fatalf("expected case-insensitive samePath on windows")
		}
	} else {
		if !samePath("/tmp/a", "/tmp/a") {
			t.Fatalf("expected equal samePath on unix")
		}
	}
}

func TestResolveJobCacheEnvDetailedSkipsInvalidSpecs(t *testing.T) {
	workDir := t.TempDir()
	execDir := t.TempDir()
	job := protocol.JobExecution{Caches: []protocol.JobCacheSpec{
		{ID: "", Env: "CCACHE_DIR"},
		{ID: "ccache", Env: ""},
		{ID: "ccache", Env: "CCACHE_DIR"},
	}}
	env, logs, resolved := resolveJobCacheEnvDetailed(workDir, execDir, job)
	if env["CCACHE_DIR"] == "" {
		t.Fatalf("expected valid cache entry to be resolved, env=%v logs=%v", env, logs)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected only one resolved cache, got %d (%+v)", len(resolved), resolved)
	}
}
