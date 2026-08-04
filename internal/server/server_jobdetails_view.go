package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
)

type jobDetailsViewResponse struct {
	ID                  string                          `json:"id"`
	Title               string                          `json:"title"`
	Context             string                          `json:"context"`
	Status              string                          `json:"status"`
	StatusLabel         string                          `json:"status_label"`
	CurrentStep         string                          `json:"current_step"`
	Agent               string                          `json:"agent"`
	Mode                string                          `json:"mode"`
	Created             string                          `json:"created"`
	Started             string                          `json:"started"`
	Finished            string                          `json:"finished"`
	Duration            string                          `json:"duration"`
	ExitCode            string                          `json:"exit_code"`
	Error               string                          `json:"error"`
	CanCancel           bool                            `json:"can_cancel"`
	CanRerun            bool                            `json:"can_rerun"`
	Output              string                          `json:"output"`
	Timeline            []jobTimelineViewResponse       `json:"timeline"`
	OutputGroups        []jobOutputGroupViewResponse    `json:"output_groups"`
	SchedulingDiagnosis *jobSchedulingDiagnosisResponse `json:"scheduling_diagnosis,omitempty"`
	Progress            domain.Progress                 `json:"progress"`
}

type jobSchedulingDiagnosisResponse struct {
	State                 string                       `json:"state"`
	Summary               string                       `json:"summary"`
	RequirementsLabel     string                       `json:"requirements_label"`
	Agents                []jobSchedulingAgentResponse `json:"agents"`
	AdditionalAgentsLabel string                       `json:"additional_agents_label"`
}

type jobSchedulingAgentResponse struct {
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
	Details string `json:"details"`
	Tone    string `json:"tone"`
}

type jobTimelineViewResponse struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	StatusLabel string          `json:"status_label"`
	Duration    string          `json:"duration"`
	ExitCode    string          `json:"exit_code"`
	Error       string          `json:"error"`
	Progress    domain.Progress `json:"progress"`
}

type jobOutputGroupViewResponse struct {
	ID              string          `json:"id"`
	StateKey        string          `json:"state_key"`
	Kind            string          `json:"kind"`
	Title           string          `json:"title"`
	CommandSummary  string          `json:"command_summary"`
	Status          string          `json:"status"`
	StatusLabel     string          `json:"status_label"`
	Reached         bool            `json:"reached"`
	Started         string          `json:"started"`
	Duration        string          `json:"duration"`
	ExitCode        string          `json:"exit_code"`
	Error           string          `json:"error"`
	Details         string          `json:"details"`
	YAMLLiteral     string          `json:"yaml_literal"`
	ExpandedCommand string          `json:"expanded_command"`
	Progress        domain.Progress `json:"progress"`
}

type jobOutputViewResponse struct {
	JobExecutionID string                       `json:"job_execution_id"`
	Events         []jobOutputEventViewResponse `json:"events"`
	NextEventID    int64                        `json:"next_event_id"`
	HasMore        bool                         `json:"has_more"`
	Terminal       bool                         `json:"terminal"`
}

type jobOutputEventViewResponse struct {
	EventID  int64  `json:"event_id"`
	Type     string `json:"type"`
	ItemID   string `json:"item_id"`
	Text     string `json:"text"`
	Error    string `json:"error"`
	ExitCode string `json:"exit_code"`
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
	events := make([]jobOutputEventViewResponse, 0, len(view.Events))
	for _, event := range view.Events {
		events = append(events, jobOutputEventViewResponse{
			EventID: event.EventID, Type: event.Type, ItemID: event.ItemID, Text: event.Text,
			Error: event.Error, ExitCode: event.ExitCode,
		})
	}
	writeJSON(w, http.StatusOK, jobOutputViewResponse{
		JobExecutionID: view.JobExecutionID, Events: events, NextEventID: view.NextEventID,
		HasMore: view.HasMore, Terminal: view.Terminal,
	})
}

func jobDetailsToResponse(view presentation.JobDetailsView) jobDetailsViewResponse {
	timeline := make([]jobTimelineViewResponse, 0, len(view.Timeline))
	for _, item := range view.Timeline {
		timeline = append(timeline, jobTimelineViewResponse{
			ID: item.ID, Kind: item.Kind, Title: item.Title, Description: item.Description,
			Status: item.Status, StatusLabel: item.StatusLabel, Duration: item.Duration,
			ExitCode: item.ExitCode, Error: item.Error, Progress: item.Progress,
		})
	}
	outputGroups := make([]jobOutputGroupViewResponse, 0, len(view.OutputGroups))
	for _, group := range view.OutputGroups {
		outputGroups = append(outputGroups, jobOutputGroupViewResponse{
			ID: group.ID, StateKey: group.StateKey, Kind: group.Kind, Title: group.Title,
			CommandSummary: group.CommandSummary, Status: group.Status, StatusLabel: group.StatusLabel,
			Reached: group.Reached, Started: group.Started, Duration: group.Duration, ExitCode: group.ExitCode,
			Error: group.Error, Details: group.Details, YAMLLiteral: group.YAMLLiteral, ExpandedCommand: group.ExpandedCommand,
			Progress: group.Progress,
		})
	}
	response := jobDetailsViewResponse{
		ID: view.ID, Title: view.Title, Context: view.Context, Status: view.Status, StatusLabel: view.StatusLabel,
		CurrentStep: view.CurrentStep, Agent: view.Agent, Mode: view.Mode, Created: view.Created,
		Started: view.Started, Finished: view.Finished, Duration: view.Duration, ExitCode: view.ExitCode,
		Error: view.Error, CanCancel: view.CanCancel, CanRerun: view.CanRerun, Timeline: timeline, OutputGroups: outputGroups,
		Progress: view.Progress,
	}
	if view.SchedulingSummary != "" {
		agents := make([]jobSchedulingAgentResponse, 0, len(view.SchedulingAgents))
		for _, agent := range view.SchedulingAgents {
			agents = append(agents, jobSchedulingAgentResponse{
				AgentID: agent.AgentID, Status: agent.Status, Details: agent.Details, Tone: agent.Tone,
			})
		}
		response.SchedulingDiagnosis = &jobSchedulingDiagnosisResponse{
			State: view.SchedulingState, Summary: view.SchedulingSummary,
			RequirementsLabel: view.SchedulingRequirements, Agents: agents,
			AdditionalAgentsLabel: view.SchedulingAdditional,
		}
	}
	return response
}
