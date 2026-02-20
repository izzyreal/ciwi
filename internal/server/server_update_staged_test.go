package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServerUpdateStagedWrappers(t *testing.T) {
	t.Setenv("CIWI_LINUX_SYSTEM_UPDATER", "true")
	if isLinuxSystemUpdaterEnabled() != (runtime.GOOS == "linux") {
		t.Fatalf("unexpected isLinuxSystemUpdaterEnabled for current runtime")
	}
	t.Setenv("CIWI_LINUX_SYSTEM_UPDATER", "false")
	if isLinuxSystemUpdaterEnabled() {
		t.Fatalf("expected disabled linux system updater when env=false")
	}

	if got := sanitizeVersionToken(" v1.2.3+meta "); got == "" || strings.Contains(got, "+") {
		t.Fatalf("unexpected sanitizeVersionToken result: %q", got)
	}

	src := filepath.Join(t.TempDir(), "ciwi-new")
	if err := os.WriteFile(src, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write staged binary source: %v", err)
	}
	stagingDir := t.TempDir()
	manifestPath := filepath.Join(stagingDir, "pending.json")
	t.Setenv("CIWI_UPDATE_STAGING_DIR", stagingDir)
	t.Setenv("CIWI_UPDATE_STAGED_MANIFEST", manifestPath)
	info := latestUpdateInfo{Asset: githubReleaseAsset{Name: "ciwi-linux-amd64"}}
	if err := stageLinuxUpdateBinary("v1.2.3", info, src); err != nil {
		t.Fatalf("stageLinuxUpdateBinary: %v", err)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest file after staging: %v", err)
	}

	hashPath := filepath.Join(stagingDir, "hash-me")
	if err := os.WriteFile(hashPath, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write hash source: %v", err)
	}
	h, err := fileSHA256(hashPath)
	if err != nil || strings.TrimSpace(h) == "" {
		t.Fatalf("fileSHA256 failed h=%q err=%v", h, err)
	}
}
