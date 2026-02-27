package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
		s.mu.Unlock()
		jobInProgress, err := s.agentJobExecutionStore().AgentHasActiveJobExecution(agentID)
		if err != nil {
			jobInProgress = false
		}
		info := agentViewFromState(agentID, a, pendingTarget, serverVersion, jobInProgress)
		writeJSON(w, http.StatusOK, agentViewResponse{Agent: info})
		return
	}
	if len(parts) != 2 || parts[1] != "actions" {
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
	var req struct {
		Action         string `json:"action"`
		Script         string `json:"script"`
		Shell          string `json:"shell"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		http.Error(w, "action is required", http.StatusBadRequest)
		return
	}
	if action == "activate" {
		s.mu.Lock()
		a, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		a.Deactivated = false
		s.agentDeactivated[agentID] = false
		a.RecentLog = appendAgentLog(a.RecentLog, "manual activation requested")
		s.agents[agentID] = a
		s.mu.Unlock()
		if err := s.updateStateStore().SetAppState(agentDeactivatedStateKey(agentID), "0"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, agentActionResponse{
			Requested: true,
			AgentID:   agentID,
			Message:   "agent activated",
		})
		return
	}
	if action == "deactivate" {
		s.mu.Lock()
		a, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		a.Deactivated = true
		s.agentDeactivated[agentID] = true
		a.RecentLog = appendAgentLog(a.RecentLog, "manual deactivation requested")
		s.agents[agentID] = a
		s.mu.Unlock()
		if err := s.updateStateStore().SetAppState(agentDeactivatedStateKey(agentID), "1"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cancelled, err := s.cancelActiveJobsForAgent(agentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if cancelled > 0 {
			s.mu.Lock()
			if a, ok := s.agents[agentID]; ok {
				a.RecentLog = appendAgentLog(a.RecentLog, "deactivation cancelled active job count="+strconv.Itoa(cancelled))
				s.agents[agentID] = a
			}
			s.mu.Unlock()
		}
		msg := "agent deactivated"
		if cancelled > 0 {
			msg += "; cancelled active jobs=" + strconv.Itoa(cancelled)
		}
		writeJSON(w, http.StatusOK, agentActionResponse{
			Requested: true,
			AgentID:   agentID,
			Message:   msg,
		})
		return
	}
	if action == "refresh-tools" {
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
	if action == "restart" {
		s.mu.Lock()
		a, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		s.agentRestarts[agentID] = true
		a.RecentLog = appendAgentLog(a.RecentLog, "manual restart requested")
		s.agents[agentID] = a
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, agentActionResponse{
			Requested: true,
			AgentID:   agentID,
			Message:   "agent restart requested",
		})
		return
	}
	if action == "wipe-cache" {
		s.mu.Lock()
		a, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		s.agentCacheWipes[agentID] = true
		a.RecentLog = appendAgentLog(a.RecentLog, "manual cache wipe requested")
		s.agents[agentID] = a
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, agentActionResponse{
			Requested: true,
			AgentID:   agentID,
			Message:   "agent cache wipe requested",
		})
		return
	}
	if action == "flush-job-history" {
		s.mu.Lock()
		_, ok := s.agents[agentID]
		if !ok {
			s.mu.Unlock()
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		s.mu.Unlock()
		deletedIDs, err := s.db.FlushJobExecutionHistoryByAgent(agentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		removed := 0
		for _, jobID := range deletedIDs {
			if err := os.RemoveAll(filepath.Join(s.artifactsDir, jobID)); err == nil {
				removed++
			}
		}
		s.mu.Lock()
		if a, ok := s.agents[agentID]; ok {
			a.RecentLog = appendAgentLog(a.RecentLog, "manual agent job history flush requested")
			s.agents[agentID] = a
		}
		s.agentHistoryWipes[agentID] = true
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, agentActionResponse{
			Requested: true,
			AgentID:   agentID,
			Message:   "agent job history flushed: sqlite=" + strconv.Itoa(len(deletedIDs)) + ", disk=" + strconv.Itoa(removed) + ", local=queued",
		})
		return
	}
	if action == "run-script" {
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
	if action != "update" {
		http.Error(w, "unsupported action", http.StatusBadRequest)
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
	a.UpdateSource = updateSourceManual
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
