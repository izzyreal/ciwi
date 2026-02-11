package server

import "strings"

func (s *stateStore) getAgentUpdateTarget() string {
	s.update.mu.Lock()
	defer s.update.mu.Unlock()
	return strings.TrimSpace(s.update.agentTarget)
}

func (s *stateStore) setAgentUpdateTarget(target string) error {
	target = strings.TrimSpace(target)
	s.update.mu.Lock()
	s.update.agentTarget = target
	s.update.mu.Unlock()
	return s.db.SetAppState("agent_update_target", target)
}
