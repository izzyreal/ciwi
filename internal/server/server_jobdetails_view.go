package server

import (
	"net/http"
	"strings"

	"github.com/izzyreal/ciwi/internal/presentation"
)

type jobDetailsViewResponse struct {
	ID          string                    `json:"id"`
	Title       string                    `json:"title"`
	Context     string                    `json:"context"`
	Status      string                    `json:"status"`
	StatusLabel string                    `json:"status_label"`
	CurrentStep string                    `json:"current_step"`
	Agent       string                    `json:"agent"`
	Mode        string                    `json:"mode"`
	Created     string                    `json:"created"`
	Started     string                    `json:"started"`
	Finished    string                    `json:"finished"`
	Duration    string                    `json:"duration"`
	ExitCode    string                    `json:"exit_code"`
	Error       string                    `json:"error"`
	Timeline    []jobTimelineViewResponse `json:"timeline"`
}

type jobTimelineViewResponse struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	StatusLabel string `json:"status_label"`
	Duration    string `json:"duration"`
	ExitCode    string `json:"exit_code"`
	Error       string `json:"error"`
}

func (s *stateStore) jobDetailsViewHandler(w http.ResponseWriter, r *http.Request) {
	jobID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/views/jobs/"), "/")
	if jobID == "" || strings.Contains(jobID, "/") {
		http.Error(w, "invalid job execution id", http.StatusBadRequest)
		return
	}
	view, err := s.app().jobDetails.GetJobDetailsView(r.Context(), jobID)
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, jobDetailsToResponse(view))
}

func jobDetailsToResponse(view presentation.JobDetailsView) jobDetailsViewResponse {
	timeline := make([]jobTimelineViewResponse, 0, len(view.Timeline))
	for _, item := range view.Timeline {
		timeline = append(timeline, jobTimelineViewResponse{
			ID: item.ID, Kind: item.Kind, Title: item.Title, Description: item.Description,
			Status: item.Status, StatusLabel: item.StatusLabel, Duration: item.Duration,
			ExitCode: item.ExitCode, Error: item.Error,
		})
	}
	return jobDetailsViewResponse{
		ID: view.ID, Title: view.Title, Context: view.Context, Status: view.Status, StatusLabel: view.StatusLabel,
		CurrentStep: view.CurrentStep, Agent: view.Agent, Mode: view.Mode, Created: view.Created,
		Started: view.Started, Finished: view.Finished, Duration: view.Duration, ExitCode: view.ExitCode,
		Error: view.Error, Timeline: timeline,
	}
}
