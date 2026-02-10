package server

import (
	"encoding/json"
	"net/http"
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
	s.agents[hb.AgentID] = agentState{
		Hostname:     hb.Hostname,
		OS:           hb.OS,
		Arch:         hb.Arch,
		Capabilities: hb.Capabilities,
		LastSeenUTC:  hb.TimestampUTC,
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, protocol.HeartbeatResponse{Accepted: true})
}

func (s *stateStore) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	agents := make([]protocol.AgentInfo, 0, len(s.agents))
	for id, a := range s.agents {
		agents = append(agents, protocol.AgentInfo{AgentID: id, Hostname: a.Hostname, OS: a.OS, Arch: a.Arch, Capabilities: a.Capabilities, LastSeenUTC: a.LastSeenUTC})
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
	writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: true, Job: job})
}
