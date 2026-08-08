package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type jobDetailsViewResponse struct {
	ID                        string                         `json:"id"`
	ProjectID                 int64                          `json:"project_id"`
	Title                     string                         `json:"title"`
	Context                   string                         `json:"context"`
	Status                    string                         `json:"status"`
	StatusLabel               string                         `json:"status_label"`
	CurrentStep               string                         `json:"current_step"`
	Agent                     string                         `json:"agent"`
	Mode                      string                         `json:"mode"`
	Created                   string                         `json:"created"`
	Started                   string                         `json:"started"`
	Finished                  string                         `json:"finished"`
	Duration                  string                         `json:"duration"`
	ExitCode                  string                         `json:"exit_code"`
	Error                     string                         `json:"error"`
	CanCancel                 bool                           `json:"can_cancel"`
	CanRerun                  bool                           `json:"can_rerun"`
	Output                    string                         `json:"output"`
	Timeline                  []jobTimelineViewResponse      `json:"timeline"`
	OutputGroups              []jobOutputGroupViewResponse   `json:"output_groups"`
	SchedulingDiagnosis       jobSchedulingDiagnosisResponse `json:"scheduling_diagnosis"`
	JobProperties             []jobDetailRowResponse         `json:"job_properties"`
	CacheStatistics           []jobDetailRowResponse         `json:"cache_statistics"`
	CacheStatisticsEmpty      string                         `json:"cache_statistics_empty"`
	HostToolRequirements      jobToolRequirementsResponse    `json:"host_tool_requirements"`
	ContainerToolRequirements jobToolRequirementsResponse    `json:"container_tool_requirements"`
	ReleaseSummary            []jobDetailRowResponse         `json:"release_summary"`
	HasReleaseSummary         bool                           `json:"has_release_summary"`
	Artifacts                 jobReportDetailsResponse       `json:"artifacts"`
	TestReport                jobReportDetailsResponse       `json:"test_report"`
	CoverageReport            jobReportDetailsResponse       `json:"coverage_report"`
	RunContext                jobRunContextResponse          `json:"run_context"`
	Progress                  domain.Progress                `json:"progress"`
}

type jobDetailRowResponse struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone"`
}

type jobToolRequirementsResponse struct {
	EmptyLabel string   `json:"empty_label"`
	Summary    string   `json:"summary"`
	Tone       string   `json:"tone"`
	Issues     []string `json:"issues"`
}

type jobReportDetailsResponse struct {
	EmptyLabel      string                    `json:"empty_label"`
	Summary         string                    `json:"summary"`
	Tone            string                    `json:"tone"`
	Rows            []jobDetailRowResponse    `json:"rows"`
	AdditionalLabel string                    `json:"additional_label"`
	Nodes           []jobTreeNodeResponse     `json:"nodes"`
	Filters         []jobReportFilterResponse `json:"filters"`
	Filter          string                    `json:"filter"`
	CanDownloadAll  bool                      `json:"can_download_all"`
}

type jobReportFilterResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type jobTreeNodeResponse struct {
	Key             string                `json:"key"`
	Label           string                `json:"label"`
	Detail          string                `json:"detail"`
	Tone            string                `json:"tone"`
	Link            string                `json:"link"`
	ActionLabel     string                `json:"action_label"`
	ActionKind      string                `json:"action_kind"`
	ActionPath      string                `json:"action_path"`
	DefaultExpanded bool                  `json:"default_expanded"`
	FilterValues    []string              `json:"filter_values"`
	Children        []jobTreeNodeResponse `json:"children"`
}

type jobRunContextResponse struct {
	Available            bool                            `json:"available"`
	Scope                string                          `json:"scope"`
	ScopeLabel           string                          `json:"scope_label"`
	CurrentExecutionID   string                          `json:"current_execution_id"`
	CurrentPipelineID    string                          `json:"current_pipeline_id"`
	CurrentPipelineJobID string                          `json:"current_pipeline_job_id"`
	Pipelines            []jobRunContextPipelineResponse `json:"pipelines"`
}

