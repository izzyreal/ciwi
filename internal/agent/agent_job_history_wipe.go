package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func wipeAgentJobHistory(workDir string) (string, error) {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return "", fmt.Errorf("read work dir %q: %w", workDir, err)
	}
	removed := 0
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if name == "workspaces" {
			workspaceRoot := filepath.Join(workDir, name)
			workspaceEntries, readErr := os.ReadDir(workspaceRoot)
			if readErr != nil {
				return "", fmt.Errorf("read workspaces dir %q: %w", workspaceRoot, readErr)
			}
			if err := os.RemoveAll(workspaceRoot); err != nil {
				return "", fmt.Errorf("remove workspace root %q: %w", workspaceRoot, err)
			}
			if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
				return "", fmt.Errorf("recreate workspace root %q: %w", workspaceRoot, err)
			}
			removed += len(workspaceEntries)
			continue
		}
		if !strings.HasPrefix(name, "job-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(workDir, name)); err != nil {
			return "", fmt.Errorf("remove legacy job dir %q: %w", name, err)
		}
		removed++
	}
	return fmt.Sprintf("local job history wipe completed: removed=%d workspaces", removed), nil
}
