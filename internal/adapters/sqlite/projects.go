package sqlite

import (
	"context"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type ProjectRepository struct {
	store *store.Store
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
