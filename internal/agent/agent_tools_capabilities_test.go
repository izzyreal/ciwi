package agent

import "testing"

func TestDetectToolVersionsIncludesDocsAndTransferTools(t *testing.T) {
	orig := detectToolVersionInShellFn
	t.Cleanup(func() { detectToolVersionInShellFn = orig })

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

	got := detectToolVersions()
	if got["lftp"] != "4.9.2" {
		t.Fatalf("expected lftp probe, got %#v", got)
	}
	if got["rinoh"] != "0.5.5" {
		t.Fatalf("expected rinoh probe, got %#v", got)
	}
	if got["sphinx-build"] != "7.3.7" {
		t.Fatalf("expected sphinx-build probe, got %#v", got)
	}
}
