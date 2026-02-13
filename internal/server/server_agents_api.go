package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) agentByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agents/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		agentID := strings.TrimSpace(parts[0])
		if agentID == "" {
			http.Error(w, "agent id is required", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		a, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		serverVersion := currentVersion()
		pendingTarget := strings.TrimSpace(s.agentUpdates[agentID])
		info := agentViewFromState(agentID, a, pendingTarget, serverVersion)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, agentViewResponse{Agent: info})
		return
	}
	if len(parts) != 2 || (parts[1] != "update" && parts[1] != "refresh-tools" && parts[1] != "run-script") {
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
		writeJSON(w, http.StatusOK, agentActionResponse{
			Requested: true,
			AgentID:   agentID,
			Message:   "tools refresh requested",
		})
		return
	}
	if parts[1] == "run-script" {
		var req struct {
			Script         string `json:"script"`
			Shell          string `json:"shell"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		script := strings.TrimSpace(req.Script)
		shell := strings.ToLower(strings.TrimSpace(req.Shell))
		if script == "" {
			http.Error(w, "script is required", http.StatusBadRequest)
			return
		}
		if shell == "" {
			http.Error(w, "shell is required", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		a, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		if strings.TrimSpace(a.Capabilities["executor"]) != "script" {
			s.mu.Unlock()
			http.Error(w, "agent does not advertise script executor support", http.StatusBadRequest)
			return
		}
		availableShells := capabilityShells(a.Capabilities)
		if !containsString(availableShells, shell) {
			s.mu.Unlock()
			http.Error(w, "agent does not support requested shell", http.StatusBadRequest)
			return
		}
		s.mu.Unlock()

		timeout := req.TimeoutSeconds
		if timeout <= 0 {
			timeout = 600
		}
		job, err := s.agentJobExecutionStore().CreateJobExecution(protocol.CreateJobExecutionRequest{
			Script: script,
			RequiredCapabilities: map[string]string{
				"agent_id": agentID,
				"executor": "script",
				"shell":    shell,
			},
			TimeoutSeconds: timeout,
			Metadata: map[string]string{
				"adhoc":          "1",
				"adhoc_agent_id": agentID,
				"adhoc_shell":    shell,
			},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		if a, ok := s.agents[agentID]; ok {
			a.RecentLog = appendAgentLog(a.RecentLog, "ad-hoc script queued ("+shell+") job="+job.ID)
			s.agents[agentID] = a
		}
		s.mu.Unlock()

		writeJSON(w, http.StatusCreated, agentRunScriptResponse{
			Queued:         true,
			AgentID:        agentID,
			JobExecutionID: job.ID,
			Shell:          shell,
			TimeoutSeconds: timeout,
		})
		return
	}

	target := resolveManualAgentUpdateTarget(currentVersion(), s.getAgentUpdateTarget())
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
	if !isVersionDifferent(target, strings.TrimSpace(a.Version)) {
		a.RecentLog = appendAgentLog(a.RecentLog, "manual update requested but agent is already at target version")
		s.agents[agentID] = a
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, agentActionResponse{
			Requested: false,
			Message:   "agent is already at target version",
		})
		return
	}
	s.agentUpdates[agentID] = target
	a.UpdateTarget = target
	a.UpdateAttempts = 0
	a.UpdateInProgress = false
	a.UpdateLastRequestUTC = time.Time{}
	a.UpdateNextRetryUTC = time.Time{}
	a.UpdateLastError = ""
	a.UpdateLastErrorUTC = time.Time{}
	a.RecentLog = appendAgentLog(a.RecentLog, "manual update requested to "+target)
	s.agents[agentID] = a
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, agentActionResponse{
		Requested: true,
		AgentID:   agentID,
		Target:    target,
	})
}

func capabilityShells(caps map[string]string) []string {
	raw := strings.TrimSpace(caps["shells"])
	if raw == "" {
		return nil
	}
	dedup := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		s := strings.ToLower(strings.TrimSpace(part))
		if s == "" {
			continue
		}
		if _, seen := dedup[s]; seen {
			continue
		}
		dedup[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func containsString(list []string, needle string) bool {
	for _, v := range list {
		if v == needle {
			return true
		}
	}
	return false
}
