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
	StructureFilters  []projectStructureFilterResponse `json:"structure_filters"`
	HistoryExecutions []executionCardResponse          `json:"history_executions"`
	HistoryEmpty      bool                             `json:"history_empty"`
}

type projectStructureFilterResponse struct {
	Value                 string                       `json:"value"`
	Label                 string                       `json:"label"`
	PipelineIDs           []string                     `json:"pipeline_ids"`
	Root                  projectStructureRootResponse `json:"root"`
	ShowChainStructure    bool                         `json:"show_chain_structure"`
	ShowPipelineStructure bool                         `json:"show_pipeline_structure"`
}

type projectStructureRootResponse struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Meta      string `json:"meta"`
	Runnable  bool   `json:"runnable"`
	ProjectID int64  `json:"project_id"`
	ChainID   string `json:"chain_id"`
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
		project.PipelineCountLabel = view.ProjectLabels.PipelineCount
		project.SourceMetadata = view.ProjectLabels.SourceMetadata
		project.HasPipelineChains = view.ProjectLabels.HasPipelineChains
	}
	pipelines := make([]projectPipelineDetailsResponse, 0, len(view.Pipelines))
	for _, pipeline := range view.Pipelines {
		jobs := make([]projectJobDetailsResponse, 0, len(pipeline.Jobs))
		for _, job := range pipeline.Jobs {
			steps := make([]projectStepDetailsResponse, 0, len(job.Steps))
			for _, step := range job.Steps {
				steps = append(steps, projectStepDetailsResponse{
					Index: step.Index, Position: step.Position, Name: step.Name, Type: step.Type,
					Command: step.DisplayCommand, SkipDryRun: step.SkipDryRun,
					Environment:      append([]string{}, step.Environment...),
					EnvironmentLabel: step.EnvironmentLabel,
				})
			}
			runsOn := job.RunsOnLabel
			jobs = append(jobs, projectJobDetailsResponse{
				ID: job.ID, Needs: append([]string{}, job.Needs...), NeedsLabel: job.NeedsLabel,
				RunsOnLabel: runsOn, ToolsLabel: job.ToolsLabel,
				TimeoutSeconds: job.TimeoutSeconds, MatrixCount: job.MatrixCount,
				StepsCount: job.StepsCount, SupportsDryRun: job.SupportsDryRun, Steps: steps,
				SummaryLabel: job.SummaryLabel,
				TimeoutLabel: job.TimeoutLabel, MatrixLabel: job.MatrixLabel,
			})
		}
		pipelines = append(pipelines, projectPipelineDetailsResponse{
			ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string{}, pipeline.DependsOn...), Dependencies: pipeline.Dependencies,
			JobsCount: pipeline.JobsCount, SupportsDryRun: pipeline.SupportsDryRun, Jobs: jobs,
			SummaryLabel:      pipeline.SummaryLabel,
			GraphSummaryLabel: pipeline.GraphSummary,
		})
	}
	structureFilters := make([]projectStructureFilterResponse, 0, len(view.StructureFilters))
	for _, filter := range view.StructureFilters {
		structureFilters = append(structureFilters, projectStructureFilterResponse{
			Value: filter.Value, Label: filter.Label, PipelineIDs: append([]string(nil), filter.PipelineIDs...),
			Root: projectStructureRootResponse{
				ID: filter.Root.ID, Label: filter.Root.Label, Meta: filter.Root.Meta, Runnable: filter.Root.Runnable,
				ProjectID: filter.Root.ProjectID, ChainID: filter.Root.ChainID,
			},
			ShowChainStructure: filter.ShowChainStructure, ShowPipelineStructure: filter.ShowPipelineStructure,
		})
	}
	history := executionCardsToResponse(view.HistoryExecutions, false)
	return projectDetailsViewResponse{Project: project, Pipelines: pipelines, StructureFilters: structureFilters, HistoryExecutions: history, HistoryEmpty: len(history) == 0}
}
