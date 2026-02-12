package server

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/server/grpcapi"
	"github.com/izzyreal/ciwi/internal/store"
	"github.com/izzyreal/ciwi/internal/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

func newGRPCTestClient(t *testing.T) (grpcapi.CiwiServiceClient, *stateStore) {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "ciwi.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	artifactsDir := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("create artifacts dir: %v", err)
	}

	s := &stateStore{
		agents:           make(map[string]agentState),
		agentUpdates:     make(map[string]string),
		agentToolRefresh: make(map[string]bool),
		db:               db,
		artifactsDir:     artifactsDir,
		vaultTokens:      newVaultTokenCache(),
	}
	router := buildRouter(s, s.artifactsDir)

	listener := bufconn.Listen(1024 * 1024)
	grpcSrv := grpc.NewServer()
	registerCiwiGRPCService(grpcSrv, newCiwiGRPCServer(router))
	go func() {
		_ = grpcSrv.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcSrv.Stop()
		_ = listener.Close()
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial grpc: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return grpcapi.NewCiwiServiceClient(conn), s
}

func TestGRPCBridgeReadEndpoints(t *testing.T) {
	client, s := newGRPCTestClient(t)
	s.mu.Lock()
	s.agents["agent-1"] = agentState{
		Hostname:     "host-a",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v0.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		LastSeenUTC:  time.Now().UTC(),
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	serverInfo, err := client.GetServerInfo(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetServerInfo: %v", err)
	}
	if got := serverInfo.GetName(); got != "ciwi" {
		t.Fatalf("expected name=ciwi, got %v", got)
	}

	agents, err := client.ListAgents(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents.GetAgents()) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents.GetAgents()))
	}

	agentDetail, err := client.GetAgent(ctx, &grpcapi.GetAgentRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if agentDetail.GetAgent().GetAgentId() != "agent-1" {
		t.Fatalf("unexpected agent id: %q", agentDetail.GetAgent().GetAgentId())
	}

	jobs, err := client.ListJobs(ctx, &grpcapi.ListJobsRequest{View: "summary", Max: 50})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if got := jobs.GetView(); got != "summary" {
		t.Fatalf("expected summary view, got %v", got)
	}
}

func TestGRPCBridgeCommandEndpoints(t *testing.T) {
	client, s := newGRPCTestClient(t)
	oldVersion := version.Version
	version.Version = "v9.9.9"
	t.Cleanup(func() { version.Version = oldVersion })

	s.mu.Lock()
	s.agents["agent-2"] = agentState{
		Hostname:     "host-b",
		OS:           "windows",
		Arch:         "amd64",
		Version:      "v0.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "cmd,powershell"},
		LastSeenUTC:  time.Now().UTC(),
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	toolsResp, err := client.RequestAgentToolsRefresh(ctx, &grpcapi.AgentRequest{AgentId: "agent-2"})
	if err != nil {
		t.Fatalf("RequestAgentToolsRefresh: %v", err)
	}
	if got := toolsResp.GetRequested(); !got {
		t.Fatalf("expected requested=true, got %#v", toolsResp)
	}

	updateResp, err := client.RequestAgentUpdate(ctx, &grpcapi.AgentRequest{AgentId: "agent-2"})
	if err != nil {
		t.Fatalf("RequestAgentUpdate: %v", err)
	}
	if got := updateResp.GetRequested(); !got {
		t.Fatalf("expected requested=true, got %#v", updateResp)
	}

	runResp, err := client.RunAdhocScript(ctx, &grpcapi.RunAdhocScriptRequest{
		AgentId:        "agent-2",
		Shell:          "cmd",
		Script:         "echo hi",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("RunAdhocScript: %v", err)
	}
	if !runResp.GetQueued() {
		t.Fatalf("expected queued=true, got %#v", runResp)
	}
	if runResp.GetJobId() == "" {
		t.Fatalf("expected job_id in response, got %#v", runResp)
	}

	_, err = client.RunPipeline(ctx, &grpcapi.RunPipelineRequest{DryRun: true})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got err=%v code=%s", err, status.Code(err))
	}
}

func TestGRPCBridgeWatchStateStreams(t *testing.T) {
	client, s := newGRPCTestClient(t)
	s.mu.Lock()
	s.agents["agent-stream"] = agentState{
		Hostname:     "host-stream",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v0.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		LastSeenUTC:  time.Now().UTC(),
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.WatchState(ctx, &grpcapi.WatchStateRequest{
		IntervalMs:         250,
		IncludeAgents:      true,
		IncludeJobsSummary: true,
	})
	if err != nil {
		t.Fatalf("WatchState start: %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("WatchState first recv: %v", err)
	}
	if first.GetSeq() != 1 {
		t.Fatalf("expected first seq=1, got %d", first.GetSeq())
	}
	if first.GetStreamId() == "" {
		t.Fatalf("expected stream_id in first event")
	}
	if first.GetServerInfo() == nil || first.GetServerInfo().GetName() != "ciwi" {
		t.Fatalf("expected server info in first event, got %#v", first.GetServerInfo())
	}
	if first.GetAgents() == nil || len(first.GetAgents().GetAgents()) != 1 {
		t.Fatalf("expected one agent in first event, got %#v", first.GetAgents())
	}
	if first.GetJobsSummary() == nil || first.GetJobsSummary().GetView() != "summary" {
		t.Fatalf("expected jobs summary view in first event, got %#v", first.GetJobsSummary())
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("WatchState second recv: %v", err)
	}
	if second.GetSeq() <= first.GetSeq() {
		t.Fatalf("expected second seq > first seq, got %d then %d", first.GetSeq(), second.GetSeq())
	}
	if second.GetStreamId() != first.GetStreamId() {
		t.Fatalf("expected same stream_id across events, got %q then %q", first.GetStreamId(), second.GetStreamId())
	}
}
