package agent

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestSelfUpdateAndRestartNoopWhenTargetMissingOrSame(t *testing.T) {
	exePath, _ := os.Executable()
	if looksLikeGoRunBinary(exePath) {
		t.Skip("test runtime is go-run style; self-update guard triggers before noop checks")
	}
	if err := selfUpdateAndRestart(context.Background(), "", "izzyreal/ciwi", "https://api.github.com", nil); err != nil {
		t.Fatalf("expected empty target to no-op, got %v", err)
	}
	if err := selfUpdateAndRestart(context.Background(), currentVersion(), "izzyreal/ciwi", "https://api.github.com", nil); err != nil {
		t.Fatalf("expected same-version target to no-op, got %v", err)
	}
}

func TestSelfUpdateAndRestartServiceModeGuard(t *testing.T) {
	target := "v999.0.0"
	if strings.EqualFold(strings.TrimSpace(target), strings.TrimSpace(currentVersion())) {
		target = "v999.0.1"
	}

	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "")
	case "darwin":
		t.Setenv("CIWI_AGENT_LAUNCHD_LABEL", "")
		t.Setenv("CIWI_AGENT_LAUNCHD_PLIST", "")
		t.Setenv("CIWI_AGENT_UPDATER_LABEL", "")
	case "windows":
		t.Setenv("CIWI_AGENT_WINDOWS_SERVICE_NAME", "")
	}

	err := selfUpdateAndRestart(context.Background(), target, "izzyreal/ciwi", "https://api.github.com", nil)
	if err == nil {
		t.Fatalf("expected self-update service-mode guard error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "self-update") {
		t.Fatalf("expected self-update guard wording, got %v", err)
	}
}
