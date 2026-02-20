package agent

import "testing"

func TestSelfUpdateServiceModeReasonForDarwinLaunchdEnv(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "CIWI_AGENT_LAUNCHD_LABEL":
			return "nl.izmar.ciwi.agent"
		case "CIWI_AGENT_LAUNCHD_PLIST":
			return "/Users/test/Library/LaunchAgents/nl.izmar.ciwi.agent.plist"
		case "CIWI_AGENT_UPDATER_LABEL":
			return "nl.izmar.ciwi.agent-updater"
		default:
			return ""
		}
	}
	if reason := selfUpdateServiceModeReasonFor("darwin", getenv); reason != "" {
		t.Fatalf("expected darwin launchd env to allow self-update, got reason=%q", reason)
	}
}

func TestSelfUpdateServiceModeReasonForDarwinMissingEnv(t *testing.T) {
	getenv := func(string) string { return "" }
	reason := selfUpdateServiceModeReasonFor("darwin", getenv)
	if reason == "" {
		t.Fatalf("expected missing darwin launchd env to disable self-update")
	}
}

func TestSelfUpdateServiceModeReasonForLinuxAndUnknown(t *testing.T) {
	linuxNoService := selfUpdateServiceModeReasonFor("linux", func(string) string { return "" })
	if linuxNoService == "" {
		t.Fatalf("expected linux without INVOCATION_ID to disable self-update")
	}

	linuxService := selfUpdateServiceModeReasonFor("linux", func(key string) string {
		if key == "INVOCATION_ID" {
			return "abc"
		}
		return ""
	})
	if linuxService != "" {
		t.Fatalf("expected linux with INVOCATION_ID to allow self-update, got %q", linuxService)
	}

	unknown := selfUpdateServiceModeReasonFor("plan9", func(string) string { return "" })
	if unknown == "" {
		t.Fatalf("expected unsupported runtime to disable self-update")
	}
}
