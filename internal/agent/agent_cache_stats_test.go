package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferCacheType(t *testing.T) {
	cases := []struct {
		cache resolvedJobCache
		want  string
	}{
		{resolvedJobCache{ID: "ccache", Env: "CCACHE_DIR"}, "ccache"},
		{resolvedJobCache{ID: "fetchcontent", Env: "FETCHCONTENT_BASE_DIR"}, "fetchcontent"},
		{resolvedJobCache{ID: "other", Env: "OTHER"}, "generic"},
	}
	for _, tc := range cases {
		if got := inferCacheType(tc.cache); got != tc.want {
			t.Fatalf("inferCacheType(%+v)=%q want=%q", tc.cache, got, tc.want)
		}
	}
}

func TestSummarizeDir(t *testing.T) {
	if _, _, _, err := summarizeDir(""); err == nil {
		t.Fatalf("expected error for empty path")
	}

	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}
	files, dirs, size, err := summarizeDir(root)
	if err != nil {
		t.Fatalf("summarizeDir failed: %v", err)
	}
	if files != 2 || dirs != 1 || size != 8 {
		t.Fatalf("unexpected summarizeDir stats files=%d dirs=%d size=%d", files, dirs, size)
	}
}

func TestReadCCacheMetrics(t *testing.T) {
	t.Run("no ccache in PATH", func(t *testing.T) {
		t.Setenv("PATH", "")
		if got := readCCacheMetrics(t.TempDir()); got != nil {
			t.Fatalf("expected nil when ccache missing, got %v", got)
		}
	})

	t.Run("parses output and keeps first duplicate key", func(t *testing.T) {
		binDir := t.TempDir()
		script := filepath.Join(binDir, "ccache")
		content := "#!/bin/sh\n" +
			"echo 'Cache directory: /tmp/cc'\n" +
			"echo 'Hits: 42'\n" +
			"echo 'Hits: 99'\n" +
			"echo 'invalid-line'\n"
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatalf("write fake ccache: %v", err)
		}
		t.Setenv("PATH", binDir)

		got := readCCacheMetrics(t.TempDir())
		if got["Cache directory"] != "/tmp/cc" {
			t.Fatalf("unexpected parsed cache directory: %v", got)
		}
		if got["Hits"] != "42" {
			t.Fatalf("expected first duplicate key value to win, got %v", got)
		}
	})
}

func TestCollectJobCacheStats(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "obj.o"), []byte("payload"), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	t.Setenv("PATH", "")
	got := collectJobCacheStats([]resolvedJobCache{{
		ID:     "ccache",
		Env:    "CCACHE_DIR",
		Path:   cacheDir,
		Source: "hit",
	}})
	if len(got) != 1 {
		t.Fatalf("expected one cache stat entry, got %d", len(got))
	}
	if got[0].Type != "ccache" || got[0].Files != 1 || got[0].SizeBytes != int64(len("payload")) {
		t.Fatalf("unexpected cache stats: %+v", got[0])
	}
	if got[0].ToolMetrics != nil {
		t.Fatalf("expected nil tool metrics when ccache is missing")
	}
}
