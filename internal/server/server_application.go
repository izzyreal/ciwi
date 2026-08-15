package server

import (
	"context"
	"log/slog"
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
	projectCommands   *application.ProjectCommands
	updates           *application.ServerUpdateOperations
	pipelines         *application.PipelineCommands
	pipelineChains    *application.PipelineChainCommands
	runOptions        *application.RunOptionsQueries
	agents            *presentation.AgentsQueries
	agentCommands     *application.AgentCommands
	agentScripts      *application.AgentScriptCommands
	executions        *application.ExecutionQueries
	executionCommands *application.ExecutionCommands
	executionControls *application.ExecutionControlCommands
	commandReceipts   *application.CommandReceiptQueries
	receipts          application.CommandReceiptRepository
	frontPage         *presentation.FrontPageQueries
	projectDetails    *presentation.ProjectDetailsQueries
	jobDetails        *presentation.JobDetailsQueries
	changes           *application.ChangeHub
}

type localServerInfoSource struct{ installationID string }

func (s localServerInfoSource) ServerInfo(context.Context) (domain.ServerInfo, error) {
	host, _ := os.Hostname()
	return domain.ServerInfo{
		Name:           "ciwi",
		APIVersion:     1,
		Version:        currentVersion(),
		Hostname:       strings.TrimSpace(host),
		InstallationID: strings.TrimSpace(s.installationID),
	}, nil
}

func newServerApplication(s *stateStore) *serverApplication {
	serverQueries := application.NewServerQueries(localServerInfoSource{installationID: s.installationID})
	projectQueries := application.NewProjectQueries(sqliteadapter.NewProjectRepository(s.db))
	executionStore := executionDetailsStore{Store: s.db, artifactsDir: s.artifactsDir}
	executionRepository := executionviewsadapter.NewRepository(executionStore, 40, schedulingAgentSourceAdapter{state: s})
	if s.jobProgress != nil {
		executionRepository = executionviewsadapter.NewRepositoryWithProgress(
			executionStore, 40, schedulingAgentSourceAdapter{state: s}, s.jobProgress,
		)
	}
	executionQueries := application.NewExecutionQueries(executionRepository)
	agentQueries := application.NewAgentQueries(agentRepositoryAdapter{state: s})
	changes := application.NewChangeHub()
	receipts := sqliteadapter.NewCommandReceiptRepository(s.db)
	frontPageQueries := presentation.NewFrontPageQueriesWithObserver(serverQueries, projectQueries, executionQueries, observeFrontPageTiming)
	return &serverApplication{
		server:          serverQueries,
		projects:        projectQueries,
		projectCommands: application.NewProjectCommands(projectMutatorAdapter{state: s}, receipts, changes),
		updates:         application.NewServerUpdateOperations(serverUpdateAdapter{state: s}, changes, receipts),
		pipelines: application.NewPipelineCommands(
			pipelineRunnerAdapter{state: s},
			receipts,
			changes,
		),
		pipelineChains:    application.NewPipelineChainCommands(pipelineChainRunnerAdapter{state: s}, receipts, changes),
		runOptions:        application.NewRunOptionsQueries(runOptionsAdapter{state: s}),
		agents:            presentation.NewAgentsQueries(agentQueries),
		agentCommands:     application.NewAgentCommands(agentMutatorAdapter{state: s}, receipts, changes),
		agentScripts:      application.NewAgentScriptCommands(agentScriptMutatorAdapter{state: s}, receipts, changes),
		executions:        executionQueries,
		executionCommands: application.NewExecutionCommands(executionMutatorAdapter{state: s}, receipts, changes),
		executionControls: application.NewExecutionControlCommands(executionControllerAdapter{state: s}, receipts, changes),
		commandReceipts:   application.NewCommandReceiptQueries(receipts),
		receipts:          receipts,
		frontPage:         frontPageQueries,
		projectDetails:    presentation.NewProjectDetailsQueries(projectQueries, executionQueries),
		jobDetails:        presentation.NewJobDetailsQueries(executionQueries),
		changes:           changes,
	}
}

func observeFrontPageTiming(_ context.Context, timing presentation.FrontPageTiming) {
	if timing.Err == nil && timing.Total < time.Second {
		return
	}
	attributes := []any{
		"elapsed_ms", timing.Total.Milliseconds(),
		"server_info_ms", timing.ServerInfo.Milliseconds(),
		"projects_ms", timing.Projects.Milliseconds(),
		"executions_ms", timing.Executions.Milliseconds(),
		"project_count", timing.ProjectCount,
		"queued_card_count", timing.QueuedCardCount,
		"history_card_count", timing.HistoryCardCount,
	}
	if timing.FailedPhase != "" {
		attributes = append(attributes, "failed_phase", timing.FailedPhase)
	}
	if timing.Err != nil {
		attributes = append(attributes, "error", timing.Err)
	}
	slog.Warn("front-page query slow or failed", attributes...)
}

