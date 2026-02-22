package agent

import "path/filepath"

func agentWorkDir() string {
	workDir := envOrDefault("CIWI_AGENT_WORKDIR", ".ciwi-agent/work")
	if abs, err := filepath.Abs(workDir); err == nil {
		return abs
	}
	return workDir
}
