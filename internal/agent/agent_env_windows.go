//go:build windows

package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func loadAgentPlatformEnv() {
	path := strings.TrimSpace(os.Getenv("CIWI_AGENT_ENV_FILE"))
	if path == "" {
		programData := strings.TrimSpace(os.Getenv("ProgramData"))
		if programData == "" {
			programData = `C:\ProgramData`
		}
		path = filepath.Join(programData, "ciwi-agent", "agent.env")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for k, v := range parseSimpleEnv(string(raw)) {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if os.Getenv(k) != "" {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			slog.Warn("set env from agent env file failed", "key", k, "error", err)
		}
	}
}
