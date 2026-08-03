package nativecnp_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/adapters/nativecnp"
	"github.com/izzyreal/ciwi/internal/adapters/nativequic"
	"github.com/izzyreal/ciwi/internal/adapters/nativetcp"
	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

func TestClientServerVerticalSlice(t *testing.T) {
	changes := application.NewChangeHub()
	pipelines := &pipelineService{}
	executions := &executionCommandService{}
	server := startServer(t, nativequic.Services{
		Server:            serverService{},
		Projects:          projectService{},
		ProjectCommands:   projectService{},
		Updates:           updateService{},
		FrontPage:         frontPageService{},
		ProjectDetails:    projectDetailsService{},
		JobDetails:        jobDetailsService{},
		Pipelines:         pipelines,
		PipelineChains:    pipelines,
		RunOptions:        pipelines,
		Agents:            agentService{},
		AgentCommands:     agentService{},
		ExecutionCommands: executions,
		ExecutionControls: executions,
		Changes:           changes,
		Version:           "v0.2.0",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := cnpclient.Dial(ctx, server.Addr(), "ciwi-test", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if got := client.Welcome(); got.ServerVersion != "v0.2.0" || got.ServerInstanceId == "" {
		t.Fatalf("welcome = %#v", got)
	}
	info, err := client.GetServerInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "ciwi" || info.ApiVersion != 1 || info.Hostname != "buildbox" {
		t.Fatalf("server info = %#v", info)
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects.Projects) != 1 || projects.Projects[0].Name != "ciwi" || len(projects.Projects[0].Pipelines) != 1 {
		t.Fatalf("projects = %#v", projects.Projects)
	}
	frontPage, err := client.GetFrontPageView(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if frontPage.Server.Version != "v0.2.0" || len(frontPage.Projects) != 1 || len(frontPage.QueuedExecutions) != 1 || frontPage.Projects[0].PipelineChains[0].SequenceLabel != "build → release" {
		t.Fatalf("front page = %#v", frontPage)
	}
	projectDetails, err := client.GetProjectDetails(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if projectDetails.Project.Name != "ciwi" || len(projectDetails.Pipelines) != 1 || projectDetails.Pipelines[0].Jobs[0].Steps[0].Name != "Compile" || !projectDetails.Pipelines[0].Jobs[0].SupportsDryRun {
		t.Fatalf("project details = %#v", projectDetails)
	}
	jobDetails, err := client.GetJobDetails(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if jobDetails.Title != "Job: compile" || len(jobDetails.Timeline) != 1 || jobDetails.Timeline[0].Status != "succeeded" || len(jobDetails.OutputGroups) != 1 || jobDetails.OutputGroups[0].ExpandedCommand != "go build ./..." || !jobDetails.CanRerun {
		t.Fatalf("job details = %#v", jobDetails)
	}
	output, outputErrors, err := client.WatchJobOutput(ctx, "job-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	batch := receiveOutput(t, output, outputErrors)
	if !batch.Terminal || batch.NextEventId != 1 || len(batch.Events) != 1 || batch.Events[0].Text != "compiled\n" {
		t.Fatalf("job output = %#v", batch)
	}

	result, err := client.RunPipeline(ctx, &cnpv1.RunPipelineRequest{
		PipelineDbId: 42,
		Selection:    &cnpv1.RunPipelineSelection{PipelineJobId: "linux", DryRun: true},
	}, "stable-command-key")
	if err != nil {
		t.Fatal(err)
	}
	if result.Enqueued != 1 || len(result.JobExecutionIds) != 1 {
		t.Fatalf("run result = %#v", result)
	}
	request := pipelines.lastRequest()
	if request.PipelineDBID != 42 || request.PipelineJobID != "linux" || !request.DryRun || request.IdempotencyKey != "stable-command-key" {
		t.Fatalf("mapped request = %#v", request)
	}
	chainResult, err := client.RunPipelineChain(ctx, &cnpv1.RunPipelineChainRequest{
		ProjectId: 7, ChainId: "build+release", Selection: &cnpv1.RunPipelineSelection{DryRun: true},
	}, "stable-chain-key")
	if err != nil || chainResult.Enqueued != 2 {
		t.Fatalf("chain result = %#v, %v", chainResult, err)
	}
	chainRequest := pipelines.lastChainRequest()
	if chainRequest.ProjectID != 7 || chainRequest.ChainID != "build+release" || !chainRequest.DryRun || chainRequest.IdempotencyKey != "stable-chain-key" {
		t.Fatalf("mapped chain request = %#v", chainRequest)
	}
	runOptions, err := client.GetRunOptions(ctx, &cnpv1.GetRunOptionsRequest{
		PipelineDbId: 42, Selection: &cnpv1.RunPipelineSelection{SourceRef: "refs/heads/main"},
	})
	if err != nil || runOptions.TargetLabel != "build" || len(runOptions.EligibleAgents) != 2 || runOptions.SelectedSourceRef != "refs/heads/main" {
		t.Fatalf("run options = %#v, %v", runOptions, err)
	}
	agents, err := client.GetAgentsView(ctx)
	if err != nil || agents.Summary != "1/1 online" || len(agents.Agents) != 1 {
		t.Fatalf("agents = %#v, %v", agents, err)
	}
	agentResult, err := client.AgentAction(ctx, &cnpv1.AgentActionRequest{AgentId: "agent-1", Action: "restart"}, "agent-command-key")
	if err != nil || !agentResult.Requested || agentResult.AgentId != "agent-1" {
		t.Fatalf("agent action = %#v, %v", agentResult, err)
	}
	projectResult, err := client.ProjectAction(ctx, 7, application.ProjectActionReload, "project-command-key")
	if err != nil || projectResult.ProjectId != 7 {
		t.Fatalf("project action = %#v, %v", projectResult, err)
	}
	importResult, err := client.ImportProject(ctx, &cnpv1.ImportProjectRequest{RepoUrl: "https://example.test/repo.git", ConfigFile: "ciwi-project.yaml"}, "import-command-key")
	if err != nil || importResult.ProjectName != "imported" || importResult.Pipelines != 1 {
		t.Fatalf("project import = %#v, %v", importResult, err)
	}
	updateStatus, err := client.GetServerUpdateStatus(ctx)
	if err != nil || updateStatus.CurrentVersion != "v0.2.0" || !updateStatus.SelfUpdateSupported {
		t.Fatalf("update status = %#v, %v", updateStatus, err)
	}
	updateCheck, err := client.CheckServerUpdates(ctx)
	if err != nil || len(updateCheck.AvailableVersions) != 1 || updateCheck.AvailableVersions[0] != "v0.2.1" {
		t.Fatalf("update check = %#v, %v", updateCheck, err)
	}
	versions, err := client.ListServerUpdateVersions(ctx)
	if err != nil || len(versions.Versions) != 1 || versions.Versions[0] != "v0.1.9" {
		t.Fatalf("update versions = %#v, %v", versions, err)
	}
	updateAction, err := client.ServerUpdateAction(ctx, application.ServerUpdateActionRestart, "")
	if err != nil || !updateAction.Restarting {
		t.Fatalf("update action = %#v, %v", updateAction, err)
	}
	cleared, err := client.ClearExecutionQueue(ctx, "clear-command-key")
	if err != nil || cleared.Cleared != 2 {
		t.Fatalf("clear queue = %#v, %v", cleared, err)
	}
	flushed, err := client.FlushExecutionHistory(ctx, &cnpv1.FlushExecutionHistoryRequest{JobExecutionIds: []string{"job-1", "job-2"}}, "flush-command-key")
	if err != nil || flushed.Flushed != 2 {
		t.Fatalf("flush history = %#v, %v", flushed, err)
	}
	if executions.clearRequest.IdempotencyKey != "clear-command-key" || executions.flushRequest.IdempotencyKey != "flush-command-key" || !reflect.DeepEqual(executions.flushRequest.JobExecutionIDs, []string{"job-1", "job-2"}) {
		t.Fatalf("execution command mapping = clear %#v, flush %#v", executions.clearRequest, executions.flushRequest)
	}
	cancelled, err := client.CancelExecution(ctx, "job-1", "cancel-command-key")
	if err != nil || cancelled.Status != "failed" {
		t.Fatalf("cancel execution = %#v, %v", cancelled, err)
	}
	rerun, err := client.RerunExecution(ctx, "job-1", "rerun-command-key")
	if err != nil || rerun.JobExecutionId != "job-rerun" {
		t.Fatalf("rerun execution = %#v, %v", rerun, err)
	}
}

func TestTCPClientMultiplexesCallsAndWatches(t *testing.T) {
	changes := application.NewChangeHub()
	services := completeTestServices(changes)
	server := startTCPServer(t, services)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := cnpclient.Dial(ctx, "tcp://"+server.Addr(), "ciwi-test", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	watchCtx, cancelWatch := context.WithCancel(ctx)
	events, watchErrors, err := client.WatchChanges(watchCtx)
	if err != nil {
		t.Fatal(err)
	}
	initial := receiveEvent(t, events, watchErrors)
	if !initial.ResyncRequired {
		t.Fatalf("initial TCP change = %#v", initial)
	}

	const calls = 24
	errorsOut := make(chan error, calls)
	var callsDone sync.WaitGroup
	for range calls {
		callsDone.Add(1)
		go func() {
			defer callsDone.Done()
			info, callErr := client.GetServerInfo(ctx)
			if callErr == nil && info.Hostname != "buildbox" {
				callErr = fmt.Errorf("hostname = %q", info.Hostname)
			}
			errorsOut <- callErr
		}()
	}
	callsDone.Wait()
	close(errorsOut)
	for callErr := range errorsOut {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}

	changes.Publish(application.ChangeProjects)
	change := receiveEvent(t, events, watchErrors)
	if len(change.Topics) != 1 || change.Topics[0] != cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS {
		t.Fatalf("TCP change topics = %v", change.Topics)
	}
	cancelWatch()
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("cancelled TCP watch did not close")
	}
	if _, err := client.GetServerInfo(ctx); err != nil {
		t.Fatalf("TCP session was lost after cancelling one stream: %v", err)
	}
}

func TestTCPAndQUICListenersShareOneNumericPort(t *testing.T) {
	services := completeTestServices(application.NewChangeHub())
	handler, err := nativecnp.NewHandler(services)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := nativecnp.ServerTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	tcpServer, err := nativetcp.ListenWithHandler("127.0.0.1:0", handler, tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	quicServer, err := nativequic.ListenWithHandler(tcpServer.Addr(), handler, tlsConfig)
	if err != nil {
		_ = tcpServer.Close()
		t.Fatalf("QUIC could not share TCP numeric port %s: %v", tcpServer.Addr(), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	tcpDone := make(chan error, 1)
	quicDone := make(chan error, 1)
	go func() { tcpDone <- tcpServer.Serve(ctx) }()
	go func() { quicDone <- quicServer.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = tcpServer.Close()
		_ = quicServer.Close()
		for name, done := range map[string]<-chan error{"TCP": tcpDone, "QUIC": quicDone} {
			select {
			case serveErr := <-done:
				if serveErr != nil {
					t.Errorf("%s Serve() error = %v", name, serveErr)
				}
			case <-time.After(2 * time.Second):
				t.Errorf("%s server did not stop", name)
			}
		}
	})

	dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
	defer cancelDial()
	for _, target := range []string{"quic://" + quicServer.Addr(), "tcp://" + tcpServer.Addr()} {
		client, dialErr := cnpclient.Dial(dialCtx, target, "ciwi-test", "test")
		if dialErr != nil {
			t.Fatalf("dial %s: %v", target, dialErr)
		}
		info, infoErr := client.GetServerInfo(dialCtx)
		_ = client.Close()
		if infoErr != nil || info.Hostname != "buildbox" {
			t.Fatalf("server info through %s = %#v, %v", target, info, infoErr)
		}
	}
}

func TestListenRejectsIncompleteServiceSetBeforeBinding(t *testing.T) {
	if _, err := nativequic.Listen("127.0.0.1:0", nativequic.Services{}); err == nil {
		t.Fatal("Listen() accepted an incomplete service set")
	}
}

func TestWatchChangesStartsWithResyncAndStreamsInvalidations(t *testing.T) {
	changes := application.NewChangeHub()
	server := startServer(t, nativequic.Services{
		Server: serverService{}, Projects: projectService{}, ProjectCommands: projectService{}, Updates: updateService{}, FrontPage: frontPageService{}, ProjectDetails: projectDetailsService{}, JobDetails: jobDetailsService{},
		Pipelines: &pipelineService{}, PipelineChains: &pipelineService{}, RunOptions: &pipelineService{}, Agents: agentService{}, AgentCommands: agentService{}, ExecutionCommands: &executionCommandService{}, ExecutionControls: &executionCommandService{}, Changes: changes, Version: "v0.2.0",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := cnpclient.Dial(ctx, server.Addr(), "ciwi-test", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	events, errorsOut, err := client.WatchChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initial := receiveEvent(t, events, errorsOut)
	if !initial.ResyncRequired || initial.ServerInstanceId == "" {
		t.Fatalf("initial event = %#v", initial)
	}
	changes.Publish(application.ChangeProjects, application.ChangeQueue)
	change := receiveEvent(t, events, errorsOut)
	if change.ResyncRequired || change.Revision <= initial.Revision {
		t.Fatalf("change event = %#v", change)
	}
	if len(change.Topics) != 2 || change.Topics[0] != cnpv1.ChangeTopic_CHANGE_TOPIC_PROJECTS || change.Topics[1] != cnpv1.ChangeTopic_CHANGE_TOPIC_QUEUE {
		t.Fatalf("change topics = %v", change.Topics)
	}
}

func TestWatchJobOutputStreamsAfterExecutionInvalidation(t *testing.T) {
	changes := application.NewChangeHub()
	jobDetails := &streamingJobDetailsService{}
	server := startServer(t, nativequic.Services{
		Server: serverService{}, Projects: projectService{}, ProjectCommands: projectService{}, Updates: updateService{}, FrontPage: frontPageService{}, ProjectDetails: projectDetailsService{}, JobDetails: jobDetails,
		Pipelines: &pipelineService{}, PipelineChains: &pipelineService{}, RunOptions: &pipelineService{}, Agents: agentService{}, AgentCommands: agentService{}, ExecutionCommands: &executionCommandService{}, ExecutionControls: &executionCommandService{}, Changes: changes, Version: "v0.2.0",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := cnpclient.Dial(ctx, server.Addr(), "ciwi-test", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	batches, errorsOut, err := client.WatchJobOutput(ctx, "job-running", 0)
	if err != nil {
		t.Fatal(err)
	}
	initial := receiveOutput(t, batches, errorsOut)
	if initial.Terminal || initial.NextEventId != 0 || len(initial.Events) != 0 {
		t.Fatalf("initial output = %#v", initial)
	}
	jobDetails.setReady()
	changes.Publish(application.ChangeQueue)
	next := receiveOutput(t, batches, errorsOut)
	if !next.Terminal || next.NextEventId != 2 || len(next.Events) != 1 || next.Events[0].Text != "next\n" {
		t.Fatalf("next output = %#v", next)
	}
}

func TestTypedApplicationErrorCrossesProtocol(t *testing.T) {
	server := startServer(t, nativequic.Services{
		Server: serverService{}, Projects: projectService{}, ProjectCommands: projectService{}, Updates: updateService{}, FrontPage: frontPageService{}, ProjectDetails: projectDetailsService{}, JobDetails: jobDetailsService{},
		Pipelines: failingPipelineService{}, PipelineChains: &pipelineService{}, RunOptions: &pipelineService{}, Agents: agentService{}, AgentCommands: agentService{}, ExecutionCommands: &executionCommandService{}, ExecutionControls: &executionCommandService{}, Changes: application.NewChangeHub(), Version: "v0.2.0",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := cnpclient.Dial(ctx, server.Addr(), "ciwi-test", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.RunPipeline(ctx, &cnpv1.RunPipelineRequest{PipelineDbId: 99}, "error-key")
	var protocolError *cnpclient.Error
	if !errors.As(err, &protocolError) || protocolError.Code != cnpv1.StatusCode_STATUS_CODE_NOT_FOUND {
		t.Fatalf("RunPipeline() error = %#v", err)
	}
}

func startServer(t *testing.T, services nativequic.Services) *nativequic.Server {
	t.Helper()
	server, err := nativequic.Listen("127.0.0.1:0", services)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("native server did not stop")
		}
	})
	return server
}

func startTCPServer(t *testing.T, services nativecnp.Services) *nativetcp.Server {
	t.Helper()
	handler, err := nativecnp.NewHandler(services)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := nativecnp.ServerTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	server, err := nativetcp.ListenWithHandler("127.0.0.1:0", handler, tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("native TCP server did not stop")
		}
	})
	return server
}

func completeTestServices(changes *application.ChangeHub) nativecnp.Services {
	pipelines := &pipelineService{}
	executions := &executionCommandService{}
	return nativecnp.Services{
		Server: serverService{}, Projects: projectService{}, ProjectCommands: projectService{}, Updates: updateService{},
		FrontPage: frontPageService{}, ProjectDetails: projectDetailsService{}, JobDetails: jobDetailsService{},
		Pipelines: pipelines, PipelineChains: pipelines, RunOptions: pipelines,
		Agents: agentService{}, AgentCommands: agentService{}, ExecutionCommands: executions,
		ExecutionControls: executions, Changes: changes, Version: "v0.2.0",
	}
}

func receiveEvent(t *testing.T, events <-chan *cnpv1.ChangeEvent, errorsOut <-chan error) *cnpv1.ChangeEvent {
	t.Helper()
	select {
	case event := <-events:
		if event == nil {
			t.Fatal("watch stream closed")
		}
		return event
	case err := <-errorsOut:
		t.Fatalf("watch error = %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for change event")
	}
	return nil
}

func receiveOutput(t *testing.T, batches <-chan *cnpv1.JobOutputBatch, errorsOut <-chan error) *cnpv1.JobOutputBatch {
	t.Helper()
	select {
	case batch := <-batches:
		if batch == nil {
			t.Fatal("job output stream closed")
		}
		return batch
	case err := <-errorsOut:
		t.Fatalf("job output error = %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job output")
	}
	return nil
}

type serverService struct{}

func (serverService) GetServerInfo(context.Context) (domain.ServerInfo, error) {
	return domain.ServerInfo{Name: "ciwi", APIVersion: 1, Version: "v0.2.0", Hostname: "buildbox"}, nil
}

type projectService struct{}

type updateService struct{}

func (updateService) Status(context.Context) (application.ServerUpdateStatus, error) {
	return application.ServerUpdateStatus{CurrentVersion: "v0.2.0", SelfUpdateSupported: true}, nil
}

func (updateService) Check(context.Context) (application.ServerUpdateCheckResult, error) {
	return application.ServerUpdateCheckResult{CurrentVersion: "v0.2.0", LatestVersion: "v0.2.1", AvailableVersions: []string{"v0.2.1"}, UpdateAvailable: true}, nil
}

func (updateService) Versions(context.Context) (application.ServerUpdateVersions, error) {
	return application.ServerUpdateVersions{CurrentVersion: "v0.2.0", Versions: []string{"v0.1.9"}}, nil
}

func (updateService) Execute(_ context.Context, request application.ServerUpdateActionRequest) (application.ServerUpdateActionResult, error) {
	return application.ServerUpdateActionResult{Restarting: request.Action == application.ServerUpdateActionRestart, Message: "accepted"}, nil
}

func (projectService) ListProjects(context.Context) ([]domain.Project, error) {
	return testProjects(), nil
}

func (projectService) Execute(_ context.Context, request application.ProjectActionRequest) (application.ProjectActionResult, error) {
	return application.ProjectActionResult{ProjectID: request.ProjectID, Message: request.Action + " complete"}, nil
}

func (projectService) Import(_ context.Context, request application.ImportProjectRequest) (application.ImportProjectResult, error) {
	return application.ImportProjectResult{ProjectName: "imported", RepoURL: request.RepoURL, ConfigFile: request.ConfigFile, Pipelines: 1}, nil
}

type frontPageService struct{}

type agentService struct{}

func (agentService) GetAgentsView(context.Context) (presentation.AgentsView, error) {
	return presentation.AgentsView{Summary: "1/1 online", Agents: []presentation.AgentView{{ID: "agent-1", Status: "online", StatusLabel: "Online"}}}, nil
}

func (agentService) Execute(_ context.Context, request application.AgentActionRequest) (application.AgentActionResult, error) {
	return application.AgentActionResult{Requested: true, AgentID: request.AgentID, Message: request.Action + " requested"}, nil
}

func (frontPageService) GetFrontPageView(context.Context) (presentation.FrontPageView, error) {
	return presentation.FrontPageView{
		Server:   domain.ServerInfo{Name: "ciwi", APIVersion: 1, Version: "v0.2.0", Hostname: "buildbox"},
		Projects: testProjects(),
		QueuedExecutions: []domain.ExecutionCard{{
			Key: "pipeline:run-1", Kind: "pipeline", Title: "ciwi build",
			JobExecutionIDs: []string{"job-1"}, Summary: domain.ExecutionSummary{TotalJobs: 1, InProgress: 1},
		}},
	}, nil
}

type projectDetailsService struct{}

func (projectDetailsService) GetProjectDetailsView(context.Context, int64) (presentation.ProjectDetailsView, error) {
	return presentation.ProjectDetailsView{
		Project: testProjects()[0],
		Pipelines: []presentation.ProjectPipelineView{{
			ID: 42, PipelineID: "build", JobsCount: 1, SupportsDryRun: true,
			Jobs: []presentation.ProjectJobView{{
				ID: "compile", StepsCount: 1, SupportsDryRun: true,
				Steps: []presentation.ProjectStepView{{Index: 0, Position: 1, Name: "Compile", Type: "run"}},
			}},
		}},
	}, nil
}

type jobDetailsService struct{}

func (jobDetailsService) GetJobDetailsView(context.Context, string) (presentation.JobDetailsView, error) {
	return presentation.JobDetailsView{
		ID: "job-1", Title: "Job: compile", Status: "succeeded", StatusLabel: "Succeeded",
		CanRerun:     true,
		Timeline:     []presentation.JobTimelineView{{ID: "step:1", Kind: "step", Title: "Job step 1/1: Compile", Status: "succeeded", StatusLabel: "Succeeded"}},
		OutputGroups: []presentation.JobOutputGroupView{{ID: "step:1", StateKey: "job-output:job-1:step:1", Kind: "step", Title: "Job step 1/1: Compile", Status: "succeeded", StatusLabel: "Succeeded", Reached: true, YAMLLiteral: "run: go build ./...", ExpandedCommand: "go build ./..."}},
	}, nil
}

type streamingJobDetailsService struct {
	mu    sync.Mutex
	ready bool
}

func (s *streamingJobDetailsService) setReady() {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
}

func (s *streamingJobDetailsService) GetJobDetailsView(context.Context, string) (presentation.JobDetailsView, error) {
	return presentation.JobDetailsView{ID: "job-running", Status: "running"}, nil
}

func (s *streamingJobDetailsService) GetJobOutputView(context.Context, string, int64) (presentation.JobOutputView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return presentation.JobOutputView{JobExecutionID: "job-running"}, nil
	}
	return presentation.JobOutputView{
		JobExecutionID: "job-running", NextEventID: 2, Terminal: true,
		Events: []presentation.JobOutputEventView{{EventID: 2, Type: "output", ItemID: "step:1", Text: "next\n"}},
	}, nil
}

func (jobDetailsService) GetJobOutputView(context.Context, string, int64) (presentation.JobOutputView, error) {
	return presentation.JobOutputView{
		JobExecutionID: "job-1", NextEventID: 1, Terminal: true,
		Events: []presentation.JobOutputEventView{{EventID: 1, Type: "output", ItemID: "step:1", Text: "compiled\n"}},
	}, nil
}

func testProjects() []domain.Project {
	return []domain.Project{{
		ID: 7, Name: "ciwi", RepoURL: "https://github.com/izzyreal/ciwi",
		Pipelines:      []domain.Pipeline{{ID: 42, PipelineID: "build", SupportsDryRun: true}},
		PipelineChains: []domain.PipelineChain{{ID: "build+release", Name: "Build and release", Pipelines: []string{"build", "release"}, SupportsDryRun: true}},
	}}
}

type pipelineService struct {
	mu           sync.Mutex
	request      application.RunPipelineRequest
	chainRequest application.RunPipelineChainRequest
}

func (s *pipelineService) RunPipelineChain(_ context.Context, request application.RunPipelineChainRequest) (application.RunPipelineChainResult, error) {
	s.mu.Lock()
	s.chainRequest = request
	s.mu.Unlock()
	return application.RunPipelineChainResult{ProjectName: "ciwi", ChainID: request.ChainID, ChainName: "Build and release", Enqueued: 2, JobExecutionIDs: []string{"job-1", "job-2"}}, nil
}

func (s *pipelineService) RunPipeline(_ context.Context, request application.RunPipelineRequest) (application.RunPipelineResult, error) {
	s.mu.Lock()
	s.request = request
	s.mu.Unlock()
	return application.RunPipelineResult{ProjectName: "ciwi", PipelineID: "build", Enqueued: 1, JobExecutionIDs: []string{"job-1"}}, nil
}

func (s *pipelineService) GetRunOptions(_ context.Context, request application.RunOptionsRequest) (application.RunOptions, error) {
	return application.RunOptions{
		TargetKind: application.RunTargetPipeline, TargetLabel: "build", PipelineDBID: request.PipelineDBID,
		ProjectID: 7, SupportsDryRun: true, SourceRepo: "https://github.com/izzyreal/ciwi",
		DefaultSourceRef:  request.SourceRef,
		SelectedSourceRef: request.SourceRef,
		SourceRefs:        []application.RunOption{{Value: "refs/heads/main", Label: "main"}},
		EligibleAgents:    []application.RunOption{{Value: "", Label: "Any eligible agent"}, {Value: "agent-1", Label: "agent-1"}},
		PendingJobs:       1,
	}, nil
}

func (s *pipelineService) lastRequest() application.RunPipelineRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.request
}

func (s *pipelineService) lastChainRequest() application.RunPipelineChainRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chainRequest
}

type failingPipelineService struct{}

func (failingPipelineService) RunPipeline(context.Context, application.RunPipelineRequest) (application.RunPipelineResult, error) {
	return application.RunPipelineResult{}, application.NewError(application.ErrorNotFound, "pipeline not found", nil)
}

type executionCommandService struct {
	clearRequest application.ClearExecutionQueueRequest
	flushRequest application.FlushExecutionHistoryRequest
}

func (s *executionCommandService) ClearQueue(_ context.Context, request application.ClearExecutionQueueRequest) (application.ClearExecutionQueueResult, error) {
	s.clearRequest = request
	return application.ClearExecutionQueueResult{Cleared: 2}, nil
}

func (s *executionCommandService) FlushHistory(_ context.Context, request application.FlushExecutionHistoryRequest) (application.FlushExecutionHistoryResult, error) {
	s.flushRequest = request
	return application.FlushExecutionHistoryResult{Flushed: 2}, nil
}

func (s *executionCommandService) Cancel(_ context.Context, request application.ExecutionControlRequest) (application.CancelExecutionResult, error) {
	return application.CancelExecutionResult{JobExecutionID: request.JobExecutionID, Status: "failed"}, nil
}

func (s *executionCommandService) Rerun(_ context.Context, request application.ExecutionControlRequest) (application.RerunExecutionResult, error) {
	return application.RerunExecutionResult{OriginalJobExecutionID: request.JobExecutionID, JobExecutionID: "job-rerun", Status: "queued"}, nil
}
