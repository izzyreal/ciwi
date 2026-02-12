package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindMSVCCompilerInInstallPathPrefersNewestToolset(t *testing.T) {
	root := t.TempDir()
	installPath := filepath.Join(root, "VS", "2022", "BuildTools")

	oldCL := filepath.Join(installPath, "VC", "Tools", "MSVC", "14.20.10000", "bin", "Hostx64", "x64", "cl.exe")
	newCL := filepath.Join(installPath, "VC", "Tools", "MSVC", "14.39.33519", "bin", "Hostx64", "x64", "cl.exe")
	armCL := filepath.Join(installPath, "VC", "Tools", "MSVC", "14.39.33519", "bin", "Hostarm64", "arm64", "cl.exe")

	for _, p := range []string{oldCL, newCL, armCL} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", p, err)
		}
		if err := os.WriteFile(p, []byte("stub"), 0o644); err != nil {
			t.Fatalf("write %q: %v", p, err)
		}
	}

	got := findMSVCCompilerInInstallPath(installPath)
	if got != newCL {
		t.Fatalf("expected newest x64 cl.exe, got %q", got)
	}
}

func TestWindowsProgramFilesRootsDeduplicatesAndPreservesOrder(t *testing.T) {
	t.Setenv("ProgramFiles(x86)", `C:\Program Files (x86)`)
	t.Setenv("ProgramW6432", `C:\Program Files`)
	t.Setenv("ProgramFiles", `C:\Program Files`)

	got := windowsProgramFilesRoots()
	if len(got) != 2 {
		t.Fatalf("expected 2 unique roots, got %d: %#v", len(got), got)
	}
	if got[0] != `C:\Program Files (x86)` {
		t.Fatalf("unexpected first root: %q", got[0])
	}
	if got[1] != `C:\Program Files` {
		t.Fatalf("unexpected second root: %q", got[1])
	}
}
