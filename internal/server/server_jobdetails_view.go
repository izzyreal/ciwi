package server

import (
	"net/http"
	"strconv"
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
	CanCancel   bool                      `json:"can_cancel"`
	CanRerun    bool                      `json:"can_rerun"`
	Output      string                    `json:"output"`
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

type jobOutputViewResponse struct {
	JobExecutionID string                      `json:"job_execution_id"`
	Lines          []jobOutputLineViewResponse `json:"lines"`
	NextEventID    int64                       `json:"next_event_id"`
	HasMore        bool                        `json:"has_more"`
	Terminal       bool                        `json:"terminal"`
}

type jobOutputLineViewResponse struct {
	EventID int64  `json:"event_id"`
	Text    string `json:"text"`
}

func (s *stateStore) jobDetailsViewHandler(w http.ResponseWriter, r *http.Request) {
	relative := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/views/jobs/"), "/")
	parts := strings.Split(relative, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" || len(parts) > 2 {
		http.Error(w, "invalid job execution id", http.StatusBadRequest)
		return
	}
	jobID := strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		if parts[1] != "output" {
			http.NotFound(w, r)
			return
		}
		s.jobOutputViewHandler(w, r, jobID)
		return
	}
	view, err := s.app().jobDetails.GetJobDetailsView(r.Context(), jobID)
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, jobDetailsToResponse(view))
}

func (s *stateStore) jobOutputViewHandler(w http.ResponseWriter, r *http.Request, jobID string) {
	afterEventID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after_event_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			http.Error(w, "after_event_id must be a non-negative integer", http.StatusBadRequest)
			return
		}
		afterEventID = parsed
	}
	view, err := s.app().jobDetails.GetJobOutputView(r.Context(), jobID, afterEventID)
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	lines := make([]jobOutputLineViewResponse, 0, len(view.Lines))
	for _, line := range view.Lines {
		lines = append(lines, jobOutputLineViewResponse{EventID: line.EventID, Text: line.Text})
	}
	writeJSON(w, http.StatusOK, jobOutputViewResponse{
		JobExecutionID: view.JobExecutionID, Lines: lines, NextEventID: view.NextEventID,
		HasMore: view.HasMore, Terminal: view.Terminal,
	})
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
		Error: view.Error, CanCancel: view.CanCancel, CanRerun: view.CanRerun, Timeline: timeline,
	}
}
