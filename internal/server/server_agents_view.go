package server

import (
	"net/http"
)

func (s *stateStore) agentsViewHandler(w http.ResponseWriter, r *http.Request) {
	view, err := s.app().agents.GetAgentsView(r.Context())
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, view)
}
