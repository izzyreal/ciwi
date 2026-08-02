package cnpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/izzyreal/ciwi/pkg/cnp"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/quic-go/quic-go"
)

const ALPN = "ciwi-native/1"

type Error struct {
	Code    cnpv1.StatusCode
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type Client struct {
	connection *quic.Conn
	welcome    *cnpv1.Welcome
}

func Dial(ctx context.Context, address, clientName, clientVersion string) (*Client, error) {
	// CNP v1 intentionally preserves ciwi's trusted-network model. QUIC still
	// encrypts the connection, but v1 does not authenticate endpoint identity.
	tlsConfig := &tls.Config{ // #nosec G402 -- documented CNP v1 trust model.
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{ALPN},
		InsecureSkipVerify: true,
	}
	connection, err := quic.DialAddr(ctx, address, tlsConfig, &quic.Config{
		Allow0RTT:       false,
		EnableDatagrams: false,
		KeepAlivePeriod: 3 * time.Second,
		MaxIdleTimeout:  10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("dial ciwi native endpoint: %w", err)
	}
	client := &Client{connection: connection}
	if err := client.hello(ctx, clientName, clientVersion); err != nil {
		_ = connection.CloseWithError(0x100, "hello failed")
		return nil, err
	}
	return client, nil
}

func (c *Client) Welcome() *cnpv1.Welcome { return c.welcome }

func (c *Client) Close() error {
	if c == nil || c.connection == nil {
		return nil
	}
	return c.connection.CloseWithError(0, "client closed")
}

func (c *Client) hello(ctx context.Context, clientName, clientVersion string) error {
	stream, err := c.connection.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open hello stream: %w", err)
	}
	message := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Hello{Hello: &cnpv1.Hello{
		ClientName: clientName, ClientVersion: clientVersion,
		Capabilities: []string{"protobuf", "invalidation_stream", "job_output_stream"},
	}}}
	if err := cnp.Write(stream, message); err != nil {
		stream.CancelRead(0x101)
		return fmt.Errorf("write hello: %w", err)
	}
	_ = stream.Close()
	var response cnpv1.ServerMessage
	if err := cnp.NewReader(stream).Read(&response); err != nil {
		return fmt.Errorf("read welcome: %w", err)
	}
	welcome := response.GetWelcome()
	if welcome == nil {
		return fmt.Errorf("native endpoint did not return welcome")
	}
	c.welcome = welcome
	return nil
}