// executionDetailsStore keeps every presentation surface on the same artifact
// list as the HTTP artifact endpoints. Test and coverage report JSON files live
// on disk rather than in the artifact table, so they must be added here before
// the shared job-details presenter builds either the native or HTTP view.
type executionDetailsStore struct {
	executionviewsadapter.Store
	artifactsDir string
}

func (s executionDetailsStore) GetJobLogDescriptor(jobID string) (domain.JobLogDescriptor, error) {
	store, ok := s.Store.(interface {
		GetJobLogDescriptor(string) (domain.JobLogDescriptor, error)
	})
	if !ok {
		return domain.JobLogDescriptor{}, domain.ErrJobExecutionNotFound
	}
	return store.GetJobLogDescriptor(jobID)
}

func (s executionDetailsStore) GetJobLogPage(jobID, itemID string, mode domain.JobLogPageMode, cursor int64) (domain.JobLogPage, error) {
	store, ok := s.Store.(interface {
		GetJobLogPage(string, string, domain.JobLogPageMode, int64) (domain.JobLogPage, error)
	})
	if !ok {
		return domain.JobLogPage{}, domain.ErrJobExecutionNotFound
	}
	return store.GetJobLogPage(jobID, itemID, mode, cursor)
}

func (s executionDetailsStore) SearchJobLog(jobID, query string, selectedIndex int64) (domain.JobLogSearchResult, error) {
	store, ok := s.Store.(interface {
		SearchJobLog(string, string, int64) (domain.JobLogSearchResult, error)
	})
	if !ok {
		return domain.JobLogSearchResult{}, domain.ErrJobExecutionNotFound
	}
	return store.SearchJobLog(jobID, query, selectedIndex)
}

func (s executionDetailsStore) ListJobExecutionArtifacts(jobID string) ([]protocol.JobExecutionArtifact, error) {
	artifacts, err := s.Store.ListJobExecutionArtifacts(jobID)
	if err != nil {
		return nil, err
	}
	artifacts = jobexecution.AppendSyntheticTestReportArtifact(s.artifactsDir, jobID, artifacts)
	artifacts = jobexecution.AppendSyntheticCoverageReportArtifact(s.artifactsDir, jobID, artifacts)
	return artifacts, nil
}

type pipelineChainRunnerAdapter struct {
	state *stateStore
}

func (a pipelineChainRunnerAdapter) RunPipelineChain(ctx context.Context, request application.RunPipelineChainRequest) (application.RunPipelineChainResult, error) {
	if err := ctx.Err(); err != nil {
		return application.RunPipelineChainResult{}, err
	}
	chain, err := a.state.pipelineStore().GetPipelineChain(request.ProjectID, request.ChainID)
	if err != nil {
		return application.RunPipelineChainResult{}, application.NewError(application.ErrorNotFound, err.Error(), err)
	}
	selection := &protocol.RunPipelineSelectionRequest{
		PipelineJobID: request.PipelineJobID, MatrixName: request.MatrixName, MatrixIndex: request.MatrixIndex,
		DryRun: request.DryRun, SourceRef: request.SourceRef, AgentID: request.AgentID, ExecutionMode: request.ExecutionMode,
	}
	result, err := a.state.enqueuePersistedPipelineChain(chain, selection)
	if err != nil {
		return application.RunPipelineChainResult{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
	}
	return application.RunPipelineChainResult{
		ProjectName: result.ProjectName, ChainID: result.PipelineChainID, ChainName: result.PipelineChainName, Enqueued: result.Enqueued,
		JobExecutionIDs: append([]string(nil), result.JobExecutionIDs...),
	}, nil
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

func (a executionMutatorAdapter) RemoveQueuedExecution(ctx context.Context, jobID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.state.jobExecutionStore().DeleteQueuedJobExecution(jobID)
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
	settings := presentation.PresentProjectSettings(project)
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
		IsManaged: settings.IsManaged, CanReload: settings.CanReload, HasRepo: settings.HasRepository,
		RepoRefLabel: settings.RepositoryRef, HasLoadedCommit: settings.HasLoadedCommit,
		LoadedCommitShort: settings.LoadedCommitShort, LoadedCommitURL: settings.LoadedCommitURL,
		SourceLabel: settings.SourceLabel,
	}
}
