package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectToolVersionByPathParsesSemver(t *testing.T) {
	binDir := t.TempDir()
	tool := filepath.Join(binDir, "mytool")
	if runtime.GOOS == "windows" {
		tool += ".bat"
		if err := os.WriteFile(tool, []byte("@echo mytool version 1.2.3\r\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	} else {
		if err := os.WriteFile(tool, []byte("#!/bin/sh\necho 'mytool version 1.2.3'\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	}
	if got := detectToolVersionByPath(tool); got != "1.2.3" {
		t.Fatalf("expected parsed version 1.2.3, got %q", got)
	}
}

func TestDetectToolVersionParsesGoVersionOutput(t *testing.T) {
	binDir := t.TempDir()
	name := "go"
	path := filepath.Join(binDir, name)
	if runtime.GOOS == "windows" {
		path += ".bat"
		if err := os.WriteFile(path, []byte("@echo go version go1.26.0 windows/amd64\r\n"), 0o755); err != nil {
			t.Fatalf("write fake go: %v", err)
		}
	} else {
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'go version go1.26.0 linux/amd64'\n"), 0o755); err != nil {
			t.Fatalf("write fake go: %v", err)
		}
	}
	t.Setenv("PATH", binDir)
	if got := detectToolVersion("go", "version"); got != "1.26.0" {
		t.Fatalf("expected go version 1.26.0, got %q", got)
	}
}

func TestDetectToolVersionByPathEdgeCases(t *testing.T) {
	if got := detectToolVersionByPath(""); got != "" {
		t.Fatalf("expected empty version for empty command path, got %q", got)
	}
	binDir := t.TempDir()
	tool := filepath.Join(binDir, "noversion")
	if runtime.GOOS == "windows" {
		tool += ".bat"
		if err := os.WriteFile(tool, []byte("@echo hello\r\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	} else {
		if err := os.WriteFile(tool, []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
			t.Fatalf("write fake tool: %v", err)
		}
	}
	if got := detectToolVersionByPath(tool); got != "" {
		t.Fatalf("expected empty version for output without semver, got %q", got)
	}
}

func TestCandidateVSWherePathsIncludesLookupAndEnvRoots(t *testing.T) {
	binDir := t.TempDir()
	lookupPath := filepath.Join(binDir, "vswhere.exe")
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(lookupPath, []byte("@echo C:\\VS\r\n"), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	} else {
		if err := os.WriteFile(lookupPath, []byte("#!/bin/sh\necho /VS\n"), 0o755); err != nil {
			t.Fatalf("write fake vswhere: %v", err)
		}
	}
	t.Setenv("PATH", binDir)
	t.Setenv("ProgramFiles(x86)", `C:\Program Files (x86)`)
	t.Setenv("ProgramW6432", `C:\Program Files`)
	t.Setenv("ProgramFiles", `C:\Program Files`)

	paths := candidateVSWherePaths()
	if len(paths) < 2 {
		t.Fatalf("expected candidateVSWherePaths to include lookup and env roots, got %v", paths)
	}
}

func TestFileExistsAndHasXorgDev(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if !fileExists(f) {
		t.Fatalf("expected fileExists true for regular file")
	}
	if fileExists(filepath.Dir(f)) {
		t.Fatalf("expected fileExists false for directory")
	}
	t.Setenv("PATH", "")
	if hasXorgDev() {
		t.Fatalf("expected hasXorgDev false without pkg-config in PATH")
	}
}

func TestDetectAgentCapabilitiesBasicShape(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("INVOCATION_ID", "")
	caps := detectAgentCapabilities()
	if caps["executor"] != executorScript {
		t.Fatalf("unexpected executor capability: %v", caps)
	}
	if caps["os"] == "" || caps["arch"] == "" || caps["shells"] == "" {
		t.Fatalf("expected base capability keys, got %v", caps)
	}
	if caps["run_mode"] == "" {
		t.Fatalf("expected run_mode capability")
	}
}
