package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/internal/application"
)

func (s *stateStore) agentByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agents/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 1 {
		s.agentDetailsHandler(w, r, strings.TrimSpace(parts[0]))
		return
	}
	if len(parts) != 2 || parts[1] != "actions" {
		http.NotFound(w, r)
		return
	}
	s.agentActionHandler(w, r, strings.TrimSpace(parts[0]))
}

func (s *stateStore) agentDetailsHandler(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if agentID == "" {
		http.Error(w, "agent id is required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	agent, ok := s.agents[agentID]
	pendingTarget := strings.TrimSpace(s.agentUpdates[agentID])
	s.mu.Unlock()
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	jobInProgress, err := s.agentJobExecutionStore().AgentHasActiveJobExecution(agentID)
	if err != nil {
		jobInProgress = false
	}
	writeJSON(w, http.StatusOK, agentViewResponse{Agent: agentViewFromState(agentID, agent, pendingTarget, currentVersion(), jobInProgress)})
}

func (s *stateStore) agentActionHandler(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
	if action == "run-script" {
		result, err := s.app().agentScripts.Run(r.Context(), application.RunAgentScriptRequest{
			AgentID: agentID, Script: req.Script, Shell: req.Shell, TimeoutSeconds: req.TimeoutSeconds,
			IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		})
		if err != nil {
			http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
			return
		}
		writeJSON(w, http.StatusCreated, agentRunScriptResponse{
			Queued: result.Queued, AgentID: result.AgentID, JobExecutionID: result.JobExecutionID,
			Shell: result.Shell, TimeoutSeconds: result.TimeoutSeconds,
		})
		return
	}
	result, err := s.app().agentCommands.Execute(r.Context(), application.AgentActionRequest{
		AgentID: agentID, Action: action, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, agentActionResponse{
		Requested: result.Requested, AgentID: result.AgentID, Message: result.Message, Target: result.Target,
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
		shell := strings.ToLower(strings.TrimSpace(part))
		if shell == "" {
			continue
		}
		if _, seen := dedup[shell]; seen {
			continue
		}
		dedup[shell] = struct{}{}
		out = append(out, shell)
	}
	sort.Strings(out)
	return out
}

func containsString(list []string, needle string) bool {
	for _, value := range list {
		if value == needle {
			return true
		}
	}
	return false
}
