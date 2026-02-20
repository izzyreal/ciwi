package version

import "testing"

func TestCurrentVersion(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = " v1.2.3 "
	if got := Current(); got != "v1.2.3" {
		t.Fatalf("expected trimmed version, got %q", got)
	}

	Version = "   "
	if got := Current(); got != "dev" {
		t.Fatalf("expected dev fallback, got %q", got)
	}
}
