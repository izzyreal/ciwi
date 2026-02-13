package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/izzyreal/ciwi/internal/linuxupdater"
)

type StageLinuxOptions struct {
	StagingDir   string
	ManifestPath string
}

func IsLinuxSystemUpdaterEnabled(goos, enabledEnv string) bool {
	if strings.TrimSpace(goos) != "linux" {
		return false
	}
	return strings.TrimSpace(enabledEnv) != "false"
}

func StageLinuxUpdateBinary(targetVersion, assetName, newBinPath string, opts StageLinuxOptions) error {
	stagingDir := strings.TrimSpace(opts.StagingDir)
	if stagingDir == "" {
		stagingDir = "/var/lib/ciwi/updates"
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	versionToken := SanitizeVersionToken(targetVersion)
	if versionToken == "" {
		versionToken = "latest"
	}
	stagedPath := filepath.Join(stagingDir, "ciwi-"+versionToken+ExeExt())
	_ = os.Remove(stagedPath)
	if err := os.Rename(newBinPath, stagedPath); err != nil {
		if err := CopyFile(newBinPath, stagedPath, 0o755); err != nil {
			return fmt.Errorf("stage update binary: %w", err)
		}
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return fmt.Errorf("chmod staged binary: %w", err)
	}
	hash, err := FileSHA256(stagedPath)
	if err != nil {
		return fmt.Errorf("hash staged binary: %w", err)
	}

	manifest, err := linuxupdater.BuildManifest(targetVersion, assetName, stagedPath, hash)
	if err != nil {
		return fmt.Errorf("build staged manifest: %w", err)
	}
	manifestPath := strings.TrimSpace(opts.ManifestPath)
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

func TriggerLinuxSystemUpdater(systemctlPath, unit string) error {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		unit = "ciwi-updater.service"
	}
	systemctlPath = strings.TrimSpace(systemctlPath)
	if systemctlPath == "" {
		systemctlPath = "/bin/systemctl"
	}
	cmd := exec.Command(systemctlPath, "start", "--no-block", unit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start updater unit: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func FileSHA256(path string) (string, error) {
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

func SanitizeVersionToken(v string) string {
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
