package agent

import (
	"os"
	"runtime"
	"strings"
)

func selfUpdateServiceModeReason() string {
	switch runtime.GOOS {
	case "linux":
		if strings.TrimSpace(os.Getenv("INVOCATION_ID")) != "" {
			return ""
		}
		return "agent is not running as a service; self-update disabled"
	case "darwin":
		if strings.TrimSpace(os.Getenv("LAUNCH_JOB_LABEL")) != "" {
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
