package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/server/grpcapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ciwiGRPCServer struct {
	grpcapi.UnimplementedCiwiServiceServer
	router http.Handler
}

func newCiwiGRPCServer(router http.Handler) *ciwiGRPCServer {
	return &ciwiGRPCServer{router: router}
}

func registerCiwiGRPCService(s *grpc.Server, impl grpcapi.CiwiServiceServer) {
	grpcapi.RegisterCiwiServiceServer(s, impl)
}

func (g *ciwiGRPCServer) GetServerInfo(ctx context.Context, _ *emptypb.Empty) (*grpcapi.GetServerInfoResponse, error) {
	resp := &grpcapi.GetServerInfoResponse{}
	if err := g.invokeAndDecodeJSON(ctx, http.MethodGet, "/api/v1/server-info", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) ListProjects(ctx context.Context, _ *emptypb.Empty) (*grpcapi.ListProjectsResponse, error) {
	resp := &grpcapi.ListProjectsResponse{}
	if err := g.invokeAndDecodeJSON(ctx, http.MethodGet, "/api/v1/projects", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) ListAgents(ctx context.Context, _ *emptypb.Empty) (*grpcapi.ListAgentsResponse, error) {
	resp := &grpcapi.ListAgentsResponse{}
	if err := g.invokeAndDecodeJSON(ctx, http.MethodGet, "/api/v1/agents", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) GetAgent(ctx context.Context, req *grpcapi.GetAgentRequest) (*grpcapi.GetAgentResponse, error) {
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	resp := &grpcapi.GetAgentResponse{}
	path := "/api/v1/agents/" + url.PathEscape(agentID)
	if err := g.invokeAndDecodeJSON(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) ListJobs(ctx context.Context, req *grpcapi.ListJobsRequest) (*grpcapi.ListJobsResponse, error) {
	query := url.Values{}
	if view := strings.TrimSpace(req.GetView()); view != "" {
		query.Set("view", view)
	}
	if max := int(req.GetMax()); max > 0 {
		query.Set("max", strconv.Itoa(max))
	}
	if offset := int(req.GetOffset()); offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}
	if limit := int(req.GetLimit()); limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/jobs"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	resp := &grpcapi.ListJobsResponse{}
	if err := g.invokeAndDecodeJSON(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) RunPipeline(ctx context.Context, req *grpcapi.RunPipelineRequest) (*grpcapi.RunPipelineResponse, error) {
	pipelineID := strings.TrimSpace(req.GetPipelineId())
	if pipelineID == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline_id is required")
	}
	body := map[string]any{}
	if req.GetDryRun() {
		body["dry_run"] = true
	}
	path := "/api/v1/pipelines/" + url.PathEscape(pipelineID) + "/run"
	resp := &grpcapi.RunPipelineResponse{}
	if err := g.invokeAndDecodeJSON(ctx, http.MethodPost, path, body, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) ClearQueue(ctx context.Context, _ *emptypb.Empty) (*grpcapi.ClearQueueResponse, error) {
	resp := &grpcapi.ClearQueueResponse{}
	if err := g.invokeAndDecodeJSON(ctx, http.MethodPost, "/api/v1/jobs/clear-queue", map[string]any{}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) FlushHistory(ctx context.Context, _ *emptypb.Empty) (*grpcapi.FlushHistoryResponse, error) {
	resp := &grpcapi.FlushHistoryResponse{}
	if err := g.invokeAndDecodeJSON(ctx, http.MethodPost, "/api/v1/jobs/flush-history", map[string]any{}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) RequestAgentUpdate(ctx context.Context, req *grpcapi.AgentRequest) (*grpcapi.AgentActionResponse, error) {
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	resp := &grpcapi.AgentActionResponse{}
	path := "/api/v1/agents/" + url.PathEscape(agentID) + "/update"
	if err := g.invokeAndDecodeJSON(ctx, http.MethodPost, path, map[string]any{}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) RequestAgentToolsRefresh(ctx context.Context, req *grpcapi.AgentRequest) (*grpcapi.AgentActionResponse, error) {
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	resp := &grpcapi.AgentActionResponse{}
	path := "/api/v1/agents/" + url.PathEscape(agentID) + "/refresh-tools"
	if err := g.invokeAndDecodeJSON(ctx, http.MethodPost, path, map[string]any{}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) RunAdhocScript(ctx context.Context, req *grpcapi.RunAdhocScriptRequest) (*grpcapi.RunAdhocScriptResponse, error) {
	agentID := strings.TrimSpace(req.GetAgentId())
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	shell := strings.TrimSpace(req.GetShell())
	if shell == "" {
		return nil, status.Error(codes.InvalidArgument, "shell is required")
	}
	script := strings.TrimSpace(req.GetScript())
	if script == "" {
		return nil, status.Error(codes.InvalidArgument, "script is required")
	}
	body := map[string]any{
		"shell":  shell,
		"script": script,
	}
	if timeout := int(req.GetTimeoutSeconds()); timeout > 0 {
		body["timeout_seconds"] = timeout
	}
	resp := &grpcapi.RunAdhocScriptResponse{}
	path := "/api/v1/agents/" + url.PathEscape(agentID) + "/run-script"
	if err := g.invokeAndDecodeJSON(ctx, http.MethodPost, path, body, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (g *ciwiGRPCServer) WatchState(req *grpcapi.WatchStateRequest, stream grpc.ServerStreamingServer[grpcapi.WatchStateEvent]) error {
	ctx := stream.Context()

	interval := 2 * time.Second
	if v := int(req.GetIntervalMs()); v > 0 {
		if v < 250 {
			v = 250
		}
		if v > 30_000 {
			v = 30_000
		}
		interval = time.Duration(v) * time.Millisecond
	}

	jobsLimit := 150
	if v := int(req.GetJobsLimit()); v > 0 {
		if v > 2000 {
			v = 2000
		}
		jobsLimit = v
	}

	includeProjects := req.GetIncludeProjects()
	includeAgents := req.GetIncludeAgents()
	includeJobsSummary := req.GetIncludeJobsSummary()
	includeQueuedJobs := req.GetIncludeQueuedJobs()
	includeHistoryJobs := req.GetIncludeHistoryJobs()
	if !includeProjects && !includeAgents && !includeJobsSummary && !includeQueuedJobs && !includeHistoryJobs {
		includeProjects = true
		includeAgents = true
		includeJobsSummary = true
		includeQueuedJobs = true
		includeHistoryJobs = true
	}

	streamID := "watch-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	seq := int64(0)
	sendSnapshot := func() error {
		seq++
		evt := &grpcapi.WatchStateEvent{
			StreamId: streamID,
			Seq:      seq,
			SentUtc:  time.Now().UTC().Format(time.RFC3339Nano),
		}

		serverInfo, err := g.GetServerInfo(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		evt.ServerInfo = serverInfo

		if includeProjects {
			projects, err := g.ListProjects(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			evt.Projects = projects
		}
		if includeAgents {
			agents, err := g.ListAgents(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			evt.Agents = agents
		}
		if includeJobsSummary {
			summary, err := g.ListJobs(ctx, &grpcapi.ListJobsRequest{
				View: "summary",
				Max:  int32(jobsLimit),
			})
			if err != nil {
				return err
			}
			evt.JobsSummary = summary
		}
		if includeQueuedJobs {
			queued, err := g.ListJobs(ctx, &grpcapi.ListJobsRequest{
				View:   "queued",
				Max:    int32(jobsLimit),
				Offset: 0,
				Limit:  int32(jobsLimit),
			})
			if err != nil {
				return err
			}
			evt.QueuedJobs = queued
		}
		if includeHistoryJobs {
			history, err := g.ListJobs(ctx, &grpcapi.ListJobsRequest{
				View:   "history",
				Max:    int32(jobsLimit),
				Offset: 0,
				Limit:  int32(jobsLimit),
			})
			if err != nil {
				return err
			}
			evt.HistoryJobs = history
		}

		if err := stream.Send(evt); err != nil {
			return err
		}
		return nil
	}

	if err := sendSnapshot(); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := sendSnapshot(); err != nil {
				return err
			}
		}
	}
}

func (g *ciwiGRPCServer) invokeAndDecodeJSON(ctx context.Context, method, targetPath string, body map[string]any, out proto.Message) error {
	raw, err := g.invokeJSON(ctx, method, targetPath, body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, out); err != nil {
		return status.Errorf(codes.Internal, "decode JSON response: %v", err)
	}
	return nil
}

func (g *ciwiGRPCServer) invokeJSON(ctx context.Context, method, targetPath string, body map[string]any) ([]byte, error) {
	if g == nil || g.router == nil {
		return nil, status.Error(codes.Internal, "gRPC bridge is not initialized")
	}

	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "marshal request body: %v", err)
		}
		payload = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, targetPath, payload).WithContext(ctx)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	g.router.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	trimmed := strings.TrimSpace(string(rawBody))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if trimmed == "" {
			trimmed = http.StatusText(resp.StatusCode)
		}
		return nil, status.Errorf(httpStatusToGRPCCode(resp.StatusCode), "http %d: %s", resp.StatusCode, trimmed)
	}
	return rawBody, nil
}

func httpStatusToGRPCCode(statusCode int) codes.Code {
	switch statusCode {
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.FailedPrecondition
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case http.StatusMethodNotAllowed:
		return codes.Unimplemented
	default:
		if statusCode >= 500 {
			return codes.Internal
		}
		return codes.Unknown
	}
}
