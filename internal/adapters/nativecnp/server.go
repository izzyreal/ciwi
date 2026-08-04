package nativecnp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/pkg/cnp"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

type Services struct {
	Server interface {
		GetServerInfo(context.Context) (domain.ServerInfo, error)
	}
	Projects interface {
		ListProjects(context.Context) ([]domain.Project, error)
	}
	ProjectCommands interface {
		Execute(context.Context, application.ProjectActionRequest) (application.ProjectActionResult, error)
		Import(context.Context, application.ImportProjectRequest) (application.ImportProjectResult, error)
	}
	Updates interface {
		Status(context.Context) (application.ServerUpdateStatus, error)
		Check(context.Context) (application.ServerUpdateCheckResult, error)
		Versions(context.Context) (application.ServerUpdateVersions, error)
		Execute(context.Context, application.ServerUpdateActionRequest) (application.ServerUpdateActionResult, error)
	}
	FrontPage interface {
		GetFrontPageView(context.Context) (presentation.FrontPageView, error)
	}
	ProjectDetails interface {
		GetProjectDetailsView(context.Context, int64) (presentation.ProjectDetailsView, error)
	}
	JobDetails interface {
		GetJobDetailsView(context.Context, string) (presentation.JobDetailsView, error)
		GetJobOutputView(context.Context, string, int64) (presentation.JobOutputView, error)
	}
	Pipelines interface {
		RunPipeline(context.Context, application.RunPipelineRequest) (application.RunPipelineResult, error)
	}
	PipelineChains interface {
		RunPipelineChain(context.Context, application.RunPipelineChainRequest) (application.RunPipelineChainResult, error)
	}
	RunOptions interface {
		GetRunOptions(context.Context, application.RunOptionsRequest) (application.RunOptions, error)
	}
	Agents interface {
		GetAgentsView(context.Context) (presentation.AgentsView, error)
		GetAgentDetailsView(context.Context, string) (presentation.AgentDetailsView, error)
	}
	AgentCommands interface {
		Execute(context.Context, application.AgentActionRequest) (application.AgentActionResult, error)
	}
	ExecutionCommands interface {
		ClearQueue(context.Context, application.ClearExecutionQueueRequest) (application.ClearExecutionQueueResult, error)
		RemoveQueued(context.Context, application.RemoveQueuedExecutionRequest) (application.RemoveQueuedExecutionResult, error)
		FlushHistory(context.Context, application.FlushExecutionHistoryRequest) (application.FlushExecutionHistoryResult, error)
	}
	ExecutionControls interface {
		Cancel(context.Context, application.ExecutionControlRequest) (application.CancelExecutionResult, error)
		Rerun(context.Context, application.ExecutionControlRequest) (application.RerunExecutionResult, error)
	}
	Changes *application.ChangeHub
	Version string
}

type Handler struct {
	services Services
}

func NewHandler(services Services) (*Handler, error) {
	if services.Server == nil || services.Projects == nil || services.ProjectCommands == nil || services.Updates == nil || services.FrontPage == nil || services.ProjectDetails == nil || services.JobDetails == nil || services.Pipelines == nil || services.PipelineChains == nil || services.RunOptions == nil || services.Agents == nil || services.AgentCommands == nil || services.ExecutionCommands == nil || services.ExecutionControls == nil || services.Changes == nil {
		return nil, fmt.Errorf("native CNP services are incomplete")
	}
	return &Handler{services: services}, nil
}

func (s *Handler) ServeSession(ctx context.Context, session cnp.Session) {
	stream, err := session.AcceptStream(ctx)
	if err != nil {
		_ = session.CloseWithError(fmt.Errorf("hello stream required: %w", err))
		return
	}
	reader := newFrameReader(stream)
	var message cnpv1.ClientMessage
	if err := reader.Read(&message); err != nil || message.GetHello() == nil {
		_ = session.CloseWithError(fmt.Errorf("valid hello required"))
		return
	}
	snapshot := s.services.Changes.Snapshot()
	welcome := &cnpv1.ServerMessage{Body: &cnpv1.ServerMessage_Welcome{Welcome: &cnpv1.Welcome{
		ServerName:       "ciwi",
		ServerVersion:    s.services.Version,
		ServerInstanceId: snapshot.InstanceID,
		Capabilities: []string{
			"server_info", "server_updates", "projects", "project_actions", "project_import", "front_page", "project_details", "job_details", "job_output_stream", "run_pipeline", "run_pipeline_chain", "run_options", "agents", "agent_details", "agent_actions", "execution_housekeeping", "execution_controls", "watch_changes",
		},
	}}}
	if err := writeFrame(stream, welcome); err != nil {
		_ = session.CloseWithError(fmt.Errorf("write welcome: %w", err))
		return
	}
	_ = stream.Close()

	for {
		stream, err := session.AcceptStream(ctx)
		if err != nil {
			return
		}
		go s.handleRequestStream(ctx, stream)
	}
}

