package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/application"
)

func (s *stateStore) runOptionsViewHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/views/run-options/"), "/")
	parts := strings.Split(path, "/")
	request := application.RunOptionsRequest{
		SourceRef: r.URL.Query().Get("source_ref"), AgentID: r.URL.Query().Get("agent_id"),
		IncludeSourceRefs: true, IncludeEligibleAgents: true,
		AllowMissingSourceRepo: true,
	}
	var err error
	switch {
	case len(parts) == 2 && parts[0] == "pipelines":
		request.PipelineDBID, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || request.PipelineDBID <= 0 {
			http.Error(w, "invalid pipeline id", http.StatusBadRequest)
			return
		}
	case len(parts) == 4 && parts[0] == "projects" && parts[2] == "chains":
		request.ProjectID, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || request.ProjectID <= 0 {
			http.Error(w, "invalid project id", http.StatusBadRequest)
			return
		}
		request.ChainID, err = url.PathUnescape(parts[3])
		if err != nil || strings.TrimSpace(request.ChainID) == "" {
			http.Error(w, "invalid pipeline chain id", http.StatusBadRequest)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	options, err := s.app().runOptions.GetRunOptions(r.Context(), request)
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, options)
}
