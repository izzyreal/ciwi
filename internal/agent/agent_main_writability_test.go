package agent

import (
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestSelfUpdateWritabilityWarningServiceModeReason(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "")
	case "darwin":
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "")
	}
	warn := selfUpdateWritabilityWarning()
	if strings.TrimSpace(warn) == "" {
		t.Fatalf("expected writability preflight warning when service-mode requirement is unmet")
	}
}

func TestStartupHeartbeatGreeting(t *testing.T) {
	msg := startupHeartbeatGreeting()
	if !strings.Contains(msg, "ciwi agent has (re)started") {
		t.Fatalf("unexpected startup heartbeat greeting: %q", msg)
	}
}

func TestSelfUpdateWritabilityWarningExecutableResolutionError(t *testing.T) {
	origExecutable := selfUpdateExecutablePathFn
	origOpen := selfUpdateOpenFileFn
	t.Cleanup(func() {
		selfUpdateExecutablePathFn = origExecutable
		selfUpdateOpenFileFn = origOpen
	})

	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "service-context")
	case "darwin":
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "nl.izmar.ciwi.agent")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "/tmp/nl.izmar.ciwi.agent.plist")
	}

	selfUpdateExecutablePathFn = func() (string, error) { return "", errors.New("boom") }
	selfUpdateOpenFileFn = func(string, int, fs.FileMode) (*os.File, error) {
		t.Fatalf("open should not be attempted when executable lookup fails")
		return nil, nil
	}

	warn := selfUpdateWritabilityWarning()
	if !strings.Contains(warn, "cannot resolve executable path: boom") {
		t.Fatalf("unexpected warning: %q", warn)
	}
}

func TestSelfUpdateWritabilityWarningGoRunBinary(t *testing.T) {
	origExecutable := selfUpdateExecutablePathFn
	origOpen := selfUpdateOpenFileFn
	t.Cleanup(func() {
		selfUpdateExecutablePathFn = origExecutable
		selfUpdateOpenFileFn = origOpen
	})

	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "service-context")
	case "darwin":
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "nl.izmar.ciwi.agent")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "/tmp/nl.izmar.ciwi.agent.plist")
	}

	selfUpdateExecutablePathFn = func() (string, error) { return "/tmp/go-build1234/b001/exe/main", nil }
	selfUpdateOpenFileFn = func(string, int, fs.FileMode) (*os.File, error) {
		t.Fatalf("open should not be attempted for go-run path")
		return nil, nil
	}

	warn := selfUpdateWritabilityWarning()
	if !strings.Contains(warn, "running via go run binary path; self-update is unavailable") {
		t.Fatalf("unexpected warning: %q", warn)
	}
}

func TestSelfUpdateWritabilityWarningOpenFailure(t *testing.T) {
	origExecutable := selfUpdateExecutablePathFn
	origOpen := selfUpdateOpenFileFn
	t.Cleanup(func() {
		selfUpdateExecutablePathFn = origExecutable
		selfUpdateOpenFileFn = origOpen
	})

	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "service-context")
	case "darwin":
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "nl.izmar.ciwi.agent")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "/tmp/nl.izmar.ciwi.agent.plist")
	}

	selfUpdateExecutablePathFn = func() (string, error) { return "/opt/ciwi/ciwi-agent", nil }
	selfUpdateOpenFileFn = func(string, int, fs.FileMode) (*os.File, error) {
		return nil, fs.ErrPermission
	}

	warn := selfUpdateWritabilityWarning()
	if !strings.Contains(warn, "binary path is not writable by current user (/opt/ciwi/ciwi-agent):") {
		t.Fatalf("unexpected warning: %q", warn)
	}
	if !strings.Contains(warn, "permission denied") {
		t.Fatalf("expected permission detail in warning, got %q", warn)
	}
}
