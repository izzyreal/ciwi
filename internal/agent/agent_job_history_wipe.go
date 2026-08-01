package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

func wipeAgentJobHistory(workDir string) (string, error) {
	if _, err := os.Stat(workDir); err != nil {
		return "", fmt.Errorf("read work dir %q: %w", workDir, err)
	}
	workspaceRoot := filepath.Join(workDir, "workspaces")
	workspaceEntries, err := os.ReadDir(workspaceRoot)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read workspaces dir %q: %w", workspaceRoot, err)
	}
	if err := removeAllWithRetry(workspaceRoot); err != nil {
		return "", fmt.Errorf("remove workspace root %q: %w", workspaceRoot, err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return "", fmt.Errorf("recreate workspace root %q: %w", workspaceRoot, err)
	}
	return fmt.Sprintf("local job history wipe completed: removed=%d workspaces", len(workspaceEntries)), nil
}