func (c *Client) GetServerInfo(ctx context.Context) (*cnpv1.ServerInfo, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetServerInfo{GetServerInfo: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetServerInfo(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ListProjects(ctx context.Context) (*cnpv1.ProjectList, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ListProjects{ListProjects: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetProjectList(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetFrontPageView(ctx context.Context) (*cnpv1.FrontPageView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetFrontPageView{GetFrontPageView: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetFrontPageView(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetProjectDetails(ctx context.Context, projectID int64) (*cnpv1.ProjectDetailsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetProjectDetails{
		GetProjectDetails: &cnpv1.GetProjectDetailsRequest{ProjectId: projectID},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetProjectDetails(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetRunOptions(ctx context.Context, request *cnpv1.GetRunOptionsRequest) (*cnpv1.RunOptionsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetRunOptions{GetRunOptions: request}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetRunOptions(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetAgentsView(ctx context.Context) (*cnpv1.AgentsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetAgentsView{GetAgentsView: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetAgentsView(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) AgentAction(ctx context.Context, request *cnpv1.AgentActionRequest, idempotencyKey string) (*cnpv1.AgentActionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_AgentAction{AgentAction: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetAgentAction(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetJobDetails(ctx context.Context, jobExecutionID string) (*cnpv1.JobDetailsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetJobDetails{
		GetJobDetails: &cnpv1.GetJobDetailsRequest{JobExecutionId: jobExecutionID},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetJobDetails(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RunPipeline(ctx context.Context, request *cnpv1.RunPipelineRequest, idempotencyKey string) (*cnpv1.RunPipelineResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RunPipeline{RunPipeline: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRunPipeline(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RunPipelineChain(ctx context.Context, request *cnpv1.RunPipelineChainRequest, idempotencyKey string) (*cnpv1.RunPipelineChainResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RunPipelineChain{RunPipelineChain: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRunPipelineChain(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ClearExecutionQueue(ctx context.Context, idempotencyKey string) (*cnpv1.ClearExecutionQueueResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ClearExecutionQueue{
		ClearExecutionQueue: &cnpv1.ClearExecutionQueueRequest{},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetClearExecutionQueue(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) FlushExecutionHistory(ctx context.Context, request *cnpv1.FlushExecutionHistoryRequest, idempotencyKey string) (*cnpv1.FlushExecutionHistoryResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_FlushExecutionHistory{
		FlushExecutionHistory: request,
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetFlushExecutionHistory(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) CancelExecution(ctx context.Context, jobExecutionID, idempotencyKey string) (*cnpv1.CancelExecutionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_CancelExecution{
		CancelExecution: &cnpv1.ControlExecutionRequest{JobExecutionId: jobExecutionID},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetCancelExecution(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RerunExecution(ctx context.Context, jobExecutionID, idempotencyKey string) (*cnpv1.RerunExecutionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RerunExecution{
		RerunExecution: &cnpv1.ControlExecutionRequest{JobExecutionId: jobExecutionID},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRerunExecution(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) WatchChanges(ctx context.Context) (<-chan *cnpv1.ChangeEvent, <-chan error, error) {
	stream, err := c.connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("open watch stream: %w", err)
	}
	requestID := uuid.NewString()
	request := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Request{Request: &cnpv1.Request{
		Metadata:  &cnpv1.RequestMetadata{RequestId: requestID},
		Operation: &cnpv1.Request_WatchChanges{WatchChanges: &cnpv1.WatchChangesRequest{}},
	}}}
	if err := cnp.Write(stream, request); err != nil {
		stream.CancelRead(0x101)
		return nil, nil, fmt.Errorf("write watch request: %w", err)
	}
	_ = stream.Close()
	events := make(chan *cnpv1.ChangeEvent, 1)
	errorsOut := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errorsOut)
		stopCancellation := context.AfterFunc(ctx, func() { stream.CancelRead(0) })
		defer stopCancellation()
		reader := cnp.NewReader(stream)
		for {
			var message cnpv1.ServerMessage
			if err := reader.Read(&message); err != nil {
				if ctx.Err() == nil && !errors.Is(err, io.EOF) {
					errorsOut <- err
				}
				return
			}
			response := message.GetResponse()
			if response == nil || response.RequestId != requestID {
				errorsOut <- fmt.Errorf("invalid watch response")
				return
			}
			if status := response.GetError(); status != nil {
				errorsOut <- &Error{Code: status.Code, Message: status.Message}
				return
			}
			change := response.GetChange()
			if change == nil {
				errorsOut <- unexpectedResult(response)
				return
			}
			select {
			case events <- change:
			case <-ctx.Done():
				stream.CancelRead(0)
				return
			}
		}
	}()
	return events, errorsOut, nil
}

func (c *Client) WatchJobOutput(ctx context.Context, jobExecutionID string, afterEventID int64) (<-chan *cnpv1.JobOutputBatch, <-chan error, error) {
	stream, err := c.connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("open job output stream: %w", err)
	}
	requestID := uuid.NewString()
	request := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Request{Request: &cnpv1.Request{
		Metadata: &cnpv1.RequestMetadata{RequestId: requestID},
		Operation: &cnpv1.Request_WatchJobOutput{WatchJobOutput: &cnpv1.WatchJobOutputRequest{
			JobExecutionId: jobExecutionID, AfterEventId: afterEventID,
		}},
	}}}
	if err := cnp.Write(stream, request); err != nil {
		stream.CancelRead(0x101)
		return nil, nil, fmt.Errorf("write job output request: %w", err)
	}
	_ = stream.Close()
	batches := make(chan *cnpv1.JobOutputBatch, 1)
	errorsOut := make(chan error, 1)
	go func() {
		defer close(batches)
		defer close(errorsOut)
		stopCancellation := context.AfterFunc(ctx, func() { stream.CancelRead(0) })
		defer stopCancellation()
		reader := cnp.NewReader(stream)
		for {
			var message cnpv1.ServerMessage
			if err := reader.Read(&message); err != nil {
				if ctx.Err() == nil && !errors.Is(err, io.EOF) {
					errorsOut <- err
				}
				return
			}
			response := message.GetResponse()
			if response == nil || response.RequestId != requestID {
				errorsOut <- fmt.Errorf("invalid job output response")
				return
			}
			if status := response.GetError(); status != nil {
				errorsOut <- &Error{Code: status.Code, Message: status.Message}
				return
			}
			batch := response.GetJobOutput()
			if batch == nil {
				errorsOut <- unexpectedResult(response)
				return
			}
			select {
			case batches <- batch:
			case <-ctx.Done():
				stream.CancelRead(0)
				return
			}
		}
	}()
	return batches, errorsOut, nil
}

func (c *Client) call(ctx context.Context, request *cnpv1.Request, idempotencyKey string) (*cnpv1.Response, error) {
	stream, err := c.connection.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open native request stream: %w", err)
	}
	requestID := uuid.NewString()
	metadata := &cnpv1.RequestMetadata{RequestId: requestID, IdempotencyKey: idempotencyKey}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 {
			metadata.TimeoutMs = uint32(min(remaining.Milliseconds(), int64(^uint32(0))))
		}
	}
	request.Metadata = metadata
	message := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Request{Request: request}}
	if err := cnp.Write(stream, message); err != nil {
		stream.CancelRead(0x101)
		return nil, fmt.Errorf("write native request: %w", err)
	}
	_ = stream.Close()
	var serverMessage cnpv1.ServerMessage
	if err := cnp.NewReader(stream).Read(&serverMessage); err != nil {
		return nil, fmt.Errorf("read native response: %w", err)
	}
	response := serverMessage.GetResponse()
	if response == nil || response.RequestId != requestID {
		return nil, fmt.Errorf("invalid native response")
	}
	if status := response.GetError(); status != nil {
		return nil, &Error{Code: status.Code, Message: status.Message}
	}
	return response, nil
}

func unexpectedResult(response *cnpv1.Response) error {
	return fmt.Errorf("unexpected native response result for request %q", response.GetRequestId())
}
