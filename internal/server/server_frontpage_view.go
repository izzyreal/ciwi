package server

import (
	"net/http"

	"github.com/izzyreal/ciwi/internal/domain"
)

type frontPageViewResponse struct {
	Server            serverInfoResponse         `json:"server"`
	Projects          []frontPageProjectResponse `json:"projects"`
	QueuedExecutions  []executionCardResponse    `json:"queued_executions"`
	HistoryExecutions []executionCardResponse    `json:"history_executions"`
}

type frontPageProjectResponse struct {
	ID             int64                            `json:"id"`
	Name           string                           `json:"name"`
	SourceKind     string                           `json:"source_kind"`
	ConfigPath     string                           `json:"config_path"`
	RepoURL        string                           `json:"repo_url"`
	RepoRef        string                           `json:"repo_ref"`
	ConfigFile     string                           `json:"config_file"`
	LoadedCommit   string                           `json:"loaded_commit"`
	UpdatedUnixMS  int64                            `json:"updated_unix_ms"`
	Pipelines      []frontPagePipelineResponse      `json:"pipelines"`
	PipelineChains []frontPagePipelineChainResponse `json:"pipeline_chains"`
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
}

type executionCardResponse struct {
	Key             string                         `json:"key"`
	Kind            string                         `json:"kind"`
	Title           string                         `json:"title"`
	JobExecutionIDs []string                       `json:"job_execution_ids"`
	Summary         executionSummaryResponse       `json:"summary"`
	Sections        []executionCardSectionResponse `json:"sections"`
}

type executionCardSectionResponse struct {
	Key   string                     `json:"key"`
	Label string                     `json:"label"`
	Jobs  []executionCardJobResponse `json:"jobs"`
}

type executionCardJobResponse struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Status      string `json:"status"`
	CurrentStep string `json:"current_step"`
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
	queued := executionCardsToResponse(view.QueuedExecutions)
	history := executionCardsToResponse(view.HistoryExecutions)
	writeJSON(w, http.StatusOK, frontPageViewResponse{
		Server: serverInfoResponse{
			Name: view.Server.Name, APIVersion: view.Server.APIVersion,
			Version: view.Server.Version, Hostname: view.Server.Hostname,
		},
		Projects: projects, QueuedExecutions: queued, HistoryExecutions: history,
	})
}

func frontPageProjectsToResponse(projects []domain.Project) []frontPageProjectResponse {
	out := make([]frontPageProjectResponse, 0, len(projects))
	for _, project := range projects {
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
		})
	}
	return out
}

func executionCardsToResponse(cards []domain.ExecutionCard) []executionCardResponse {
	out := make([]executionCardResponse, 0, len(cards))
	for _, card := range cards {
		out = append(out, executionCardResponse{
			Key: card.Key, Kind: card.Kind, Title: card.Title,
			JobExecutionIDs: append([]string(nil), card.JobExecutionIDs...),
			Summary: executionSummaryResponse{
				TotalJobs: card.Summary.TotalJobs, Succeeded: card.Summary.Succeeded,
				Failed: card.Summary.Failed, InProgress: card.Summary.InProgress, Waiting: card.Summary.Waiting,
			},
			Sections: executionCardSectionsToResponse(card.Sections),
		})
	}
	return out
}

func executionCardSectionsToResponse(sections []domain.ExecutionCardSection) []executionCardSectionResponse {
	out := make([]executionCardSectionResponse, 0, len(sections))
	for _, section := range sections {
		jobs := make([]executionCardJobResponse, 0, len(section.Jobs))
		for _, job := range section.Jobs {
			jobs = append(jobs, executionCardJobResponse{
				ID: job.ID, Label: job.Label, Status: job.Status, CurrentStep: job.CurrentStep,
			})
		}
		out = append(out, executionCardSectionResponse{Key: section.Key, Label: section.Label, Jobs: jobs})
	}
	return out
}
