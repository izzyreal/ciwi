package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
)

type projectDetailsViewResponse struct {
	Project           frontPageProjectResponse         `json:"project"`
	Pipelines         []projectPipelineDetailsResponse `json:"pipelines"`
	HistoryExecutions []executionCardResponse          `json:"history_executions"`
	HistoryEmpty      bool                             `json:"history_empty"`
}

type projectPipelineDetailsResponse struct {
	ID                int64                       `json:"id"`
	PipelineID        string                      `json:"pipeline_id"`
	Trigger           string                      `json:"trigger"`
	DependsOn         []string                    `json:"depends_on"`
	Dependencies      string                      `json:"dependencies"`
	JobsCount         int                         `json:"jobs_count"`
	SupportsDryRun    bool                        `json:"supports_dry_run"`
	Jobs              []projectJobDetailsResponse `json:"jobs"`
	SummaryLabel      string                      `json:"summary_label"`
	GraphSummaryLabel string                      `json:"graph_summary_label"`
}

type projectJobDetailsResponse struct {
	ID             string                       `json:"id"`
	Needs          []string                     `json:"needs"`
	NeedsLabel     string                       `json:"needs_label"`
	RunsOnLabel    string                       `json:"runs_on_label"`
	ToolsLabel     string                       `json:"tools_label"`
	TimeoutSeconds int                          `json:"timeout_seconds"`
	MatrixCount    int                          `json:"matrix_count"`
	StepsCount     int                          `json:"steps_count"`
	SupportsDryRun bool                         `json:"supports_dry_run"`
	Steps          []projectStepDetailsResponse `json:"steps"`
	SummaryLabel   string                       `json:"summary_label"`
	TimeoutLabel   string                       `json:"timeout_label"`
	MatrixLabel    string                       `json:"matrix_label"`
}

type projectStepDetailsResponse struct {
	Index            int      `json:"index"`
	Position         int      `json:"position"`
	Name             string   `json:"name"`
	Type             string   `json:"type"`
	Command          string   `json:"command"`
	SkipDryRun       bool     `json:"skip_dry_run"`
	Environment      []string `json:"environment"`
	EnvironmentLabel string   `json:"environment_label"`
}

func (s *stateStore) projectDetailsViewHandler(w http.ResponseWriter, r *http.Request) {
	rawID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/views/projects/"), "/")
	projectID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || projectID <= 0 {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}
	view, err := s.app().projectDetails.GetProjectDetailsView(r.Context(), projectID)
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, projectDetailsToResponse(view))
}

func projectDetailsToResponse(view presentation.ProjectDetailsView) projectDetailsViewResponse {
	projects := frontPageProjectsToResponse([]domain.Project{view.Project})
	var project frontPageProjectResponse
	if len(projects) > 0 {
		project = projects[0]
	}
	pipelines := make([]projectPipelineDetailsResponse, 0, len(view.Pipelines))
	for _, pipeline := range view.Pipelines {
		jobs := make([]projectJobDetailsResponse, 0, len(pipeline.Jobs))
		for _, job := range pipeline.Jobs {
			steps := make([]projectStepDetailsResponse, 0, len(job.Steps))
			for _, step := range job.Steps {
				steps = append(steps, projectStepDetailsResponse{
					Index: step.Index, Position: step.Position, Name: step.Name, Type: step.Type,
					Command: presentation.ProjectStepCommand(step.Command), SkipDryRun: step.SkipDryRun,
					Environment:      append([]string{}, step.Environment...),
					EnvironmentLabel: presentation.ProjectStepEnvironmentLabel(step.Environment),
				})
			}
			runsOn := presentation.DeclarativeDefaultLabel(job.RunsOnLabel, "unspecified")
			jobs = append(jobs, projectJobDetailsResponse{
				ID: job.ID, Needs: append([]string{}, job.Needs...), NeedsLabel: presentation.DeclarativeDefaultLabel(job.NeedsLabel, "none"),
				RunsOnLabel: runsOn, ToolsLabel: presentation.DeclarativeDefaultLabel(job.ToolsLabel, "none"),
				TimeoutSeconds: job.TimeoutSeconds, MatrixCount: job.MatrixCount,
				StepsCount: job.StepsCount, SupportsDryRun: job.SupportsDryRun, Steps: steps,
				SummaryLabel: presentation.ProjectJobSummaryLabel(job.StepsCount, runsOn),
				TimeoutLabel: presentation.ProjectJobTimeoutLabel(job.TimeoutSeconds), MatrixLabel: presentation.ProjectJobMatrixLabel(job.MatrixCount),
			})
		}
		pipelines = append(pipelines, projectPipelineDetailsResponse{
			ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string{}, pipeline.DependsOn...), Dependencies: pipeline.Dependencies,
			JobsCount: pipeline.JobsCount, SupportsDryRun: pipeline.SupportsDryRun, Jobs: jobs,
			SummaryLabel:      presentation.PipelineSummaryLabel(pipeline.JobsCount, pipeline.Dependencies),
			GraphSummaryLabel: presentation.PipelineGraphSummaryLabel(pipeline.JobsCount, len(pipeline.DependsOn)),
		})
	}
	history := executionCardsToResponse(view.HistoryExecutions, false)
	return projectDetailsViewResponse{Project: project, Pipelines: pipelines, HistoryExecutions: history, HistoryEmpty: len(history) == 0}
}
