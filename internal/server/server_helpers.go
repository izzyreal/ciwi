package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/httpx"
)

func isValidUpdateStatus(status string) bool {
	return protocol.IsValidJobExecutionUpdateStatus(status)
}

func resolveConfigPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("config_path must be relative")
	}
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		return "", fmt.Errorf("config_path is invalid")
	}
	if strings.HasPrefix(cleanPath, "..") {
		return "", fmt.Errorf("config_path must stay within working directory")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Join(cwd, cleanPath), nil
}

func renderTemplate(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeCapabilities(agent agentState, override map[string]string) map[string]string {
	merged := map[string]string{"os": agent.OS, "arch": agent.Arch}
	for k, v := range agent.Capabilities {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, healthzResponse{Status: "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	httpx.WriteJSON(w, status, v)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