type jobRunContextPipelineResponse struct {
	ID           int64                      `json:"id"`
	PipelineID   string                     `json:"pipeline_id"`
	DependsOn    []string                   `json:"depends_on"`
	Status       string                     `json:"status"`
	SummaryLabel string                     `json:"summary_label"`
	Jobs         []jobRunContextJobResponse `json:"jobs"`
}

type jobRunContextJobResponse struct {
	ID           string                           `json:"id"`
	Needs        []string                         `json:"needs"`
	Status       string                           `json:"status"`
	SummaryLabel string                           `json:"summary_label"`
	Executions   []jobRunContextExecutionResponse `json:"executions"`
}

type jobRunContextExecutionResponse struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	MatrixLabel  string `json:"matrix_label"`
	AttemptLabel string `json:"attempt_label"`
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
	runContext, err := s.GetJobExecutionGraphContext(r.Context(), jobID)
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, jobDetailsToResponse(view, runContext))
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

func jobDetailsToResponse(view presentation.JobDetailsView, runContext protocol.JobExecutionGraphContext) jobDetailsViewResponse {
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
		ID: view.ID, ProjectID: view.ProjectID, Title: view.Title, Context: view.Context, Status: view.Status, StatusLabel: view.StatusLabel,
		CurrentStep: view.CurrentStep, Agent: view.Agent, Mode: view.Mode, Created: view.Created,
		Started: view.Started, Finished: view.Finished, Duration: view.Duration, ExitCode: view.ExitCode,
		Error: view.Error, CanCancel: view.CanCancel, CanRerun: view.CanRerun, Timeline: timeline, OutputGroups: outputGroups,
		JobProperties:   jobDetailRowsToResponse(view.JobProperties),
		CacheStatistics: jobDetailRowsToResponse(view.CacheStatistics), CacheStatisticsEmpty: view.CacheStatisticsEmpty,
		HostToolRequirements:      jobToolRequirementsToResponse(view.HostToolRequirements),
		ContainerToolRequirements: jobToolRequirementsToResponse(view.ContainerToolRequirements),
		ReleaseSummary:            jobDetailRowsToResponse(view.ReleaseSummary), HasReleaseSummary: view.HasReleaseSummary,
		Artifacts: jobReportDetailsToResponse(view.Artifacts), TestReport: jobReportDetailsToResponse(view.TestReport),
		CoverageReport: jobReportDetailsToResponse(view.CoverageReport), RunContext: jobRunContextToResponse(runContext),
		Progress: view.Progress,
	}
	agents := make([]jobSchedulingAgentResponse, 0, len(view.SchedulingAgents))
	for _, agent := range view.SchedulingAgents {
		agents = append(agents, jobSchedulingAgentResponse{
			AgentID: agent.AgentID, Status: agent.Status, Details: agent.Details, Tone: agent.Tone,
		})
	}
	response.SchedulingDiagnosis = jobSchedulingDiagnosisResponse{
		State: view.SchedulingState, Summary: view.SchedulingSummary,
		RequirementsLabel: view.SchedulingRequirements, Agents: agents,
		AdditionalAgentsLabel: view.SchedulingAdditional,
	}
	return response
}

func jobDetailRowsToResponse(rows []presentation.JobDetailRowView) []jobDetailRowResponse {
	result := make([]jobDetailRowResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, jobDetailRowResponse{Label: row.Label, Value: row.Value, Tone: row.Tone})
	}
	return result
}

func jobToolRequirementsToResponse(view presentation.ToolRequirementsView) jobToolRequirementsResponse {
	return jobToolRequirementsResponse{
		EmptyLabel: view.EmptyLabel, Summary: view.Summary, Tone: view.Tone,
		Issues: append([]string{}, view.Issues...),
	}
}

