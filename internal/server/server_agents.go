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

	s.mu.Lock()
	updateRequested := shouldRequestAgentUpdate(hb.Version, currentVersion())
	prev := s.agents[hb.AgentID]
	state := agentState{
		Hostname:     hb.Hostname,
		OS:           hb.OS,
		Arch:         hb.Arch,
		Version:      hb.Version,
		Capabilities: hb.Capabilities,
		LastSeenUTC:  hb.TimestampUTC,
		RecentLog:    append([]string(nil), prev.RecentLog...),
	}
	state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("heartbeat version=%s platform=%s/%s", strings.TrimSpace(hb.Version), strings.TrimSpace(hb.OS), strings.TrimSpace(hb.Arch)))
	if updateRequested {
		state.RecentLog = appendAgentLog(state.RecentLog, "server requested update to "+currentVersion())
	}
	s.agents[hb.AgentID] = state
	s.mu.Unlock()

	resp := protocol.HeartbeatResponse{
		Accepted: true,
	}
	if updateRequested {
		resp.UpdateRequested = true
		resp.UpdateTarget = currentVersion()
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
	for id, a := range s.agents {
		agents = append(agents, protocol.AgentInfo{
			AgentID:      id,
			Hostname:     a.Hostname,
			OS:           a.OS,
			Arch:         a.Arch,
			Version:      a.Version,
			Capabilities: a.Capabilities,
			LastSeenUTC:  a.LastSeenUTC,
			RecentLog:    append([]string(nil), a.RecentLog...),
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
			Status:  "failed",
			Error:   failMsg,
		})
		writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: false, Message: failMsg})
		return
	}
	writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: true, Job: job})
}
