package server

import (
	"net/http"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
)

type frontPageViewResponse struct {
	Server            serverInfoResponse         `json:"server"`
	Projects          []frontPageProjectResponse `json:"projects"`
	QueuedExecutions  []executionCardResponse    `json:"queued_executions"`
	HistoryExecutions []executionCardResponse    `json:"history_executions"`
	Loading           bool                       `json:"loading"`
	QueuedEmpty       bool                       `json:"queued_empty"`
	HistoryEmpty      bool                       `json:"history_empty"`
}

type frontPageProjectResponse struct {
	ID                 int64                            `json:"id"`
	Name               string                           `json:"name"`
	SourceKind         string                           `json:"source_kind"`
	ConfigPath         string                           `json:"config_path"`
	RepoURL            string                           `json:"repo_url"`
	RepoRef            string                           `json:"repo_ref"`
	ConfigFile         string                           `json:"config_file"`
	LoadedCommit       string                           `json:"loaded_commit"`
	UpdatedUnixMS      int64                            `json:"updated_unix_ms"`
	Pipelines          []frontPagePipelineResponse      `json:"pipelines"`
	PipelineChains     []frontPagePipelineChainResponse `json:"pipeline_chains"`
	PipelineCountLabel string                           `json:"pipeline_count_label"`
	SourceMetadata     string                           `json:"source_metadata"`
	HasPipelineChains  bool                             `json:"has_pipeline_chains"`
}

type frontPagePipelineResponse struct {
	ID             int64    `json:"id"`
	PipelineID     string   `json:"pipeline_id"`
	Trigger        string   `json:"trigger"`
	DependsOn      []string `json:"depends_on"`
	SourceRepo     string   `json:"source_repo"`
	SourceRef      string   `json:"source_ref"`
	SupportsDryRun bool     `json:"supports_dry_run"`
}

type frontPagePipelineChainResponse struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Pipelines         []string `json:"pipelines"`
	SupportsDryRun    bool     `json:"supports_dry_run"`
	VersionPipelineID int64    `json:"version_pipeline_id"`
	SequenceLabel     string   `json:"sequence_label"`
}

type executionCardResponse struct {
	Key                string                         `json:"key"`
	Kind               string                         `json:"kind"`
	Title              string                         `json:"title"`
	JobExecutionIDs    []string                       `json:"job_execution_ids"`
	Summary            executionSummaryResponse       `json:"summary"`
	Sections           []executionCardSectionResponse `json:"sections"`
	Progress           domain.Progress                `json:"progress"`
	Status             string                         `json:"status"`
	SummaryTone        string                         `json:"summary_tone"`
	SummaryLabel       string                         `json:"summary_label"`
	JobExecutionIDsCSV string                         `json:"job_execution_ids_csv"`
}

type executionCardSectionResponse struct {
	Key      string                     `json:"key"`
	Label    string                     `json:"label"`
	Jobs     []executionCardJobResponse `json:"jobs"`
	Progress domain.Progress            `json:"progress"`
}

type executionCardJobResponse struct {
	ID            string          `json:"id"`
	Label         string          `json:"label"`
	Status        string          `json:"status"`
	StatusLabel   string          `json:"status_label"`
	PipelineID    string          `json:"pipeline_id"`
	BuildLabel    string          `json:"build_label"`
	AgentID       string          `json:"agent_id"`
	CreatedUTC    string          `json:"created_utc"`
	StartedUTC    string          `json:"started_utc"`
	FinishedUTC   string          `json:"finished_utc"`
	Reason        string          `json:"reason"`
	Action        string          `json:"action"`
	CurrentStep   string          `json:"current_step"`
	Progress      domain.Progress `json:"progress"`
	CreatedLabel  string          `json:"created_label"`
	DurationLabel string          `json:"duration_label"`
}

