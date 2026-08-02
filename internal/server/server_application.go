package server

import (
	"context"
	"os"
	"strings"

	executionviewsadapter "github.com/izzyreal/ciwi/internal/adapters/executionviews"
	sqliteadapter "github.com/izzyreal/ciwi/internal/adapters/sqlite"
	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type serverApplication struct {
	server         *application.ServerQueries
	projects       *application.ProjectQueries
	pipelines      *application.PipelineCommands
	executions     *application.ExecutionQueries
	frontPage      *presentation.FrontPageQueries
	projectDetails *presentation.ProjectDetailsQueries
	jobDetails     *presentation.JobDetailsQueries
	changes        *application.ChangeHub
}

type localServerInfoSource struct{}

func (localServerInfoSource) ServerInfo(context.Context) (domain.ServerInfo, error) {
	host, _ := os.Hostname()
	return domain.ServerInfo{
		Name:       "ciwi",
		APIVersion: 1,
		Version:    currentVersion(),
		Hostname:   strings.TrimSpace(host),
	}, nil
}

func newServerApplication(s *stateStore) *serverApplication {
	serverQueries := application.NewServerQueries(localServerInfoSource{})
	projectQueries := application.NewProjectQueries(sqliteadapter.NewProjectRepository(s.db))
	executionQueries := application.NewExecutionQueries(executionviewsadapter.NewRepository(s.db, 40))
	changes := application.NewChangeHub()
	return &serverApplication{
		server:   serverQueries,
		projects: projectQueries,
		pipelines: application.NewPipelineCommands(
			pipelineRunnerAdapter{state: s},
			sqliteadapter.NewCommandReceiptRepository(s.db),
			changes,
		),
		executions:     executionQueries,
		frontPage:      presentation.NewFrontPageQueries(serverQueries, projectQueries, executionQueries),
		projectDetails: presentation.NewProjectDetailsQueries(projectQueries),
		jobDetails:     presentation.NewJobDetailsQueries(executionQueries),
		changes:        changes,
	}
}

func (s *stateStore) app() *serverApplication {
	s.applicationOnce.Do(func() {
		s.application = newServerApplication(s)
	})
	return s.application
}

type pipelineRunnerAdapter struct {
	state *stateStore
}

func (a pipelineRunnerAdapter) RunPipeline(ctx context.Context, request application.RunPipelineRequest) (application.RunPipelineResult, error) {
	if err := ctx.Err(); err != nil {
		return application.RunPipelineResult{}, err
	}
	pipeline, err := a.state.pipelineStore().GetPipelineByDBID(request.PipelineDBID)
	if err != nil {
		return application.RunPipelineResult{}, application.NewError(application.ErrorNotFound, err.Error(), err)
	}
	selection := &protocol.RunPipelineSelectionRequest{
		PipelineJobID: request.PipelineJobID,
		MatrixName:    request.MatrixName,
		MatrixIndex:   request.MatrixIndex,
		DryRun:        request.DryRun,
		SourceRef:     request.SourceRef,
		AgentID:       request.AgentID,
		ExecutionMode: request.ExecutionMode,
	}
	result, err := a.state.enqueuePersistedPipeline(pipeline, selection)
	if err != nil {
		return application.RunPipelineResult{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
	}
	return application.RunPipelineResult{
		ProjectName: result.ProjectName, PipelineID: result.PipelineID, Enqueued: result.Enqueued,
		JobExecutionIDs: append([]string(nil), result.JobExecutionIDs...),
	}, nil
}

func projectToProtocol(project domain.Project) protocol.ProjectSummary {
	pipelines := make([]protocol.PipelineSummary, 0, len(project.Pipelines))
	for _, pipeline := range project.Pipelines {
		pipelines = append(pipelines, protocol.PipelineSummary{
			ID: pipeline.ID, PipelineID: pipeline.PipelineID, Trigger: pipeline.Trigger,
			DependsOn: append([]string(nil), pipeline.DependsOn...), SourceRepo: pipeline.SourceRepo,
			SourceRef: pipeline.SourceRef, SupportsDryRun: pipeline.SupportsDryRun,
		})
	}
	chains := make([]protocol.PipelineChainSummary, 0, len(project.PipelineChains))
	for _, chain := range project.PipelineChains {
		chains = append(chains, protocol.PipelineChainSummary{
			ID: chain.ID, Name: chain.Name, Pipelines: append([]string(nil), chain.Pipelines...),
			SupportsDryRun: chain.SupportsDryRun, VersionPipelineID: chain.VersionPipelineID,
		})
	}
	return protocol.ProjectSummary{
		ID: project.ID, Name: project.Name, SourceKind: project.SourceKind, ConfigPath: project.ConfigPath,
		RepoURL: project.RepoURL, RepoRef: project.RepoRef, ConfigFile: project.ConfigFile,
		LoadedCommit: project.LoadedCommit, UpdatedUTC: project.UpdatedUTC,
		Pipelines: pipelines, PipelineChains: chains,
	}
}
