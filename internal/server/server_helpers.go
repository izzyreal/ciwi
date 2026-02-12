package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func isValidUpdateStatus(status string) bool {
	switch status {
	case jobStatusRunning, jobStatusSucceeded, jobStatusFailed:
		return true
	default:
		return false
	}
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
	return normalizeCapabilities(merged, merged["os"])
}

func normalizeCapabilities(in map[string]string, osName string) map[string]string {
	out := cloneMap(in)
	if out == nil {
		out = map[string]string{}
	}

	normalizedOS := strings.ToLower(strings.TrimSpace(osName))
	if normalizedOS == "" {
		normalizedOS = strings.ToLower(strings.TrimSpace(out["os"]))
	}
	if normalizedOS != "" {
		out["os"] = normalizedOS
	}

	executor := strings.ToLower(strings.TrimSpace(out["executor"]))
	if executor == "shell" {
		executor = "script"
	}
	if executor != "" {
		out["executor"] = executor
	}

	shell := strings.ToLower(strings.TrimSpace(out["shell"]))
	if shell == "" && executor == "script" {
		shell = defaultShellForOS(normalizedOS)
	}
	if shell != "" {
		out["shell"] = shell
	}

	shellList := parseShellList(out["shells"])
	if len(shellList) == 0 {
		shellList = supportedShellsForOS(normalizedOS)
	}
	if shell != "" {
		seen := false
		for _, s := range shellList {
			if s == shell {
				seen = true
				break
			}
		}
		if !seen {
			shellList = append(shellList, shell)
		}
	}
	if len(shellList) > 0 {
		out["shells"] = strings.Join(shellList, ",")
	}

	return out
}

func parseShellList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		v := strings.ToLower(strings.TrimSpace(part))
		switch v {
		case "posix", "cmd", "powershell":
		default:
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func defaultShellForOS(osName string) string {
	if strings.EqualFold(strings.TrimSpace(osName), "windows") {
		return "cmd"
	}
	return "posix"
}

func supportedShellsForOS(osName string) []string {
	if strings.EqualFold(strings.TrimSpace(osName), "windows") {
		return []string{"cmd", "powershell"}
	}
	return []string{"posix"}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode JSON response", "error", err)
	}
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

func sanitizeMarkerToken(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "\t", "_")
	v = strings.ReplaceAll(v, "\"", "")
	return v
}
