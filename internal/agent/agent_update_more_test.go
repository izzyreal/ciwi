package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentUpdateUtilityWrappers(t *testing.T) {
	if got := expectedAssetName("linux", "amd64"); strings.TrimSpace(got) == "" {
		t.Fatalf("expected known asset name for linux/amd64")
	}
	if got := expectedAssetName("unknown", "unknown"); got != "" {
		t.Fatalf("expected empty asset name for unknown platform, got %q", got)
	}

	if !isVersionNewer("v1.1.0", "v1.0.0") {
		t.Fatalf("expected v1.1.0 to be newer than v1.0.0")
	}
	if isVersionNewer("v1.0.0", "v1.0.0") {
		t.Fatalf("did not expect equal versions to be newer")
	}
	if !isVersionDifferent("v1.0.1", "v1.0.0") {
		t.Fatalf("expected different versions")
	}
	if isVersionDifferent("v1.0.0", "v1.0.0") {
		t.Fatalf("did not expect equal versions to differ")
	}
	if !looksLikeGoRunBinary("/tmp/go-build1234/b001/exe/main") {
		t.Fatalf("expected go-run path detection")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	dst := filepath.Join(tmp, "dst.bin")
	if err := os.WriteFile(src, []byte("copy"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := copyFile(src, dst, 0o700); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(raw) != "copy" {
		t.Fatalf("unexpected copied content: %q", string(raw))
	}
}

func TestRunLaunchctlAndAdHocSignBinary(t *testing.T) {
	t.Setenv("CIWI_LAUNCHCTL_PATH", "/usr/bin/true")
	if err := runLaunchctl("kickstart", "-k", "dummy/service"); err != nil {
		t.Fatalf("runLaunchctl with /usr/bin/true: %v", err)
	}
	t.Setenv("CIWI_LAUNCHCTL_PATH", "/usr/bin/false")
	if err := runLaunchctl("kickstart", "-k", "dummy/service"); err == nil {
		t.Fatalf("expected runLaunchctl failure with /usr/bin/false")
	}

	if err := adHocSignBinary("   "); err == nil {
		t.Fatalf("expected empty path error")
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(bin, []byte("payload"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	t.Setenv("CIWI_CODESIGN_PATH", "/usr/bin/true")
	if err := adHocSignBinary(bin); err != nil {
		t.Fatalf("adHocSignBinary with /usr/bin/true: %v", err)
	}
	t.Setenv("CIWI_CODESIGN_PATH", "/usr/bin/false")
	if err := adHocSignBinary(bin); err == nil {
		t.Fatalf("expected adHocSignBinary failure with /usr/bin/false")
	}
}

func TestDarwinUpdaterConfigGates(t *testing.T) {
	t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "")
	t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "")
	t.Setenv("CIWI_AGENT_UPDATER_LABEL", "")
	if err := stageAndTriggerDarwinUpdater("v1.0.0", "asset", "/tmp/target", "/tmp/staged"); err == nil || !strings.Contains(err.Error(), "missing launchd updater configuration") {
		t.Fatalf("expected missing launchd config error, got %v", err)
	}

	// Non-darwin runtimes should never advertise launchd updater readiness.
	if runtime.GOOS != "darwin" {
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "agent")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "/tmp/agent.plist")
		t.Setenv("CIWI_AGENT_UPDATER_LABEL", "updater")
		if hasDarwinUpdaterConfig() {
			t.Fatalf("expected hasDarwinUpdaterConfig=false on non-darwin")
		}
	}
	if runtime.GOOS == "darwin" {
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "agent")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "/tmp/agent.plist")
		t.Setenv("CIWI_AGENT_UPDATER_LABEL", "updater")
		if !hasDarwinUpdaterConfig() {
			t.Fatalf("expected hasDarwinUpdaterConfig=true on darwin with launchd env")
		}
	}
}

func TestStageAndTriggerDarwinUpdater(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only update staging flow")
	}
	tmp := t.TempDir()
	workDir := filepath.Join(tmp, "work")
	manifestPath := filepath.Join(workDir, "updates", "pending.json")
	targetBinary := filepath.Join(tmp, "ciwi-agent")
	stagedBinary := filepath.Join(tmp, "ciwi-agent.new")
	launchctlPath := filepath.Join(tmp, "launchctl.sh")

	if err := os.WriteFile(targetBinary, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target binary: %v", err)
	}
	if err := os.WriteFile(stagedBinary, []byte("new"), 0o755); err != nil {
		t.Fatalf("write staged binary: %v", err)
	}
	if err := os.WriteFile(launchctlPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write launchctl script: %v", err)
	}

	t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "io.github.ciwi.agent")
	t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", filepath.Join(tmp, "agent.plist"))
	t.Setenv("CIWI_AGENT_UPDATER_LABEL", "io.github.ciwi.agent-updater")
	t.Setenv("CIWI_AGENT_UPDATER_PLIST", filepath.Join(tmp, "updater.plist"))
	t.Setenv("CIWI_AGENT_WORKDIR", workDir)
	t.Setenv("CIWI_AGENT_UPDATE_MANIFEST", manifestPath)
	t.Setenv("CIWI_DARWIN_ADHOC_SIGN", "false")
	t.Setenv("CIWI_LAUNCHCTL_PATH", launchctlPath)

	if err := stageAndTriggerDarwinUpdater("v1.2.3", "ciwi-darwin", targetBinary, stagedBinary); err != nil {
		t.Fatalf("stageAndTriggerDarwinUpdater success: %v", err)
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected update manifest written: %v", err)
	}

	if err := os.WriteFile(stagedBinary, []byte("new-2"), 0o755); err != nil {
		t.Fatalf("rewrite staged binary: %v", err)
	}
	if err := os.WriteFile(launchctlPath, []byte("#!/bin/sh\necho fail >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("rewrite launchctl script: %v", err)
	}
	if err := stageAndTriggerDarwinUpdater("v1.2.4", "ciwi-darwin", targetBinary, stagedBinary); err == nil {
		t.Fatalf("expected trigger failure")
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected manifest removed on trigger failure, stat err=%v", err)
	}
}
