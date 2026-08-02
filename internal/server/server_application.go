package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	executionviewsadapter "github.com/izzyreal/ciwi/internal/adapters/executionviews"
	sqliteadapter "github.com/izzyreal/ciwi/internal/adapters/sqlite"
	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobexecution"
)

type serverApplication struct {
	server            *application.ServerQueries
	projects          *application.ProjectQueries
	pipelines         *application.PipelineCommands
	executions        *application.ExecutionQueries
	executionCommands *application.ExecutionCommands
	executionControls *application.ExecutionControlCommands
	frontPage         *presentation.FrontPageQueries
	projectDetails    *presentation.ProjectDetailsQueries
	jobDetails        *presentation.JobDetailsQueries
	changes           *application.ChangeHub
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
	receipts := sqliteadapter.NewCommandReceiptRepository(s.db)
	return &serverApplication{
		server:   serverQueries,
		projects: projectQueries,
		pipelines: application.NewPipelineCommands(
			pipelineRunnerAdapter{state: s},
			receipts,
			changes,
		),
		executions:        executionQueries,
		executionCommands: application.NewExecutionCommands(executionMutatorAdapter{state: s}, receipts, changes),
		executionControls: application.NewExecutionControlCommands(executionControllerAdapter{state: s}, receipts, changes),
		frontPage:         presentation.NewFrontPageQueries(serverQueries, projectQueries, executionQueries),
		projectDetails:    presentation.NewProjectDetailsQueries(projectQueries),
		jobDetails:        presentation.NewJobDetailsQueries(executionQueries),
		changes:           changes,
	}
}

type executionControllerAdapter struct {
	state *stateStore
}

func (a executionControllerAdapter) CancelExecution(ctx context.Context, jobID string) (application.CancelExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return application.CancelExecutionResult{}, err
	}
	job, err := jobexecution.CancelJobExecution(a.state.jobExecutionStore(), jobID, time.Now().UTC())
	if err != nil {
		return application.CancelExecutionResult{}, executionControlError(err)
	}
	return application.CancelExecutionResult{JobExecutionID: job.ID, Status: protocol.NormalizeJobExecutionStatus(job.Status)}, nil
}

func (a executionControllerAdapter) RerunExecution(ctx context.Context, jobID string) (application.RerunExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return application.RerunExecutionResult{}, err
	}
	job, err := jobexecution.RerunJobExecution(a.state.jobExecutionStore(), jobID, a.state.prepareJobExecutionRerun)
	if err != nil {
		return application.RerunExecutionResult{}, executionControlError(err)
	}
	return application.RerunExecutionResult{
		OriginalJobExecutionID: jobID, JobExecutionID: job.ID,
		Status: protocol.NormalizeJobExecutionStatus(job.Status),
	}, nil
}

func executionControlError(err error) error {
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(strings.ToLower(message), "not found"):
		return application.NewError(application.ErrorNotFound, message, err)
	case strings.Contains(message, "not active"), strings.Contains(message, "has not started"), strings.Contains(message, "dependencies"), strings.Contains(message, "required job"):
		return application.NewError(application.ErrorFailedPrecondition, message, err)
	default:
		return application.WrapInternal("control job execution", err)
	}
}

type executionMutatorAdapter struct {
	state *stateStore
}

func (a executionMutatorAdapter) ClearQueuedExecutions(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return a.state.jobExecutionStore().ClearQueuedJobExecutions()
}

func (a executionMutatorAdapter) FlushExecutionHistory(ctx context.Context, all bool, jobIDs []string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var (
		deleted []string
		err     error
	)
	if all {
		deleted, err = a.state.jobExecutionStore().FlushJobExecutionHistory()
	} else {
		deleted, err = a.state.jobExecutionStore().FlushJobExecutionHistoryByIDs(jobIDs)
	}
	if err != nil {
		return nil, err
	}
	for _, jobID := range deleted {
		_ = os.RemoveAll(filepath.Join(a.state.artifactsDir, jobID))
	}
	return deleted, nil
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
