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
	return a.state.schedulingAgentSnapshotsContext(ctx, time.Now().UTC())
}

func (s *stateStore) schedulingAgentSnapshots(now time.Time) []requirements.AgentSnapshot {
	snapshots, _ := s.schedulingAgentSnapshotsContext(context.Background(), now)
	return snapshots
}

func (s *stateStore) schedulingAgentSnapshotsContext(ctx context.Context, now time.Time) ([]requirements.AgentSnapshot, error) {
	busy, err := s.agentJobExecutionStore().ListActiveJobExecutionAgentIDsContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
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
	return snapshots, nil
}
