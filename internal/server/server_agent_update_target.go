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
	return s.updateStateStore().SetAppState("agent_update_target", target)
}

func resolveEffectiveAgentUpdateTarget(configuredTarget, serverVersion string) string {
	target := strings.TrimSpace(configuredTarget)
	if target == "" {
		// Empty target means no server-driven agent update request.
		return ""
	}
	serverVersion = strings.TrimSpace(serverVersion)
	if serverVersion == "" || serverVersion == "dev" {
		return target
	}
	if isVersionNewer(serverVersion, target) {
		return serverVersion
	}
	return target
}

func resolveManualAgentUpdateTarget(serverVersion, configuredTarget string) string {
	serverVersion = strings.TrimSpace(serverVersion)
	if serverVersion == "" || serverVersion == "dev" {
		return serverVersion
	}
	target := serverVersion
	effective := resolveEffectiveAgentUpdateTarget(configuredTarget, serverVersion)
	if effective != "" && isVersionNewer(effective, target) {
		target = effective
	}
	return target
}
