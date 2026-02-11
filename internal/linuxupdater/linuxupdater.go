package linuxupdater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/store"
)

type stagedManifest struct {
	VersionUTC     string `json:"version_utc"`
	TargetVersion  string `json:"target_version"`
	AssetName      string `json:"asset_name"`
	StagedBinary   string `json:"staged_binary"`
	StagedSHA256   string `json:"staged_sha256"`
	RequestedAtUTC string `json:"requested_at_utc"`
}

func RunApplyStaged(args []string) (retErr error) {
	if os.Geteuid() != 0 {
		return fmt.Errorf("apply-staged-update requires root privileges")
	}

	fs := flag.NewFlagSet("apply-staged-update", flag.ContinueOnError)
	manifestPath := fs.String("manifest", envOrDefault("CIWI_UPDATE_STAGED_MANIFEST", "/var/lib/ciwi/updates/pending.json"), "path to staged update manifest")
	serviceName := fs.String("service", envOrDefault("CIWI_SERVER_SERVICE_NAME", "ciwi.service"), "systemd service to restart")
	if err := fs.Parse(args); err != nil {
		return err
	}
	state := openStateStore()
	if state != nil {
		_ = setUpdateState(state, map[string]string{
			"update_last_apply_status": "running",
			"update_message":           "linux updater applying staged update",
			"update_last_apply_utc":    time.Now().UTC().Format(time.RFC3339Nano),
		})
		defer state.Close()
		defer func() {
			if retErr != nil {
				_ = setUpdateState(state, map[string]string{
					"update_last_apply_status": "failed",
					"update_message":           "linux updater failed: " + retErr.Error(),
				})
			}
		}()
	}

	targetPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	targetPath, _ = filepath.Abs(targetPath)

	manifest, err := readManifest(*manifestPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(manifest.StagedBinary) == "" {
		return fmt.Errorf("manifest missing staged_binary")
	}
	if strings.TrimSpace(manifest.StagedSHA256) == "" {
		return fmt.Errorf("manifest missing staged_sha256")
	}
	if _, err := os.Stat(manifest.StagedBinary); err != nil {
		return fmt.Errorf("staged binary not found: %w", err)
	}
	gotHash, err := fileSHA256(manifest.StagedBinary)
	if err != nil {
		return fmt.Errorf("hash staged binary: %w", err)
	}
	wantHash := strings.ToLower(strings.TrimSpace(manifest.StagedSHA256))
	if gotHash != wantHash {
		return fmt.Errorf("staged binary hash mismatch: got=%s want=%s", gotHash, wantHash)
	}

	backupPath := targetPath + ".prev"
	_ = os.Remove(backupPath)
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(manifest.StagedBinary, targetPath); err != nil {
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("move staged binary into place: %w", err)
	}
	if err := os.Chmod(targetPath, 0o755); err != nil {
		_ = os.Rename(targetPath, manifest.StagedBinary)
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("chmod target binary: %w", err)
	}
	_ = os.Remove(*manifestPath)

	systemctlPath := strings.TrimSpace(envOrDefault("CIWI_SYSTEMCTL_PATH", "/bin/systemctl"))
	if systemctlPath == "" {
		systemctlPath = "/bin/systemctl"
	}
	cmd := exec.Command(systemctlPath, "restart", *serviceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restart service %q: %w (%s)", *serviceName, err, strings.TrimSpace(string(out)))
	}
	if state != nil {
		msg := "update successful"
		if strings.TrimSpace(manifest.TargetVersion) != "" {
			msg = "update successful: " + strings.TrimSpace(manifest.TargetVersion)
		}
		_ = setUpdateState(state, map[string]string{
			"update_last_apply_status": "success",
			"update_latest_version":    strings.TrimSpace(manifest.TargetVersion),
			"agent_update_target":      strings.TrimSpace(manifest.TargetVersion),
			"update_message":           msg,
			"update_last_apply_utc":    time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	return nil
}

func readManifest(path string) (stagedManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return stagedManifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var m stagedManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return stagedManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return m, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func BuildManifest(targetVersion, assetName, stagedBinary, stagedSHA256 string) ([]byte, error) {
	m := stagedManifest{
		VersionUTC:     time.Now().UTC().Format(time.RFC3339Nano),
		TargetVersion:  strings.TrimSpace(targetVersion),
		AssetName:      strings.TrimSpace(assetName),
		StagedBinary:   strings.TrimSpace(stagedBinary),
		StagedSHA256:   strings.ToLower(strings.TrimSpace(stagedSHA256)),
		RequestedAtUTC: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return json.MarshalIndent(m, "", "  ")
}

func openStateStore() *store.Store {
	dbPath := strings.TrimSpace(envOrDefault("CIWI_DB_PATH", "/var/lib/ciwi/ciwi.db"))
	if dbPath == "" {
		return nil
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return nil
	}
	return st
}

func setUpdateState(st *store.Store, values map[string]string) error {
	for k, v := range values {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if err := st.SetAppState(k, v); err != nil {
			return err
		}
	}
	return nil
}
