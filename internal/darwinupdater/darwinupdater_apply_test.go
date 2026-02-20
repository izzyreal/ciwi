package darwinupdater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDarwinManifest(t *testing.T, path string, m stagedManifest) {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestRunApplyStagedAgentSuccessWithLaunchctlScript(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "ciwi-agent")
	staged := filepath.Join(tmp, "ciwi-agent.new")
	manifestPath := filepath.Join(tmp, "pending.json")
	logPath := filepath.Join(tmp, "launchctl.log")
	launchctlPath := filepath.Join(tmp, "launchctl.sh")
	agentPlist := filepath.Join(tmp, "agent.plist")
	agentLabel := "io.github.ciwi.agent"

	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	if err := os.WriteFile(agentPlist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write agent plist: %v", err)
	}
	sum, err := fileSHA256(staged)
	if err != nil {
		t.Fatalf("hash staged: %v", err)
	}
	writeDarwinManifest(t, manifestPath, stagedManifest{
		TargetVersion: "v1.2.3",
		TargetBinary:  target,
		StagedBinary:  staged,
		StagedSHA256:  sum,
		AgentLabel:    agentLabel,
		AgentPlist:    agentPlist,
	})

	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"" + logPath + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(launchctlPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write launchctl script: %v", err)
	}

	t.Setenv("CIWI_LAUNCHCTL_PATH", launchctlPath)
	t.Setenv("CIWI_DARWIN_ADHOC_SIGN", "false")

	if err := RunApplyStagedAgent([]string{"--manifest", manifestPath}); err != nil {
		t.Fatalf("RunApplyStagedAgent: %v", err)
	}

	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(gotTarget) != "new" {
		t.Fatalf("expected target replaced by staged binary, got %q", string(gotTarget))
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected manifest removed, stat err=%v", err)
	}

	logRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read launchctl log: %v", err)
	}
	logText := string(logRaw)
	if !strings.Contains(logText, "bootout") || !strings.Contains(logText, "bootstrap") || !strings.Contains(logText, "kickstart -k") {
		t.Fatalf("unexpected launchctl call log: %q", logText)
	}
}

func TestRunApplyStagedAgentAllowsBootstrapAlreadyLoaded(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "ciwi-agent")
	staged := filepath.Join(tmp, "ciwi-agent.new")
	manifestPath := filepath.Join(tmp, "pending.json")
	logPath := filepath.Join(tmp, "launchctl.log")
	launchctlPath := filepath.Join(tmp, "launchctl.sh")
	agentPlist := filepath.Join(tmp, "agent.plist")
	agentLabel := "io.github.ciwi.agent"

	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatalf("write staged: %v", err)
	}
	if err := os.WriteFile(agentPlist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatalf("write agent plist: %v", err)
	}
	sum, err := fileSHA256(staged)
	if err != nil {
		t.Fatalf("hash staged: %v", err)
	}
	writeDarwinManifest(t, manifestPath, stagedManifest{
		TargetVersion: "v1.2.3",
		TargetBinary:  target,
		StagedBinary:  staged,
		StagedSHA256:  sum,
		AgentLabel:    agentLabel,
		AgentPlist:    agentPlist,
	})

	// Return a bootstrap error containing "already loaded"; updater should treat it as non-fatal.
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"" + logPath + "\"\n" +
		"if [ \"$1\" = \"bootstrap\" ]; then\n" +
		"  echo \"service already loaded\" >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(launchctlPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write launchctl script: %v", err)
	}

	t.Setenv("CIWI_LAUNCHCTL_PATH", launchctlPath)
	t.Setenv("CIWI_DARWIN_ADHOC_SIGN", "false")

	if err := RunApplyStagedAgent([]string{"--manifest", manifestPath}); err != nil {
		t.Fatalf("RunApplyStagedAgent with bootstrap already loaded: %v", err)
	}
}