func jobReportDetailsToResponse(view presentation.ReportDetailsView) jobReportDetailsResponse {
	filters := make([]jobReportFilterResponse, 0, len(view.Filters))
	for _, filter := range view.Filters {
		filters = append(filters, jobReportFilterResponse{Value: filter.Value, Label: filter.Label})
	}
	return jobReportDetailsResponse{
		EmptyLabel: view.EmptyLabel, Summary: view.Summary, Tone: view.Tone,
		Rows: jobDetailRowsToResponse(view.Rows), AdditionalLabel: view.AdditionalLabel,
		Nodes: jobTreeNodesToResponse(view.Nodes), Filters: filters, Filter: view.Filter, CanDownloadAll: view.CanDownloadAll,
	}
}

func jobTreeNodesToResponse(nodes []presentation.TreeNodeView) []jobTreeNodeResponse {
	result := make([]jobTreeNodeResponse, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, jobTreeNodeResponse{
			Key: node.Key, Label: node.Label, Detail: node.Detail, Tone: node.Tone, Link: node.Link,
			ActionLabel: node.ActionLabel, ActionKind: node.ActionKind, ActionPath: node.ActionPath,
			DefaultExpanded: node.DefaultExpanded, FilterValues: append([]string{}, node.FilterValues...), Children: jobTreeNodesToResponse(node.Children),
		})
	}
	return result
}

func jobRunContextToResponse(view protocol.JobExecutionGraphContext) jobRunContextResponse {
	pipelines := make([]jobRunContextPipelineResponse, 0, len(view.Pipelines))
	for _, pipeline := range view.Pipelines {
		jobs := make([]jobRunContextJobResponse, 0, len(pipeline.Jobs))
		for _, job := range pipeline.Jobs {
			executions := make([]jobRunContextExecutionResponse, 0, len(job.Executions))
			for _, execution := range job.Executions {
				matrixLabel := strings.TrimSpace(execution.MatrixName)
				if matrixLabel == "" && strings.TrimSpace(execution.MatrixIndex) != "" {
					matrixLabel = "index-" + strings.TrimSpace(execution.MatrixIndex)
				}
				if matrixLabel == "" {
					matrixLabel = "job"
				}
				attemptLabel := "Previous attempt"
				if execution.LatestAttempt {
					attemptLabel = "Latest attempt"
				}
				executions = append(executions, jobRunContextExecutionResponse{
					ID: execution.ID, Status: execution.Status, MatrixLabel: matrixLabel, AttemptLabel: attemptLabel,
				})
			}
			jobs = append(jobs, jobRunContextJobResponse{
				ID: job.PipelineJobID, Needs: append([]string{}, job.Needs...), Status: job.Status,
				SummaryLabel: fmt.Sprintf("%s · %d execution(s)", job.Status, len(job.Executions)), Executions: executions,
			})
		}
		pipelines = append(pipelines, jobRunContextPipelineResponse{
			ID: pipeline.PipelineDBID, PipelineID: pipeline.PipelineID, DependsOn: append([]string{}, pipeline.DependsOn...),
			Status: pipeline.Status, SummaryLabel: fmt.Sprintf("%s · %d job(s)", pipeline.Status, len(pipeline.Jobs)), Jobs: jobs,
		})
	}
	scopeLabel := strings.TrimSpace(view.Scope)
	if scopeLabel != "" {
		scopeLabel = strings.ToUpper(scopeLabel[:1]) + scopeLabel[1:] + " run"
	}
	return jobRunContextResponse{
		Available: len(pipelines) > 0, Scope: view.Scope, ScopeLabel: scopeLabel,
		CurrentExecutionID: view.CurrentExecutionID, CurrentPipelineID: view.CurrentPipelineID,
		CurrentPipelineJobID: view.CurrentPipelineJobID, Pipelines: pipelines,
	}
}
