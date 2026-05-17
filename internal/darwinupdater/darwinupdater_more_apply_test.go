package darwinupdater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunApplyStagedAgentBundleInstallUsesServiceHelper(t *testing.T) {
	tmp := t.TempDir()
	targetBundle := filepath.Join(tmp, "installed", "CiwiAgent.app")
	targetBinary := filepath.Join(targetBundle, "Contents", "MacOS", "ciwi")
	targetHelper := filepath.Join(targetBundle, "Contents", "MacOS", "ciwi-service")
	stagedBundle := filepath.Join(tmp, "staged", "CiwiAgent.app")
	stagedBinary := filepath.Join(stagedBundle, "Contents", "MacOS", "ciwi")
	stagedHelper := filepath.Join(stagedBundle, "Contents", "MacOS", "ciwi-service")
	manifestPath := filepath.Join(tmp, "pending.json")
	launchctlPath := filepath.Join(tmp, "launchctl.sh")
	launchctlLog := filepath.Join(tmp, "launchctl.log")
	serviceLog := filepath.Join(tmp, "service.log")
	agentPlist := filepath.Join(tmp, "agent.plist")

	for _, path := range []string{filepath.Dir(targetBinary), filepath.Dir(stagedBinary)} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir bundle dir: %v", err)
		}
	}
	if err := os.WriteFile(targetBinary, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write target binary: %v", err)
	}
	if err := os.WriteFile(stagedBinary, []byte("new-binary"), 0o755); err != nil {
		t.Fatalf("write staged binary: %v", err)
	}
	if err := os.WriteFile(targetHelper, []byte("#!/bin/sh\nexit 99\n"), 0o755); err != nil {
		t.Fatalf("write target helper: %v", err)
	}
	serviceScript := "#!/bin/sh\n" +
		"echo \"$@\" >> \"" + serviceLog + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(stagedHelper, []byte(serviceScript), 0o755); err != nil {
		t.Fatalf("write staged helper: %v", err)
	}
	if err := os.WriteFile(agentPlist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write agent plist: %v", err)
	}
	sum, err := fileSHA256(stagedBinary)
	if err != nil {
		t.Fatalf("hash staged binary: %v", err)
	}
	writeDarwinManifest(t, manifestPath, stagedManifest{
		TargetVersion: "v1.2.3",
		TargetBinary:  targetBinary,
		StagedBinary:  stagedBinary,
		StagedSHA256:  sum,
		TargetBundle:  targetBundle,
		StagedBundle:  stagedBundle,
		AgentLabel:    "io.github.ciwi.agent",
		AgentPlist:    agentPlist,
	})

	launchctlScript := "#!/bin/sh\n" +
		"echo \"$@\" >> \"" + launchctlLog + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(launchctlPath, []byte(launchctlScript), 0o755); err != nil {
		t.Fatalf("write launchctl script: %v", err)
	}
	t.Setenv("CIWI_LAUNCHCTL_PATH", launchctlPath)

	if err := RunApplyStagedAgent([]string{"--manifest", manifestPath}); err != nil {
		t.Fatalf("RunApplyStagedAgent bundle install: %v", err)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected manifest removed, stat err=%v", err)
	}
	gotBinary, err := os.ReadFile(targetBinary)
	if err != nil {
		t.Fatalf("read swapped target binary: %v", err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("unexpected swapped binary content: %q", string(gotBinary))
	}
	serviceRaw, err := os.ReadFile(serviceLog)
	if err != nil {
		t.Fatalf("read service log: %v", err)
	}
	if !strings.Contains(string(serviceRaw), "register-agent") {
		t.Fatalf("expected bundled service helper registration, got %q", string(serviceRaw))
	}
	launchctlRaw, err := os.ReadFile(launchctlLog)
	if err != nil {
		t.Fatalf("read launchctl log: %v", err)
	}
	logText := string(launchctlRaw)
	if !strings.Contains(logText, "kickstart -k") {
		t.Fatalf("expected kickstart in launchctl log, got %q", logText)
	}
	if strings.Contains(logText, "bootstrap") {
		t.Fatalf("did not expect bootstrap for bundled service path, got %q", logText)
	}
}

func TestRunApplyStagedAgentBundleSwapRollbackOnMissingStagedBundle(t *testing.T) {
	tmp := t.TempDir()
	targetBundle := filepath.Join(tmp, "installed", "CiwiAgent.app")
	targetBinary := filepath.Join(targetBundle, "Contents", "MacOS", "ciwi")
	hashInput := filepath.Join(tmp, "hash-input", "ciwi")
	manifestPath := filepath.Join(tmp, "pending.json")
	launchctlPath := filepath.Join(tmp, "launchctl.sh")
	agentPlist := filepath.Join(tmp, "agent.plist")

	if err := os.MkdirAll(filepath.Dir(targetBinary), 0o755); err != nil {
		t.Fatalf("mkdir target bundle dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(hashInput), 0o755); err != nil {
		t.Fatalf("mkdir hash input dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "missing"), 0o755); err != nil {
		t.Fatalf("mkdir missing parent dir: %v", err)
	}
	if err := os.WriteFile(targetBinary, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write target binary: %v", err)
	}
	if err := os.WriteFile(hashInput, []byte("new-binary"), 0o755); err != nil {
		t.Fatalf("write hash input binary: %v", err)
	}
	if err := os.WriteFile(agentPlist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write agent plist: %v", err)
	}
	sum, err := fileSHA256(hashInput)
	if err != nil {
		t.Fatalf("hash input binary: %v", err)
	}
	writeDarwinManifest(t, manifestPath, stagedManifest{
		TargetVersion: "v1.2.3",
		TargetBinary:  targetBinary,
		StagedBinary:  hashInput,
		StagedSHA256:  sum,
		TargetBundle:  targetBundle,
		StagedBundle:  filepath.Join(tmp, "missing", "CiwiAgent.app"),
		AgentLabel:    "io.github.ciwi.agent",
		AgentPlist:    agentPlist,
	})

	if err := os.WriteFile(launchctlPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write launchctl script: %v", err)
	}
	t.Setenv("CIWI_LAUNCHCTL_PATH", launchctlPath)

	err = RunApplyStagedAgent([]string{"--manifest", manifestPath})
	if err == nil || !strings.Contains(err.Error(), "move staged bundle into place") {
		t.Fatalf("expected staged bundle move failure, got %v", err)
	}
	gotBinary, readErr := os.ReadFile(targetBinary)
	if readErr != nil {
		t.Fatalf("read target binary after rollback: %v", readErr)
	}
	if string(gotBinary) != "old-binary" {
		t.Fatalf("expected rollback to restore original bundle, got %q", string(gotBinary))
	}
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest to remain for failed apply, stat err=%v", err)
	}
}
