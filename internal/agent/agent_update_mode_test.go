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
