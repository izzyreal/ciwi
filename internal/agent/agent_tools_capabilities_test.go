package agent

import (
	"os/exec"
	"testing"
)

func TestDetectToolVersionsIncludesDocsAndTransferTools(t *testing.T) {
	orig := detectToolVersionInShellFn
	origLookPath := lookPathFn
	t.Cleanup(func() {
		detectToolVersionInShellFn = orig
		lookPathFn = origLookPath
	})

	detectToolVersionInShellFn = func(shell, cmd string, args ...string) string {
		switch cmd {
		case "lftp":
			return "4.9.2"
		case "rinoh":
			return "0.5.5"
		case "sphinx-build":
			return "7.3.7"
		default:
			return ""
		}
	}
	lookPathFn = func(file string) (string, error) {
		if file == "dmgbuild" {
			return "/usr/local/bin/dmgbuild", nil
		}
		return "", exec.ErrNotFound
	}

	got := detectToolVersions()
	if got["lftp"] != "4.9.2" {
		t.Fatalf("expected lftp probe, got %#v", got)
	}
	if got["dmgbuild"] != "1" {
		t.Fatalf("expected dmgbuild presence probe, got %#v", got)
	}
	if got["rinoh"] != "0.5.5" {
		t.Fatalf("expected rinoh probe, got %#v", got)
	}
	if got["sphinx-build"] != "7.3.7" {
		t.Fatalf("expected sphinx-build probe, got %#v", got)
	}
}
