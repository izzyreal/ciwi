package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

func wipeAgentCache(workDir string) (string, error) {
	cacheDir := filepath.Join(workDir, "cache")
	if err := os.RemoveAll(cacheDir); err != nil {
		return "", fmt.Errorf("remove cache dir %q: %w", cacheDir, err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("recreate cache dir %q: %w", cacheDir, err)
	}
	return "cache wipe completed: " + cacheDir, nil
}
