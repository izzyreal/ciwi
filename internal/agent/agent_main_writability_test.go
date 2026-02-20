package agent

import (
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
		t.Setenv("CIWI_AGENT_UPDATER_LABEL", "")
	}
	warn := selfUpdateWritabilityWarning()
	if strings.TrimSpace(warn) == "" {
		t.Fatalf("expected writability preflight warning when service-mode requirement is unmet")
	}
}
