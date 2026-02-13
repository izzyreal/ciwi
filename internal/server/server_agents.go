package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var hb protocol.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if hb.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	if hb.TimestampUTC.IsZero() {
		hb.TimestampUTC = time.Now().UTC()
	}

	now := time.Now().UTC()
	s.mu.Lock()
	prev := s.agents[hb.AgentID]
	refreshTools := s.agentToolRefresh[hb.AgentID]
	if refreshTools {
		delete(s.agentToolRefresh, hb.AgentID)
	}
	target := strings.TrimSpace(s.getAgentUpdateTarget())
	manualTarget := strings.TrimSpace(s.agentUpdates[hb.AgentID])
	if manualTarget != "" {
		target = manualTarget
	}

	updateRequested := false
	needsUpdate := target != "" && isVersionDifferent(target, strings.TrimSpace(hb.Version))
	if needsUpdate {
		if strings.TrimSpace(prev.UpdateTarget) != target {
			prev.UpdateTarget = target
			prev.UpdateAttempts = 0
			prev.UpdateLastRequestUTC = time.Time{}
			prev.UpdateNextRetryUTC = time.Time{}
		}
		if prev.UpdateNextRetryUTC.IsZero() || !now.Before(prev.UpdateNextRetryUTC) {
			updateRequested = true
			prev.UpdateAttempts++
			prev.UpdateLastRequestUTC = now
			prev.UpdateNextRetryUTC = now.Add(agentUpdateBackoff(prev.UpdateAttempts))
		}
	} else {
		if manualTarget != "" {
			delete(s.agentUpdates, hb.AgentID)
		}
		prev.UpdateTarget = ""
		prev.UpdateAttempts = 0
		prev.UpdateLastRequestUTC = time.Time{}
		prev.UpdateNextRetryUTC = time.Time{}
	}

	state := agentState{
		Hostname:             hb.Hostname,
		OS:                   hb.OS,
		Arch:                 hb.Arch,
		Version:              hb.Version,
		Capabilities:         hb.Capabilities,
		LastSeenUTC:          hb.TimestampUTC,
		RecentLog:            append([]string(nil), prev.RecentLog...),
		UpdateTarget:         prev.UpdateTarget,
		UpdateAttempts:       prev.UpdateAttempts,
		UpdateLastRequestUTC: prev.UpdateLastRequestUTC,
		UpdateNextRetryUTC:   prev.UpdateNextRetryUTC,
	}
	state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("heartbeat version=%s platform=%s/%s", strings.TrimSpace(hb.Version), strings.TrimSpace(hb.OS), strings.TrimSpace(hb.Arch)))
	if refreshTools {
		state.RecentLog = appendAgentLog(state.RecentLog, "server requested tools refresh")
	}
	if updateRequested {
		state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("server requested update to %s (attempt=%d, next_retry=%s)", target, state.UpdateAttempts, state.UpdateNextRetryUTC.Local().Format("15:04:05")))
	}
	s.agents[hb.AgentID] = state
	s.mu.Unlock()

	resp := protocol.HeartbeatResponse{
		Accepted: true,
	}
	if refreshTools {
		resp.RefreshToolsRequested = true
	}
	if updateRequested {
		resp.UpdateRequested = true
		resp.UpdateTarget = target
		resp.UpdateRepository = strings.TrimSpace(envOrDefault("CIWI_UPDATE_REPO", "izzyreal/ciwi"))
		resp.UpdateAPIBase = strings.TrimRight(strings.TrimSpace(envOrDefault("CIWI_UPDATE_API_BASE", "https://api.github.com")), "/")
		resp.Message = "server requested agent update"
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *stateStore) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	agents := make([]protocol.AgentInfo, 0, len(s.agents))
	serverVersion := currentVersion()
	for id, a := range s.agents {
		pendingTarget := strings.TrimSpace(s.agentUpdates[id])
		needsUpdate := serverVersion != "" && isVersionNewer(serverVersion, strings.TrimSpace(a.Version))
		updateTarget := serverVersion
		if pendingTarget != "" {
			updateTarget = pendingTarget
		} else if strings.TrimSpace(a.UpdateTarget) != "" {
			updateTarget = strings.TrimSpace(a.UpdateTarget)
		}
		updateRequested := pendingTarget != "" || (a.UpdateTarget != "" && isVersionDifferent(a.UpdateTarget, strings.TrimSpace(a.Version)))
		agents = append(agents, protocol.AgentInfo{
			AgentID:              id,
			Hostname:             a.Hostname,
			OS:                   a.OS,
			Arch:                 a.Arch,
			Version:              a.Version,
			Capabilities:         a.Capabilities,
			LastSeenUTC:          a.LastSeenUTC,
			RecentLog:            append([]string(nil), a.RecentLog...),
			NeedsUpdate:          needsUpdate,
			UpdateTarget:         updateTarget,
			UpdateRequested:      updateRequested,
			UpdateAttempts:       a.UpdateAttempts,
			UpdateLastRequestUTC: a.UpdateLastRequestUTC,
			UpdateNextRetryUTC:   a.UpdateNextRetryUTC,
		})
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *stateStore) leaseJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.LeaseJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	agentCaps := req.Capabilities
	s.mu.Lock()
	if a, ok := s.agents[req.AgentID]; ok {
		agentCaps = mergeCapabilities(a, req.Capabilities)
	}
	s.mu.Unlock()
	if agentCaps == nil {
		agentCaps = map[string]string{}
	}
	agentCaps["agent_id"] = req.AgentID
	hasActive, err := s.db.AgentHasActiveJob(req.AgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hasActive {
		writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{
			Assigned: false,
			Message:  "agent already has an active job",
		})
		return
	}

	job, err := s.db.LeaseJob(req.AgentID, agentCaps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: false, Message: "no matching queued job"})
		return
	}
	if err := s.resolveJobSecrets(r.Context(), job); err != nil {
		failMsg := fmt.Sprintf("secret resolution failed before execution: %v", err)
		_, _ = s.db.UpdateJobStatus(job.ID, protocol.JobStatusUpdateRequest{
			AgentID: req.AgentID,
			Status:  protocol.JobStatusFailed,
			Error:   failMsg,
		})
		writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: false, Message: failMsg})
		return
	}
	writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: true, Job: job})
}
