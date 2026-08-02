package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/izzyreal/ciwi/internal/application"
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
		s.runAgentScriptHandler(w, agentID, req.Script, req.Shell, req.TimeoutSeconds)
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

func (s *stateStore) runAgentScriptHandler(w http.ResponseWriter, agentID, rawScript, rawShell string, timeout int) {
	script := strings.TrimSpace(rawScript)
	shell := strings.ToLower(strings.TrimSpace(rawShell))
	if script == "" {
		http.Error(w, "script is required", http.StatusBadRequest)
		return
	}
	if shell == "" {
		http.Error(w, "shell is required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	agent, ok := s.agents[agentID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if strings.TrimSpace(agent.Capabilities["executor"]) != "script" {
		s.mu.Unlock()
		http.Error(w, "agent does not advertise script executor support", http.StatusBadRequest)
		return
	}
	if !containsString(capabilityShells(agent.Capabilities), shell) {
		s.mu.Unlock()
		http.Error(w, "agent does not support requested shell", http.StatusBadRequest)
		return
	}
	s.mu.Unlock()
	if timeout <= 0 {
		timeout = 600
	}
	job, err := s.agentJobExecutionStore().CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               script,
		RequiredCapabilities: map[string]string{"agent_id": agentID, "executor": "script", "shell": shell},
		TimeoutSeconds:       timeout,
		Metadata:             map[string]string{"adhoc": "1", "adhoc_agent_id": agentID, "adhoc_shell": shell},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if agent, ok := s.agents[agentID]; ok {
		agent.RecentLog = appendAgentLog(agent.RecentLog, "ad-hoc script queued ("+shell+") job="+job.ID)
		s.agents[agentID] = agent
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, agentRunScriptResponse{
		Queued: true, AgentID: agentID, JobExecutionID: job.ID, Shell: shell, TimeoutSeconds: timeout,
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
