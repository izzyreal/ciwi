package server

import (
	"net/http"
	"strings"
)

func (s *stateStore) agentsViewHandler(w http.ResponseWriter, r *http.Request) {
	view, err := s.app().agents.GetAgentsView(r.Context())
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *stateStore) agentDetailsViewHandler(w http.ResponseWriter, r *http.Request) {
	agentID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/views/agents/"), "/")
	if agentID == "" || strings.Contains(agentID, "/") {
		http.Error(w, "agent id is required", http.StatusBadRequest)
		return
	}
	view, err := s.app().agents.GetAgentDetailsView(r.Context(), agentID)
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}
