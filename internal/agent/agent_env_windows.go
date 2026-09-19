//go:build windows

package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func loadAgentPlatformEnv() {
	// Configure after loading overrides, including when the env file is absent.
	defer configureWindowsAgentFileLogging()
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
		if os.Getenv(k) != "" && !envFileShouldOverrideExisting(k) {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			slog.Warn("set env from agent env file failed", "key", k, "error", err)
		}
	}
}

func configureWindowsAgentFileLogging() {
	path := strings.TrimSpace(os.Getenv("CIWI_AGENT_LOG_FILE"))
	if path == "" {
		programData := strings.TrimSpace(os.Getenv("ProgramData"))
		if programData == "" {
			programData = `C:\ProgramData`
		}
		path = filepath.Join(programData, "ciwi-agent", "logs", "agent.log")
	}
	logger, _, err := newAgentFileLogger(path, os.Stderr, os.Getenv("CIWI_LOG_LEVEL"))
	if err != nil {
		slog.Error("configure agent file logging failed", "path", path, "error", err)
		return
	}
	// The log stays open for the lifetime of this agent process.
	slog.SetDefault(logger)
	slog.Info("agent file logging enabled", "path", path)
}
