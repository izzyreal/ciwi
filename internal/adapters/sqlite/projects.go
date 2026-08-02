package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type ProjectRepository struct {
	store *store.Store
}

func (r *ProjectRepository) GetProjectDetails(ctx context.Context, projectID int64) (domain.ProjectDetails, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProjectDetails{}, err
	}
	detail, err := r.store.GetProjectDetail(projectID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "project not found") {
			return domain.ProjectDetails{}, fmt.Errorf("%w: %v", domain.ErrProjectNotFound, err)
		}
		return domain.ProjectDetails{}, err
	}
	return projectDetailsFromProtocol(detail), nil
}

func NewProjectRepository(db *store.Store) *ProjectRepository {
	return &ProjectRepository{store: db}
}

func (r *ProjectRepository) ListProjects(ctx context.Context) ([]domain.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projects, err := r.store.ListProjects()
	if err != nil {
		return nil, err
	}
	out := make([]domain.Project, 0, len(projects))
	for _, project := range projects {
		out = append(out, projectFromProtocol(project))
	}
	return out, nil
}

func projectFromProtocol(project protocol.ProjectSummary) domain.Project {
	pipelines := make([]domain.Pipeline, 0, len(project.Pipelines))
	for _, pipeline := range project.Pipelines {
		pipelines = append(pipelines, domain.Pipeline{
			ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string(nil), pipeline.DependsOn...), SourceRepo: pipeline.SourceRepo,
			SourceRef: pipeline.SourceRef, SupportsDryRun: pipeline.SupportsDryRun,
		})
	}
	chains := make([]domain.PipelineChain, 0, len(project.PipelineChains))
	for _, chain := range project.PipelineChains {
		chains = append(chains, domain.PipelineChain{
			ID: chain.ID, Name: chain.Name, Pipelines: append([]string(nil), chain.Pipelines...),
			SupportsDryRun: chain.SupportsDryRun, VersionPipelineID: chain.VersionPipelineID,
		})
	}
	return domain.Project{
		ID: project.ID, Name: project.Name, SourceKind: project.SourceKind, ConfigPath: project.ConfigPath,
		RepoURL: project.RepoURL, RepoRef: project.RepoRef, ConfigFile: project.ConfigFile,
		LoadedCommit: project.LoadedCommit, UpdatedUTC: project.UpdatedUTC,
		Pipelines: pipelines, PipelineChains: chains,
	}
}

func projectDetailsFromProtocol(detail protocol.ProjectDetail) domain.ProjectDetails {
	pipelineSummaries := make([]domain.Pipeline, 0, len(detail.Pipelines))
	pipelines := make([]domain.PipelineDetails, 0, len(detail.Pipelines))
	for _, pipeline := range detail.Pipelines {
		jobs := make([]domain.PipelineJobDetails, 0, len(pipeline.Jobs))
		supportsDryRun := false
		for _, job := range pipeline.Jobs {
			steps := make([]domain.PipelineStepDetails, 0, len(job.Steps))
			for index, step := range job.Steps {
				command := step.Run
				if step.Type == "test" {
					command = step.TestCommand
				}
				steps = append(steps, domain.PipelineStepDetails{
					Index: index, Type: step.Type, Name: step.Name, TestName: step.TestName,
					Command: command, SkipDryRun: step.SkipDryRun, Environment: cloneStringMap(step.Env),
				})
				if step.SkipDryRun {
					supportsDryRun = true
				}
			}
			jobs = append(jobs, domain.PipelineJobDetails{
				ID: job.ID, Needs: append([]string{}, job.Needs...), TimeoutSeconds: job.TimeoutSeconds,
				RunsOn: cloneStringMap(job.RunsOn), RequiresTools: cloneStringMap(job.RequiresTools),
				MatrixCount: len(job.MatrixIncludes), Steps: steps,
			})
		}
		pipelineSummaries = append(pipelineSummaries, domain.Pipeline{
			ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string{}, pipeline.DependsOn...), SourceRepo: pipeline.SourceRepo,
			SourceRef: pipeline.SourceRef, SupportsDryRun: supportsDryRun,
		})
		pipelines = append(pipelines, domain.PipelineDetails{
			ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string{}, pipeline.DependsOn...), SourceRepo: pipeline.SourceRepo,
			SourceRef: pipeline.SourceRef, Jobs: jobs,
		})
	}
	chains := make([]domain.PipelineChain, 0, len(detail.PipelineChains))
	for _, chain := range detail.PipelineChains {
		chains = append(chains, domain.PipelineChain{
			ID: chain.ID, Name: chain.Name, Pipelines: append([]string{}, chain.Pipelines...),
			SupportsDryRun: chain.SupportsDryRun, VersionPipelineID: chain.VersionPipelineID,
		})
	}
	return domain.ProjectDetails{
		Project: domain.Project{
			ID: detail.ID, Name: detail.Name, SourceKind: detail.SourceKind,
			RepoURL: detail.RepoURL, RepoRef: detail.RepoRef, ConfigFile: detail.ConfigFile,
			LoadedCommit: detail.LoadedCommit, UpdatedUTC: detail.UpdatedUTC,
			Pipelines: pipelineSummaries, PipelineChains: chains,
		},
		Pipelines: pipelines,
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
