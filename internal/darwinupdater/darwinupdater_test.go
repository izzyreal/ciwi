package darwinupdater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildManifestAndReadManifest(t *testing.T) {
	raw, err := BuildManifest(
		"v2.0.0",
		"ciwi-darwin-arm64",
		"/usr/local/bin/ciwi-agent",
		"/tmp/staged",
		strings.Repeat("b", 64),
		"io.github.ciwi.agent",
		"/Users/test/Library/LaunchAgents/io.github.ciwi.agent.plist",
		"io.github.ciwi.updater",
		"/Users/test/Library/LaunchAgents/io.github.ciwi.updater.plist",
		"source-1",
		123,
	)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	var parsed stagedManifest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("decode built manifest: %v", err)
	}
	if parsed.TargetVersion != "v2.0.0" || parsed.AgentPID != 123 {
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
	if m.AgentLabel != "io.github.ciwi.agent" || m.TargetBinary == "" {
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
	content := []byte("darwin-updater")
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

func TestRunApplyStagedAgentRequiresManifest(t *testing.T) {
	t.Setenv("CIWI_AGENT_UPDATE_MANIFEST", "")
	err := RunApplyStagedAgent(nil)
	if err == nil || !strings.Contains(err.Error(), "requires --manifest") {
		t.Fatalf("expected missing manifest error, got %v", err)
	}
}

func TestRunApplyStagedAgentValidatesManifestFieldsBeforeCommands(t *testing.T) {
	m := stagedManifest{
		TargetBinary: "/tmp/ciwi-agent",
		StagedBinary: "",
		StagedSHA256: strings.Repeat("a", 64),
		AgentLabel:   "io.github.ciwi.agent",
		AgentPlist:   "/tmp/agent.plist",
	}
	raw, _ := json.Marshal(m)
	path := filepath.Join(t.TempDir(), "pending.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	err := RunApplyStagedAgent([]string{"--manifest", path})
	if err == nil || !strings.Contains(err.Error(), "missing staged_binary") {
		t.Fatalf("expected staged_binary validation error, got %v", err)
	}
}

func TestRunApplyStagedAgentDetectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "staged-bin")
	if err := os.WriteFile(staged, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	m := stagedManifest{
		TargetBinary: "/tmp/ciwi-agent",
		StagedBinary: staged,
		StagedSHA256: strings.Repeat("0", 64),
		AgentLabel:   "io.github.ciwi.agent",
		AgentPlist:   "/tmp/agent.plist",
	}
	raw, _ := json.Marshal(m)
	path := filepath.Join(dir, "pending.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	err := RunApplyStagedAgent([]string{"--manifest", path})
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch error, got %v", err)
	}
}

func TestEnvOrDefaultAndAlreadyLoaded(t *testing.T) {
	const key = "CIWI_TEST_DARWINUPDATER_ENV"
	t.Setenv(key, "")
	if got := envOrDefault(key, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	t.Setenv(key, " value ")
	if got := envOrDefault(key, "fallback"); got != "value" {
		t.Fatalf("expected trimmed env value, got %q", got)
	}

	if !isAlreadyLoadedErr(assertErr("Service is already loaded")) {
		t.Fatalf("expected already-loaded detection")
	}
	if isAlreadyLoadedErr(assertErr("random failure")) {
		t.Fatalf("unexpected already-loaded detection")
	}
}

func TestWaitForProcessExit(t *testing.T) {
	const impossiblePID = int(1 << 30)
	if err := waitForProcessExit(impossiblePID, 50*time.Millisecond); err != nil {
		t.Fatalf("missing pid should return quickly, got %v", err)
	}
	err := waitForProcessExit(os.Getpid(), 50*time.Millisecond)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout waiting for pid") {
		t.Fatalf("expected timeout for current running pid, got %v", err)
	}
}

func TestAdHocSignBinaryEmptyPath(t *testing.T) {
	if err := adHocSignBinary("   "); err == nil {
		t.Fatalf("expected empty path error")
	}
}

type fixedErr string

func (e fixedErr) Error() string { return string(e) }

func assertErr(msg string) error { return fixedErr(msg) }
