package darwinupdater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type stagedManifest struct {
	VersionUTC      string `json:"version_utc"`
	TargetVersion   string `json:"target_version"`
	AssetName       string `json:"asset_name"`
	TargetBinary    string `json:"target_binary"`
	StagedBinary    string `json:"staged_binary"`
	StagedSHA256    string `json:"staged_sha256"`
	AgentLabel      string `json:"agent_label"`
	AgentPlist      string `json:"agent_plist"`
	AgentPID        int    `json:"agent_pid"`
	RequestedAtUTC  string `json:"requested_at_utc"`
	UpdaterLabel    string `json:"updater_label,omitempty"`
	UpdaterPlist    string `json:"updater_plist,omitempty"`
	TriggerSourceID string `json:"trigger_source_id,omitempty"`
}

func BuildManifest(targetVersion, assetName, targetBinary, stagedBinary, stagedSHA256, agentLabel, agentPlist, updaterLabel, updaterPlist, triggerSourceID string, agentPID int) ([]byte, error) {
	m := stagedManifest{
		VersionUTC:      time.Now().UTC().Format(time.RFC3339Nano),
		TargetVersion:   strings.TrimSpace(targetVersion),
		AssetName:       strings.TrimSpace(assetName),
		TargetBinary:    strings.TrimSpace(targetBinary),
		StagedBinary:    strings.TrimSpace(stagedBinary),
		StagedSHA256:    strings.ToLower(strings.TrimSpace(stagedSHA256)),
		AgentLabel:      strings.TrimSpace(agentLabel),
		AgentPlist:      strings.TrimSpace(agentPlist),
		UpdaterLabel:    strings.TrimSpace(updaterLabel),
		UpdaterPlist:    strings.TrimSpace(updaterPlist),
		TriggerSourceID: strings.TrimSpace(triggerSourceID),
		AgentPID:        agentPID,
		RequestedAtUTC:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	return json.MarshalIndent(m, "", "  ")
}

func RunApplyStagedAgent(args []string) error {
	fs := flag.NewFlagSet("apply-staged-agent-update", flag.ContinueOnError)
	manifestPath := fs.String("manifest", strings.TrimSpace(envOrDefault("CIWI_AGENT_UPDATE_MANIFEST", "")), "path to staged update manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*manifestPath) == "" {
		return fmt.Errorf("apply-staged-agent-update requires --manifest or CIWI_AGENT_UPDATE_MANIFEST")
	}

	manifest, err := readManifest(*manifestPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(manifest.TargetBinary) == "" {
		return fmt.Errorf("manifest missing target_binary")
	}
	if strings.TrimSpace(manifest.StagedBinary) == "" {
		return fmt.Errorf("manifest missing staged_binary")
	}
	if strings.TrimSpace(manifest.StagedSHA256) == "" {
		return fmt.Errorf("manifest missing staged_sha256")
	}
	if strings.TrimSpace(manifest.AgentLabel) == "" {
		return fmt.Errorf("manifest missing agent_label")
	}
	if strings.TrimSpace(manifest.AgentPlist) == "" {
		return fmt.Errorf("manifest missing agent_plist")
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

	launchctlPath := strings.TrimSpace(envOrDefault("CIWI_LAUNCHCTL_PATH", "/bin/launchctl"))
	if launchctlPath == "" {
		launchctlPath = "/bin/launchctl"
	}
	uid := os.Getuid()
	domain := "gui/" + strconv.Itoa(uid)
	service := domain + "/" + manifest.AgentLabel

	_ = runCmd(launchctlPath, "bootout", service)
	_ = runCmd(launchctlPath, "bootout", domain, manifest.AgentPlist)
	_ = runCmd(launchctlPath, "disable", service)
	_ = runCmd(launchctlPath, "enable", service)

	if manifest.AgentPID > 0 {
		_ = waitForProcessExit(manifest.AgentPID, 45*time.Second)
	}

	backupPath := manifest.TargetBinary + ".prev"
	_ = os.Remove(backupPath)
	if err := os.Rename(manifest.TargetBinary, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(manifest.StagedBinary, manifest.TargetBinary); err != nil {
		_ = os.Rename(backupPath, manifest.TargetBinary)
		return fmt.Errorf("move staged binary into place: %w", err)
	}
	if err := os.Chmod(manifest.TargetBinary, 0o755); err != nil {
		_ = os.Rename(manifest.TargetBinary, manifest.StagedBinary)
		_ = os.Rename(backupPath, manifest.TargetBinary)
		return fmt.Errorf("chmod target binary: %w", err)
	}
	if strings.TrimSpace(envOrDefault("CIWI_DARWIN_ADHOC_SIGN", "true")) != "false" {
		if err := adHocSignBinary(manifest.TargetBinary); err != nil {
			_ = os.Rename(manifest.TargetBinary, manifest.StagedBinary)
			_ = os.Rename(backupPath, manifest.TargetBinary)
			return fmt.Errorf("ad-hoc sign target binary: %w", err)
		}
	}
	_ = os.Remove(*manifestPath)

	if err := runCmd(launchctlPath, "bootstrap", domain, manifest.AgentPlist); err != nil && !isAlreadyLoadedErr(err) {
		return fmt.Errorf("bootstrap agent launchd plist: %w", err)
	}
	if err := runCmd(launchctlPath, "kickstart", "-k", service); err != nil {
		return fmt.Errorf("kickstart agent launchd service: %w", err)
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

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func waitForProcessExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for pid %d", pid)
		}
		p, err := os.FindProcess(pid)
		if err != nil {
			return nil
		}
		if err := p.Signal(syscall.Signal(0)); err != nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func isAlreadyLoadedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already loaded")
}

func adHocSignBinary(path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("empty path")
	}
	codesignPath := strings.TrimSpace(envOrDefault("CIWI_CODESIGN_PATH", "/usr/bin/codesign"))
	if codesignPath == "" {
		codesignPath = "/usr/bin/codesign"
	}
	return runCmd(codesignPath, "--force", "--sign", "-", p)
}
