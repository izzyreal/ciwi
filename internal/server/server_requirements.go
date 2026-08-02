package server

import (
	"context"
	"time"

	"github.com/izzyreal/ciwi/internal/requirements"
)

type schedulingAgentSourceAdapter struct{ state *stateStore }

func (a schedulingAgentSourceAdapter) ListSchedulingAgents(ctx context.Context) ([]requirements.AgentSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.state.schedulingAgentSnapshots(time.Now().UTC()), nil
}

func (s *stateStore) schedulingAgentSnapshots(now time.Time) []requirements.AgentSnapshot {
	busy, err := s.agentJobExecutionStore().ListActiveJobExecutionAgentIDs()
	if err != nil {
		busy = map[string]bool{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshots := make([]requirements.AgentSnapshot, 0, len(s.agents))
	for id, agent := range s.agents {
		snapshots = append(snapshots, requirements.AgentSnapshot{
			ID: id, OS: agent.OS, Arch: agent.Arch, Capabilities: cloneMap(agent.Capabilities),
			Freshness: classifyAgentFreshness(agent.LastSeenUTC, now), Authorized: agent.Authorized,
			Deactivated: agent.Deactivated, Updating: agent.UpdateInProgress || s.agentLeasePendingUpdateReasonLocked(id, agent) != "",
			Busy: busy[id],
		})
	}
	return snapshots
}
