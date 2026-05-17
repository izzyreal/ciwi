//go:build darwin

package agent

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func loadAgentPlatformEnv() {
	path := strings.TrimSpace(os.Getenv("CIWI_AGENT_ENV_FILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return
		}
		path = filepath.Join(home, "Library", "Application Support", "ciwi", "agent.env")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for k, v := range parseSimpleEnv(string(raw)) {
		if strings.TrimSpace(k) == "" || os.Getenv(k) != "" {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			slog.Warn("set env from agent env file failed", "key", k, "error", err)
		}
	}
	configureDarwinAgentFileLogging()
}

func configureDarwinAgentFileLogging() {
	path := strings.TrimSpace(os.Getenv("CIWI_AGENT_LOG_FILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return
		}
		path = filepath.Join(home, "Library", "Logs", "ciwi", "agent.err.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Warn("create agent log directory failed", "path", filepath.Dir(path), "error", err)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		slog.Warn("open agent log file failed", "path", path, "error", err)
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, f), nil)))
}