func (s *Handler) handleRequestStream(parent context.Context, stream cnp.Stream) {
	defer stream.Close()
	reader := newFrameReader(stream)
	var message cnpv1.ClientMessage
	if err := reader.Read(&message); err != nil {
		stream.CancelWrite()
		return
	}
	request := message.GetRequest()
	if request == nil || request.Metadata == nil || request.Metadata.RequestId == "" {
		stream.CancelWrite()
		return
	}
	ctx := parent
	cancel := func() {}
	if request.Metadata.TimeoutMs > 0 {
		ctx, cancel = context.WithTimeout(parent, time.Duration(request.Metadata.TimeoutMs)*time.Millisecond)
	}
	defer cancel()

	if _, watch := request.Operation.(*cnpv1.Request_WatchChanges); watch {
		s.writeChanges(ctx, stream, request.Metadata.RequestId, monitorPeerClose(stream))
		return
	}
	if operation, watch := request.Operation.(*cnpv1.Request_WatchJobOutput); watch {
		s.writeJobOutput(ctx, stream, request.Metadata.RequestId, operation.WatchJobOutput, monitorPeerClose(stream))
		return
	}
	response := s.execute(ctx, request)
	if err := writeFrame(stream, &cnpv1.ServerMessage{Body: &cnpv1.ServerMessage_Response{Response: response}}); err != nil && !errors.Is(err, io.EOF) {
		slog.Debug("write native response failed", "error", err)
	}
}

func monitorPeerClose(stream cnp.Stream) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stream)
		close(done)
	}()
	return done
}

func (s *Handler) writeJobOutput(ctx context.Context, stream cnp.Stream, requestID string, request *cnpv1.WatchJobOutputRequest, peerDone <-chan struct{}) {
	if request == nil {
		response := &cnpv1.Response{RequestId: requestID, Result: &cnpv1.Response_Error{
			Error: errorToProto(application.NewError(application.ErrorInvalidArgument, "job output request is required", nil)),
		}}
		_ = writeFrame(stream, &cnpv1.ServerMessage{Body: &cnpv1.ServerMessage_Response{Response: response}})
		return
	}
	afterEventID := request.GetAfterEventId()
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	changes := s.services.Changes.Watch(watchCtx)
	select {
	case _, ok := <-changes:
		if !ok {
			return
		}
	case <-ctx.Done():
		return
	case <-peerDone:
		return
	}
	for {
		view, err := s.services.JobDetails.GetJobOutputView(ctx, request.GetJobExecutionId(), afterEventID)
		response := &cnpv1.Response{RequestId: requestID}
		if err != nil {
			response.Result = &cnpv1.Response_Error{Error: errorToProto(err)}
		} else {
			response.Result = &cnpv1.Response_JobOutput{JobOutput: jobOutputToProto(view)}
		}
		if writeErr := writeFrame(stream, &cnpv1.ServerMessage{Body: &cnpv1.ServerMessage_Response{Response: response}}); writeErr != nil {
			return
		}
		if err != nil {
			return
		}
		afterEventID = view.NextEventID
		if view.Terminal && !view.HasMore {
			return
		}
		if view.HasMore {
			continue
		}
		if !waitForExecutionChange(ctx, peerDone, changes) {
			return
		}
	}
}

func waitForExecutionChange(ctx context.Context, peerDone <-chan struct{}, changes <-chan application.Change) bool {
	for {
		select {
		case change, ok := <-changes:
			if !ok {
				return false
			}
			if change.Resync || hasExecutionChange(change.Topics) {
				return true
			}
		case <-ctx.Done():
			return false
		case <-peerDone:
			return false
		}
	}
}

