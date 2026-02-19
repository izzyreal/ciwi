package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const agentNonServiceUpdateErrorMarker = "agent is not running as a service; self-update disabled"

type serverUpdateCapability struct {
	Mode      string
	Supported bool
	Reason    string
}

func detectServerUpdateCapability() serverUpdateCapability {
	if isServerRunningInDevMode() {
		return serverUpdateCapability{
			Mode:      "dev",
			Supported: false,
			Reason:    "Running in dev mode. Updates disabled.",
		}
	}
	if isServerRunningAsService() {
		return serverUpdateCapability{
			Mode:      "service",
			Supported: true,
		}
	}
	return serverUpdateCapability{
		Mode:      "standalone",
		Supported: false,
		Reason:    "Server is not running as a service. Updates disabled. Install updates manually. See README.",
	}
}

func isServerRunningInDevMode() bool {
	if strings.EqualFold(strings.TrimSpace(currentVersion()), "dev") {
		return true
	}
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	exePath, _ = filepath.Abs(exePath)
	return looksLikeGoRunBinary(exePath)
}

func isServerRunningAsService() bool {
	switch runtime.GOOS {
	case "linux":
		return strings.TrimSpace(os.Getenv("INVOCATION_ID")) != ""
	case "darwin":
		return strings.TrimSpace(os.Getenv("LAUNCH_JOB_LABEL")) != ""
	case "windows":
		return strings.TrimSpace(envOrDefault("CIWI_SERVER_WINDOWS_SERVICE_NAME", "")) != ""
	default:
		return false
	}
}

func (s *stateStore) listAgentsBlockedOnNonServiceSelfUpdate() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.agents))
	for agentID, state := range s.agents {
		errText := strings.TrimSpace(state.UpdateLastError)
		if errText == "" {
			continue
		}
		if strings.Contains(strings.ToLower(errText), strings.ToLower(agentNonServiceUpdateErrorMarker)) {
			out = append(out, strings.TrimSpace(agentID))
		}
	}
	return out
}
