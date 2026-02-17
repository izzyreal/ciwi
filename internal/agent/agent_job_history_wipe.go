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
		if name == "" || !strings.HasPrefix(name, "job-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(workDir, name)); err != nil {
			return "", fmt.Errorf("remove job dir %q: %w", name, err)
		}
		removed++
	}
	return fmt.Sprintf("local job history wipe completed: removed=%d", removed), nil
}
