package server

import (
	"github.com/izzyreal/ciwi/internal/requirements"
)

func diagnoseUnmetRequirements(required map[string]string, agents map[string]agentState) []string {
	snapshots := make([]requirements.AgentSnapshot, 0, len(agents))
	for id, agent := range agents {
		snapshots = append(snapshots, requirements.AgentSnapshot{
			ID:           id,
			OS:           agent.OS,
			Arch:         agent.Arch,
			Capabilities: cloneMap(agent.Capabilities),
		})
	}
	return requirements.DiagnoseUnmetRequirements(required, snapshots)
}
