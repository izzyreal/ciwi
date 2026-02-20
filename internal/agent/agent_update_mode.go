package agent

import (
	"os"
	"runtime"
	"strings"
)

func selfUpdateServiceModeReason() string {
	return selfUpdateServiceModeReasonFor(runtime.GOOS, func(key string) string {
		return os.Getenv(key)
	})
}

func selfUpdateServiceModeReasonFor(goos string, getenv func(string) string) string {
	switch strings.TrimSpace(goos) {
	case "linux":
		if strings.TrimSpace(getenv("INVOCATION_ID")) != "" {
			return ""
		}
		return "agent is not running as a service; self-update disabled"
	case "darwin":
		// macOS installer wires launchd update support via these env vars.
		if strings.TrimSpace(getenv("CIWI_AGENT_LAUNCHD_LABEL")) != "" &&
			strings.TrimSpace(getenv("CIWI_AGENT_LAUNCHD_PLIST")) != "" &&
			strings.TrimSpace(getenv("CIWI_AGENT_UPDATER_LABEL")) != "" {
			return ""
		}
		return "agent is not running as a service; self-update disabled"
	case "windows":
		active, _ := windowsServiceInfo()
		if active {
			return ""
		}
		return "agent is not running as a service; self-update disabled"
	default:
		return "agent is not running as a supported service manager; self-update disabled"
	}
}
