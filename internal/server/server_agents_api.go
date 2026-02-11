package server

import (
	"net/http"
	"strings"
	"time"
)

func (s *stateStore) agentByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agents/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || (parts[1] != "update" && parts[1] != "refresh-tools") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agentID := strings.TrimSpace(parts[0])
	if agentID == "" {
		http.Error(w, "agent id is required", http.StatusBadRequest)
		return
	}
	if parts[1] == "refresh-tools" {
		s.mu.Lock()
		a, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		s.agentToolRefresh[agentID] = true
		a.RecentLog = appendAgentLog(a.RecentLog, "manual tools refresh requested")
		s.agents[agentID] = a
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"requested": true,
			"agent_id":  agentID,
			"message":   "tools refresh requested",
		})
		return
	}

	target := currentVersion()
	if target == "" || target == "dev" {
		http.Error(w, "server version is not a release version", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	a, ok := s.agents[agentID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if !isVersionNewer(target, strings.TrimSpace(a.Version)) {
		a.RecentLog = appendAgentLog(a.RecentLog, "manual update requested but agent is already up to date")
		s.agents[agentID] = a
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"requested": false, "message": "agent is already up to date"})
		return
	}
	s.agentUpdates[agentID] = target
	a.UpdateTarget = target
	a.UpdateAttempts = 0
	a.UpdateLastRequestUTC = time.Time{}
	a.UpdateNextRetryUTC = time.Time{}
	a.RecentLog = appendAgentLog(a.RecentLog, "manual update requested to "+target)
	s.agents[agentID] = a
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"requested": true,
		"agent_id":  agentID,
		"target":    target,
	})
}
