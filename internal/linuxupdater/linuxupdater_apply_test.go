package linuxupdater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifestFile(t *testing.T, path string, m stagedManifest) {
	t.Helper()
	raw, err := BuildManifest(m.TargetVersion, m.AssetName, m.StagedBinary, m.StagedSHA256)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestApplyStagedUpdateValidationErrors(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "ciwi")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	systemctlPath := "/usr/bin/true"

	t.Run("missing staged_binary", func(t *testing.T) {
		manifestPath := filepath.Join(tmp, "missing-staged.json")
		writeManifestFile(t, manifestPath, stagedManifest{
			TargetVersion: "v1.0.0",
			AssetName:     "ciwi-linux-amd64",
			StagedBinary:  "",
			StagedSHA256:  strings.Repeat("a", 64),
		})
		if _, err := applyStagedUpdate(manifestPath, target, "ciwi.service", systemctlPath); err == nil || !strings.Contains(err.Error(), "missing staged_binary") {
			t.Fatalf("expected missing staged_binary error, got %v", err)
		}
	})

	t.Run("missing staged_sha256", func(t *testing.T) {
		manifestPath := filepath.Join(tmp, "missing-sha.json")
		writeManifestFile(t, manifestPath, stagedManifest{
			TargetVersion: "v1.0.0",
			AssetName:     "ciwi-linux-amd64",
			StagedBinary:  filepath.Join(tmp, "staged"),
			StagedSHA256:  "",
		})
		if _, err := applyStagedUpdate(manifestPath, target, "ciwi.service", systemctlPath); err == nil || !strings.Contains(err.Error(), "missing staged_sha256") {
			t.Fatalf("expected missing staged_sha256 error, got %v", err)
		}
	})

	t.Run("staged binary missing", func(t *testing.T) {
		manifestPath := filepath.Join(tmp, "missing-binary.json")
		writeManifestFile(t, manifestPath, stagedManifest{
			TargetVersion: "v1.0.0",
			AssetName:     "ciwi-linux-amd64",
			StagedBinary:  filepath.Join(tmp, "nope"),
			StagedSHA256:  strings.Repeat("a", 64),
		})
		if _, err := applyStagedUpdate(manifestPath, target, "ciwi.service", systemctlPath); err == nil || !strings.Contains(err.Error(), "staged binary not found") {
			t.Fatalf("expected staged binary not found error, got %v", err)
		}
	})
}

func TestApplyStagedUpdateHashMismatch(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "ciwi")
	staged := filepath.Join(tmp, "ciwi.new")
	manifestPath := filepath.Join(tmp, "pending.json")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	writeManifestFile(t, manifestPath, stagedManifest{
		TargetVersion: "v1.0.0",
		AssetName:     "ciwi-linux-amd64",
		StagedBinary:  staged,
		StagedSHA256:  strings.Repeat("0", 64),
	})
	if _, err := applyStagedUpdate(manifestPath, target, "ciwi.service", "/usr/bin/true"); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch error, got %v", err)
	}
}

func TestApplyStagedUpdateSuccessAndRestartFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tmp := t.TempDir()
		target := filepath.Join(tmp, "ciwi")
		staged := filepath.Join(tmp, "ciwi.new")
		manifestPath := filepath.Join(tmp, "pending.json")
		oldContent := []byte("old")
		newContent := []byte("new")
		if err := os.WriteFile(target, oldContent, 0o755); err != nil {
			t.Fatalf("write target: %v", err)
		}
		if err := os.WriteFile(staged, newContent, 0o755); err != nil {
			t.Fatalf("write staged: %v", err)
		}
		sum, err := fileSHA256(staged)
		if err != nil {
			t.Fatalf("fileSHA256: %v", err)
		}
		writeManifestFile(t, manifestPath, stagedManifest{
			TargetVersion: "v2.0.0",
			AssetName:     "ciwi-linux-amd64",
			StagedBinary:  staged,
			StagedSHA256:  sum,
		})

		gotManifest, err := applyStagedUpdate(manifestPath, target, "ciwi.service", "/usr/bin/true")
		if err != nil {
			t.Fatalf("applyStagedUpdate success: %v", err)
		}
		if gotManifest.TargetVersion != "v2.0.0" {
			t.Fatalf("unexpected returned manifest: %+v", gotManifest)
		}
		gotTarget, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read replaced target: %v", err)
		}
		if string(gotTarget) != string(newContent) {
			t.Fatalf("expected replaced target content, got %q", string(gotTarget))
		}
		if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
			t.Fatalf("expected manifest to be removed, err=%v", err)
		}
	})

	t.Run("restart failure", func(t *testing.T) {
		tmp := t.TempDir()
		target := filepath.Join(tmp, "ciwi")
		staged := filepath.Join(tmp, "ciwi.new")
		manifestPath := filepath.Join(tmp, "pending.json")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatalf("write target: %v", err)
		}
		if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
			t.Fatalf("write staged: %v", err)
		}
		sum, err := fileSHA256(staged)
		if err != nil {
			t.Fatalf("fileSHA256: %v", err)
		}
		writeManifestFile(t, manifestPath, stagedManifest{
			TargetVersion: "v2.0.0",
			AssetName:     "ciwi-linux-amd64",
			StagedBinary:  staged,
			StagedSHA256:  sum,
		})

		if _, err := applyStagedUpdate(manifestPath, target, "ciwi.service", "/usr/bin/false"); err == nil || !strings.Contains(err.Error(), "restart service") {
			t.Fatalf("expected restart service error, got %v", err)
		}
		// Existing behavior: binary replacement already happened before restart.
		gotTarget, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read target after restart failure: %v", err)
		}
		if string(gotTarget) != "new" {
			t.Fatalf("expected target to remain updated after restart failure, got %q", string(gotTarget))
		}
	})
}