type executionSummaryResponse struct {
	TotalJobs  int `json:"total_jobs"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	InProgress int `json:"in_progress"`
	Waiting    int `json:"waiting"`
}

func (s *stateStore) frontPageViewHandler(w http.ResponseWriter, r *http.Request) {
	view, err := s.app().frontPage.GetFrontPageView(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects := frontPageProjectsToResponse(view.Projects)
	queued := executionCardsToResponse(view.QueuedExecutions, true)
	history := executionCardsToResponse(view.HistoryExecutions, false)
	writeJSON(w, http.StatusOK, frontPageViewResponse{
		Server: serverInfoResponse{
			Name: view.Server.Name, APIVersion: view.Server.APIVersion,
			Version: view.Server.Version, Hostname: view.Server.Hostname,
		},
		Projects: projects, QueuedExecutions: queued, HistoryExecutions: history,
		QueuedEmpty: len(queued) == 0, HistoryEmpty: len(history) == 0,
	})
}

func frontPageProjectsToResponse(projects []domain.Project) []frontPageProjectResponse {
	out := make([]frontPageProjectResponse, 0, len(projects))
	for _, project := range projects {
		labels := presentation.PresentProjectLabels(project)
		pipelines := make([]frontPagePipelineResponse, 0, len(project.Pipelines))
		for _, pipeline := range project.Pipelines {
			pipelines = append(pipelines, frontPagePipelineResponse{
				ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
				DependsOn: append([]string{}, pipeline.DependsOn...), SourceRepo: pipeline.SourceRepo,
				SourceRef: pipeline.SourceRef, SupportsDryRun: pipeline.SupportsDryRun,
			})
		}
		chains := make([]frontPagePipelineChainResponse, 0, len(project.PipelineChains))
		for _, chain := range project.PipelineChains {
			chains = append(chains, frontPagePipelineChainResponse{
				ID: chain.ID, Name: chain.Name, Pipelines: append([]string{}, chain.Pipelines...),
				SupportsDryRun: chain.SupportsDryRun, VersionPipelineID: chain.VersionPipelineID,
				SequenceLabel: presentation.PipelineChainSequenceLabel(chain.Pipelines),
			})
		}
		updatedUnixMS := int64(0)
		if !project.UpdatedUTC.IsZero() {
			updatedUnixMS = project.UpdatedUTC.UnixMilli()
		}
		out = append(out, frontPageProjectResponse{
			ID: project.ID, Name: project.Name, SourceKind: project.SourceKind, ConfigPath: project.ConfigPath,
			RepoURL: project.RepoURL, RepoRef: project.RepoRef, ConfigFile: project.ConfigFile,
			LoadedCommit: project.LoadedCommit, UpdatedUnixMS: updatedUnixMS,
			Pipelines: pipelines, PipelineChains: chains,
			PipelineCountLabel: labels.PipelineCount,
			SourceMetadata:     labels.SourceMetadata,
			HasPipelineChains:  labels.HasPipelineChains,
		})
	}
	return out
}

func executionCardsToResponse(cards []domain.ExecutionCard, queued bool) []executionCardResponse {
	out := make([]executionCardResponse, 0, len(cards))
	for _, card := range cards {
		display := presentation.PresentExecutionCard(card, queued)
		out = append(out, executionCardResponse{
			Key: card.Key, Kind: card.Kind, Title: card.Title,
			JobExecutionIDs: append([]string(nil), card.JobExecutionIDs...),
			Summary: executionSummaryResponse{
				TotalJobs: card.Summary.TotalJobs, Succeeded: card.Summary.Succeeded,
				Failed: card.Summary.Failed, InProgress: card.Summary.InProgress, Waiting: card.Summary.Waiting,
			},
			Sections: executionCardSectionsToResponse(card.Sections), Progress: card.Progress,
			Status: display.Status, SummaryTone: display.SummaryTone, SummaryLabel: display.SummaryLabel,
			JobExecutionIDsCSV: display.JobExecutionIDsCSV,
		})
	}
	return out
}

func executionCardSectionsToResponse(sections []domain.ExecutionCardSection) []executionCardSectionResponse {
	now := time.Now()
	out := make([]executionCardSectionResponse, 0, len(sections))
	for _, section := range sections {
		jobs := make([]executionCardJobResponse, 0, len(section.Jobs))
		for _, job := range section.Jobs {
			display := presentation.PresentExecutionCardJob(job, now)
			jobs = append(jobs, executionCardJobResponse{
				ID: job.ID, Label: job.Label, Status: job.Status, StatusLabel: display.StatusLabel,
				PipelineID: job.PipelineID, BuildLabel: job.BuildLabel, AgentID: job.AgentID,
				CreatedUTC: formatExecutionCardTime(job.CreatedUTC), StartedUTC: formatExecutionCardTime(job.StartedUTC),
				FinishedUTC: formatExecutionCardTime(job.FinishedUTC), Reason: job.Reason, Action: job.Action,
				CurrentStep: job.CurrentStep, Progress: job.Progress,
				CreatedLabel: display.CreatedLabel, DurationLabel: display.DurationLabel,
			})
		}
		out = append(out, executionCardSectionResponse{
			Key: section.Key, Label: section.Label, Jobs: jobs, Progress: section.Progress,
		})
	}
	return out
}

func formatExecutionCardTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
