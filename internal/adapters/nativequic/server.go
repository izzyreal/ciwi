package nativequic

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
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/quic-go/quic-go"
)

const (
	connectionProtocolError quic.ApplicationErrorCode = 0x100
	streamProtocolError     quic.StreamErrorCode      = 0x101
)

type Services struct {
	Server interface {
		GetServerInfo(context.Context) (domain.ServerInfo, error)
	}
	Projects interface {
		ListProjects(context.Context) ([]domain.Project, error)
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
	ExecutionCommands interface {
		ClearQueue(context.Context, application.ClearExecutionQueueRequest) (application.ClearExecutionQueueResult, error)
		FlushHistory(context.Context, application.FlushExecutionHistoryRequest) (application.FlushExecutionHistoryResult, error)
	}
	ExecutionControls interface {
		Cancel(context.Context, application.ExecutionControlRequest) (application.CancelExecutionResult, error)
		Rerun(context.Context, application.ExecutionControlRequest) (application.RerunExecutionResult, error)
	}
	Changes *application.ChangeHub
	Version string
}

type Server struct {
	listener *quic.Listener
	services Services
}

func Listen(address string, services Services) (*Server, error) {
	if services.Server == nil || services.Projects == nil || services.FrontPage == nil || services.ProjectDetails == nil || services.JobDetails == nil || services.Pipelines == nil || services.PipelineChains == nil || services.ExecutionCommands == nil || services.ExecutionControls == nil || services.Changes == nil {
		return nil, fmt.Errorf("native QUIC services are incomplete")
	}
	tlsConfig, err := serverTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("create native TLS configuration: %w", err)
	}
	listener, err := quic.ListenAddr(address, tlsConfig, &quic.Config{
		Allow0RTT:          false,
		EnableDatagrams:    false,
		MaxIncomingStreams: 128,
		KeepAlivePeriod:    15 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("listen for native QUIC connections: %w", err)
	}
	return &Server{listener: listener, services: services}, nil
}

func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	for {
		connection, err := s.listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, quic.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("accept native QUIC connection: %w", err)
		}
		go s.handleConnection(ctx, connection)
	}
}

func (s *Server) handleConnection(ctx context.Context, connection *quic.Conn) {
	stream, err := connection.AcceptStream(ctx)
	if err != nil {
		_ = connection.CloseWithError(connectionProtocolError, "hello stream required")
		return
	}
	reader := newFrameReader(stream)
	var message cnpv1.ClientMessage
	if err := reader.Read(&message); err != nil || message.GetHello() == nil {
		_ = connection.CloseWithError(connectionProtocolError, "valid hello required")
		return
	}
	snapshot := s.services.Changes.Snapshot()
	welcome := &cnpv1.ServerMessage{Body: &cnpv1.ServerMessage_Welcome{Welcome: &cnpv1.Welcome{
		ServerName:       "ciwi",
		ServerVersion:    s.services.Version,
		ServerInstanceId: snapshot.InstanceID,
		Capabilities: []string{
			"server_info", "projects", "front_page", "project_details", "job_details", "job_output_stream", "run_pipeline", "run_pipeline_chain", "execution_housekeeping", "execution_controls", "watch_changes",
		},
	}}}
	if err := writeFrame(stream, welcome); err != nil {
		_ = connection.CloseWithError(connectionProtocolError, "write welcome")
		return
	}
	_ = stream.Close()

	for {
		stream, err := connection.AcceptStream(ctx)
		if err != nil {
			return
		}
		go s.handleRequestStream(ctx, stream)
	}
}

func (s *Server) handleRequestStream(parent context.Context, stream *quic.Stream) {
	defer stream.Close()
	reader := newFrameReader(stream)
	var message cnpv1.ClientMessage
	if err := reader.Read(&message); err != nil {
		stream.CancelWrite(streamProtocolError)
		return
	}
	request := message.GetRequest()
	if request == nil || request.Metadata == nil || request.Metadata.RequestId == "" {
		stream.CancelWrite(streamProtocolError)
		return
	}
	ctx := parent
	cancel := func() {}
	if request.Metadata.TimeoutMs > 0 {
		ctx, cancel = context.WithTimeout(parent, time.Duration(request.Metadata.TimeoutMs)*time.Millisecond)
	}
	defer cancel()

	if _, watch := request.Operation.(*cnpv1.Request_WatchChanges); watch {
		s.writeChanges(ctx, stream, request.Metadata.RequestId)
		return
	}
	if operation, watch := request.Operation.(*cnpv1.Request_WatchJobOutput); watch {
		s.writeJobOutput(ctx, stream, request.Metadata.RequestId, operation.WatchJobOutput)
		return
	}
	response := s.execute(ctx, request)
	if err := writeFrame(stream, &cnpv1.ServerMessage{Body: &cnpv1.ServerMessage_Response{Response: response}}); err != nil && !errors.Is(err, io.EOF) {
		slog.Debug("write native response failed", "error", err)
	}
}

func (s *Server) writeJobOutput(ctx context.Context, stream *quic.Stream, requestID string, request *cnpv1.WatchJobOutputRequest) {
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
	case <-stream.Context().Done():
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
		if !waitForExecutionChange(ctx, stream, changes) {
			return
		}
	}
}

func waitForExecutionChange(ctx context.Context, stream *quic.Stream, changes <-chan application.Change) bool {
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
		case <-stream.Context().Done():
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

func (s *Server) execute(ctx context.Context, request *cnpv1.Request) *cnpv1.Response {
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

func (s *Server) writeChanges(ctx context.Context, stream *quic.Stream, requestID string) {
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
		case <-stream.Context().Done():
			return
		}
	}
}
