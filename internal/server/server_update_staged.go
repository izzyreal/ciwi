package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/izzyreal/ciwi/internal/linuxupdater"
)

func isLinuxSystemUpdaterEnabled() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	return strings.TrimSpace(envOrDefault("CIWI_LINUX_SYSTEM_UPDATER", "true")) != "false"
}

func stageLinuxUpdateBinary(targetVersion string, info latestUpdateInfo, newBinPath string) error {
	stagingDir := strings.TrimSpace(envOrDefault("CIWI_UPDATE_STAGING_DIR", "/var/lib/ciwi/updates"))
	if stagingDir == "" {
		stagingDir = "/var/lib/ciwi/updates"
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	versionToken := sanitizeVersionToken(targetVersion)
	if versionToken == "" {
		versionToken = "latest"
	}
	stagedPath := filepath.Join(stagingDir, "ciwi-"+versionToken+exeExt())
	_ = os.Remove(stagedPath)
	if err := os.Rename(newBinPath, stagedPath); err != nil {
		if err := copyFile(newBinPath, stagedPath, 0o755); err != nil {
			return fmt.Errorf("stage update binary: %w", err)
		}
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return fmt.Errorf("chmod staged binary: %w", err)
	}
	hash, err := fileSHA256(stagedPath)
	if err != nil {
		return fmt.Errorf("hash staged binary: %w", err)
	}

	manifest, err := linuxupdater.BuildManifest(targetVersion, info.Asset.Name, stagedPath, hash)
	if err != nil {
		return fmt.Errorf("build staged manifest: %w", err)
	}
	manifestPath := strings.TrimSpace(envOrDefault("CIWI_UPDATE_STAGED_MANIFEST", filepath.Join(stagingDir, "pending.json")))
	if manifestPath == "" {
		manifestPath = filepath.Join(stagingDir, "pending.json")
	}
	tmpPath := manifestPath + ".tmp"
	if err := os.WriteFile(tmpPath, manifest, 0o644); err != nil {
		return fmt.Errorf("write staged manifest: %w", err)
	}
	if err := os.Rename(tmpPath, manifestPath); err != nil {
		return fmt.Errorf("publish staged manifest: %w", err)
	}
	return nil
}

func triggerLinuxSystemUpdater() error {
	unit := strings.TrimSpace(envOrDefault("CIWI_UPDATER_UNIT", "ciwi-updater.service"))
	if unit == "" {
		unit = "ciwi-updater.service"
	}
	systemctlPath := strings.TrimSpace(envOrDefault("CIWI_SYSTEMCTL_PATH", "/bin/systemctl"))
	if systemctlPath == "" {
		systemctlPath = "/bin/systemctl"
	}
	cmd := exec.Command(systemctlPath, "start", "--no-block", unit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start updater unit: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
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

func sanitizeVersionToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "._-")
}