func hasExecutionChange(topics []application.ChangeTopic) bool {
	for _, topic := range topics {
		if topic == application.ChangeQueue || topic == application.ChangeHistory {
			return true
		}
	}
	return false
}

func (s *Handler) execute(ctx context.Context, request *cnpv1.Request) *cnpv1.Response {
	response := &cnpv1.Response{RequestId: request.Metadata.RequestId}
	var err error
	switch operation := request.Operation.(type) {
	case *cnpv1.Request_GetServerInfo:
		var info domain.ServerInfo
		info, err = s.services.Server.GetServerInfo(ctx)
		if err == nil {
			response.Result = &cnpv1.Response_ServerInfo{ServerInfo: serverInfoToProto(info)}
		}
	case *cnpv1.Request_ListProjects:
		var projects []domain.Project
		projects, err = s.services.Projects.ListProjects(ctx)
		if err == nil {
			response.Result = &cnpv1.Response_ProjectList{ProjectList: &cnpv1.ProjectList{Projects: projectsToProto(projects)}}
		}
	case *cnpv1.Request_GetFrontPageView:
		var view presentation.FrontPageView
		view, err = s.services.FrontPage.GetFrontPageView(ctx)
		if err == nil {
			response.Result = &cnpv1.Response_FrontPageView{FrontPageView: &cnpv1.FrontPageView{
				Server: serverInfoToProto(view.Server), Projects: projectsToProto(view.Projects),
				QueuedExecutions:  executionCardsToProto(view.QueuedExecutions),
				HistoryExecutions: executionCardsToProto(view.HistoryExecutions),
			}}
		}
	case *cnpv1.Request_GetProjectDetails:
		var view presentation.ProjectDetailsView
		view, err = s.services.ProjectDetails.GetProjectDetailsView(ctx, operation.GetProjectDetails.GetProjectId())
		if err == nil {
			response.Result = &cnpv1.Response_ProjectDetails{ProjectDetails: projectDetailsToProto(view)}
		}
	case *cnpv1.Request_GetJobDetails:
		var view presentation.JobDetailsView
		view, err = s.services.JobDetails.GetJobDetailsView(ctx, operation.GetJobDetails.GetJobExecutionId())
		if err == nil {
			response.Result = &cnpv1.Response_JobDetails{JobDetails: jobDetailsToProto(view)}
		}
	case *cnpv1.Request_RunPipeline:
		var result application.RunPipelineResult
		result, err = s.services.Pipelines.RunPipeline(ctx, runPipelineRequestFromProto(operation.RunPipeline, request.Metadata.IdempotencyKey))
		if err == nil {
			response.Result = &cnpv1.Response_RunPipeline{RunPipeline: &cnpv1.RunPipelineResult{
				ProjectName: result.ProjectName, PipelineId: result.PipelineID, Enqueued: uint32(result.Enqueued),
				JobExecutionIds: append([]string(nil), result.JobExecutionIDs...),
			}}
		}
	case *cnpv1.Request_RunPipelineChain:
		var result application.RunPipelineChainResult
		result, err = s.services.PipelineChains.RunPipelineChain(ctx, runPipelineChainRequestFromProto(operation.RunPipelineChain, request.Metadata.IdempotencyKey))
		if err == nil {
			response.Result = &cnpv1.Response_RunPipelineChain{RunPipelineChain: &cnpv1.RunPipelineChainResult{
				ProjectName: result.ProjectName, ChainId: result.ChainID, ChainName: result.ChainName, Enqueued: uint32(result.Enqueued),
				JobExecutionIds: append([]string(nil), result.JobExecutionIDs...),
			}}
		}
	case *cnpv1.Request_GetRunOptions:
		var result application.RunOptions
		result, err = s.services.RunOptions.GetRunOptions(ctx, runOptionsRequestFromProto(operation.GetRunOptions))
		if err == nil {
			response.Result = &cnpv1.Response_RunOptions{RunOptions: runOptionsToProto(result)}
		}
	case *cnpv1.Request_GetAgentsView:
		var result presentation.AgentsView
		result, err = s.services.Agents.GetAgentsView(ctx)
		if err == nil {
			response.Result = &cnpv1.Response_AgentsView{AgentsView: agentsViewToProto(result)}
		}
	case *cnpv1.Request_GetAgentDetails:
		var result presentation.AgentDetailsView
		result, err = s.services.Agents.GetAgentDetailsView(ctx, operation.GetAgentDetails.GetAgentId())
		if err == nil {
			response.Result = &cnpv1.Response_AgentDetails{AgentDetails: agentDetailsToProto(result)}
		}
	case *cnpv1.Request_AgentAction:
		var result application.AgentActionResult
		result, err = s.services.AgentCommands.Execute(ctx, application.AgentActionRequest{
			AgentID: operation.AgentAction.GetAgentId(), Action: operation.AgentAction.GetAction(),
			IdempotencyKey: request.Metadata.IdempotencyKey,
		})
		if err == nil {
			response.Result = &cnpv1.Response_AgentAction{AgentAction: &cnpv1.AgentActionResult{
				Requested: result.Requested, AgentId: result.AgentID, Message: result.Message, Target: result.Target,
			}}
		}
	case *cnpv1.Request_ProjectAction:
		var result application.ProjectActionResult
		result, err = s.services.ProjectCommands.Execute(ctx, application.ProjectActionRequest{
			ProjectID: operation.ProjectAction.GetProjectId(), Action: operation.ProjectAction.GetAction(),
			IdempotencyKey: request.Metadata.IdempotencyKey,
		})
		if err == nil {
			response.Result = &cnpv1.Response_ProjectAction{ProjectAction: &cnpv1.ProjectActionResult{
				ProjectId: result.ProjectID, Message: result.Message,
			}}
		}
	case *cnpv1.Request_ImportProject:
		var result application.ImportProjectResult
		result, err = s.services.ProjectCommands.Import(ctx, application.ImportProjectRequest{
			RepoURL: operation.ImportProject.GetRepoUrl(), RepoRef: operation.ImportProject.GetRepoRef(),
			ConfigFile: operation.ImportProject.GetConfigFile(), IdempotencyKey: request.Metadata.IdempotencyKey,
		})
		if err == nil {
			response.Result = &cnpv1.Response_ImportProject{ImportProject: &cnpv1.ImportProjectResult{
				ProjectName: result.ProjectName, RepoUrl: result.RepoURL, RepoRef: result.RepoRef,
				ConfigFile: result.ConfigFile, Pipelines: uint32(max(result.Pipelines, 0)),
			}}
		}
	case *cnpv1.Request_GetServerUpdateStatus:
		var result application.ServerUpdateStatus
		result, err = s.services.Updates.Status(ctx)
		if err == nil {
			response.Result = &cnpv1.Response_ServerUpdateStatus{ServerUpdateStatus: serverUpdateStatusToProto(result)}
		}
	case *cnpv1.Request_CheckServerUpdates:
		var result application.ServerUpdateCheckResult
		result, err = s.services.Updates.Check(ctx)
		if err == nil {
			response.Result = &cnpv1.Response_ServerUpdateCheck{ServerUpdateCheck: &cnpv1.ServerUpdateCheckResult{
				CurrentVersion: result.CurrentVersion, LatestVersion: result.LatestVersion,
				AvailableVersions: append([]string(nil), result.AvailableVersions...), UpdateAvailable: result.UpdateAvailable,
				ReleaseUrl: result.ReleaseURL, AssetName: result.AssetName, Message: result.Message,
			}}
		}
	case *cnpv1.Request_ListServerUpdateVersions:
		var result application.ServerUpdateVersions
		result, err = s.services.Updates.Versions(ctx)
		if err == nil {
			response.Result = &cnpv1.Response_ServerUpdateVersions{ServerUpdateVersions: &cnpv1.ServerUpdateVersions{
				Versions: append([]string(nil), result.Versions...), CurrentVersion: result.CurrentVersion,
			}}
		}
	case *cnpv1.Request_ServerUpdateAction:
		var result application.ServerUpdateActionResult
		result, err = s.services.Updates.Execute(ctx, application.ServerUpdateActionRequest{
			Action: operation.ServerUpdateAction.GetAction(), TargetVersion: operation.ServerUpdateAction.GetTargetVersion(),
		})
		if err == nil {
			response.Result = &cnpv1.Response_ServerUpdateAction{ServerUpdateAction: &cnpv1.ServerUpdateActionResult{
				Updated: result.Updated, Restarting: result.Restarting, Staged: result.Staged, Message: result.Message,
				TargetVersion: result.TargetVersion, CurrentVersion: result.CurrentVersion,
			}}
		}
	case *cnpv1.Request_ClearExecutionQueue:
		var result application.ClearExecutionQueueResult
		result, err = s.services.ExecutionCommands.ClearQueue(ctx, application.ClearExecutionQueueRequest{IdempotencyKey: request.Metadata.IdempotencyKey})
		if err == nil {
			response.Result = &cnpv1.Response_ClearExecutionQueue{ClearExecutionQueue: &cnpv1.ClearExecutionQueueResult{Cleared: result.Cleared}}
		}
	case *cnpv1.Request_FlushExecutionHistory:
		operationRequest := operation.FlushExecutionHistory
		var result application.FlushExecutionHistoryResult
		result, err = s.services.ExecutionCommands.FlushHistory(ctx, application.FlushExecutionHistoryRequest{
			All: operationRequest.GetAll(), JobExecutionIDs: append([]string(nil), operationRequest.GetJobExecutionIds()...),
			IdempotencyKey: request.Metadata.IdempotencyKey,
		})
		if err == nil {
			response.Result = &cnpv1.Response_FlushExecutionHistory{FlushExecutionHistory: &cnpv1.FlushExecutionHistoryResult{Flushed: result.Flushed}}
		}
	case *cnpv1.Request_RemoveQueuedExecution:
		var result application.RemoveQueuedExecutionResult
		result, err = s.services.ExecutionCommands.RemoveQueued(ctx, application.RemoveQueuedExecutionRequest{
			JobExecutionID: operation.RemoveQueuedExecution.GetJobExecutionId(), IdempotencyKey: request.Metadata.IdempotencyKey,
		})
		if err == nil {
			response.Result = &cnpv1.Response_RemoveQueuedExecution{RemoveQueuedExecution: &cnpv1.RemoveQueuedExecutionResult{
				JobExecutionId: result.JobExecutionID, Removed: result.Removed,
			}}
		}
	case *cnpv1.Request_CancelExecution:
		var result application.CancelExecutionResult
		result, err = s.services.ExecutionControls.Cancel(ctx, application.ExecutionControlRequest{
			JobExecutionID: operation.CancelExecution.GetJobExecutionId(), IdempotencyKey: request.Metadata.IdempotencyKey,
		})
		if err == nil {
			response.Result = &cnpv1.Response_CancelExecution{CancelExecution: &cnpv1.CancelExecutionResult{JobExecutionId: result.JobExecutionID, Status: result.Status}}
		}
	case *cnpv1.Request_RerunExecution:
		var result application.RerunExecutionResult
		result, err = s.services.ExecutionControls.Rerun(ctx, application.ExecutionControlRequest{
			JobExecutionID: operation.RerunExecution.GetJobExecutionId(), IdempotencyKey: request.Metadata.IdempotencyKey,
		})
		if err == nil {
			response.Result = &cnpv1.Response_RerunExecution{RerunExecution: &cnpv1.RerunExecutionResult{
				OriginalJobExecutionId: result.OriginalJobExecutionID, JobExecutionId: result.JobExecutionID, Status: result.Status,
			}}
		}
	default:
		err = application.NewError(application.ErrorUnsupported, "unsupported native operation", nil)
	}
	if err != nil {
		response.Result = &cnpv1.Response_Error{Error: errorToProto(err)}
	}
	return response
}

func (s *Handler) writeChanges(ctx context.Context, stream cnp.Stream, requestID string, peerDone <-chan struct{}) {
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	changes := s.services.Changes.Watch(watchCtx)
	for {
		select {
		case change, ok := <-changes:
			if !ok {
				return
			}
			response := &cnpv1.Response{RequestId: requestID, Result: &cnpv1.Response_Change{Change: changeToProto(change)}}
			if err := writeFrame(stream, &cnpv1.ServerMessage{Body: &cnpv1.ServerMessage_Response{Response: response}}); err != nil {
				return
			}
		case <-ctx.Done():
			return
		case <-peerDone:
			return
		}
	}
}
