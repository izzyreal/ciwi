package linuxupdater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildManifestAndReadManifest(t *testing.T) {
	raw, err := BuildManifest("v1.2.3", "ciwi-linux-amd64", "/tmp/staged", strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	var parsed stagedManifest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("decode built manifest: %v", err)
	}
	if parsed.TargetVersion != "v1.2.3" || parsed.AssetName != "ciwi-linux-amd64" {
		t.Fatalf("unexpected manifest fields: %+v", parsed)
	}

	path := filepath.Join(t.TempDir(), "pending.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, err := readManifest(path)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if m.TargetVersion != "v1.2.3" || m.StagedBinary != "/tmp/staged" {
		t.Fatalf("unexpected read manifest: %+v", m)
	}
}

func TestReadManifestInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := readManifest(path); err == nil {
		t.Fatalf("expected decode error for invalid JSON")
	}
}

func TestFileSHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bin")
	content := []byte("linux-updater")
	if err := os.WriteFile(p, content, 0o755); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got, err := fileSHA256(p)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("unexpected hash: got=%q want=%q", got, want)
	}
}

func TestEnvOrDefault(t *testing.T) {
	const key = "CIWI_TEST_LINUXUPDATER_ENV"
	t.Setenv(key, "")
	if got := envOrDefault(key, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	t.Setenv(key, " value ")
	if got := envOrDefault(key, "fallback"); got != "value" {
		t.Fatalf("expected trimmed env value, got %q", got)
	}
}

func TestRunApplyStagedRequiresRoot(t *testing.T) {
	err := RunApplyStaged(nil)
	if os.Geteuid() == 0 {
		// In elevated environments behavior continues further; do not assert here.
		return
	}
	if err == nil || !strings.Contains(err.Error(), "requires root privileges") {
		t.Fatalf("expected root privilege error, got %v", err)
	}
}
