package server

import (
	"encoding/json"
	"strings"
)

const (
	agentDeactivatedStatePrefix = "agent_deactivated:"
	agentSnapshotStatePrefix    = "agent_snapshot:"
)

func agentDeactivatedStateKey(agentID string) string {
	return agentDeactivatedStatePrefix + strings.TrimSpace(agentID)
}

func agentSnapshotStateKey(agentID string) string {
	return agentSnapshotStatePrefix + strings.TrimSpace(agentID)
}

func parseBooleanStateValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *stateStore) persistAgentSnapshot(agentID string, state agentState) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || s.db == nil {
		return nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.updateStateStore().SetAppState(agentSnapshotStateKey(agentID), string(raw))
}

func (s *stateStore) hydrateAgentStateFromAppState(appState map[string]string) {
	if s.agents == nil {
		s.agents = make(map[string]agentState)
	}
	if s.agentDeactivated == nil {
		s.agentDeactivated = make(map[string]bool)
	}
	for key, value := range appState {
		if !strings.HasPrefix(key, agentSnapshotStatePrefix) {
			continue
		}
		agentID := strings.TrimSpace(strings.TrimPrefix(key, agentSnapshotStatePrefix))
		if agentID == "" {
			continue
		}
		var snap agentState
		if err := json.Unmarshal([]byte(value), &snap); err != nil {
			continue
		}
		s.agents[agentID] = snap
		if snap.Deactivated {
			s.agentDeactivated[agentID] = true
		}
	}
	for key, value := range appState {
		if !strings.HasPrefix(key, agentDeactivatedStatePrefix) {
			continue
		}
		agentID := strings.TrimSpace(strings.TrimPrefix(key, agentDeactivatedStatePrefix))
		if agentID == "" {
			continue
		}
		deactivated := parseBooleanStateValue(value)
		s.agentDeactivated[agentID] = deactivated
		if snap, ok := s.agents[agentID]; ok {
			snap.Deactivated = deactivated
			s.agents[agentID] = snap
		}
	}
}
