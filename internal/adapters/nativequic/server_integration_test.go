package nativequic_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/adapters/nativequic"
	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

func TestClientServerVerticalSlice(t *testing.T) {
	changes := application.NewChangeHub()
	pipelines := &pipelineService{}
	server := startServer(t, nativequic.Services{
		Server:         serverService{},
		Projects:       projectService{},
		FrontPage:      frontPageService{},
		ProjectDetails: projectDetailsService{},
		JobDetails:     jobDetailsService{},
		Pipelines:      pipelines,
		Changes:        changes,
		Version:        "v0.2.0",
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
	if frontPage.Server.Version != "v0.2.0" || len(frontPage.Projects) != 1 || len(frontPage.QueuedExecutions) != 1 {
		t.Fatalf("front page = %#v", frontPage)
	}
	projectDetails, err := client.GetProjectDetails(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if projectDetails.Project.Name != "ciwi" || len(projectDetails.Pipelines) != 1 || projectDetails.Pipelines[0].Jobs[0].Steps[0].Name != "Compile" {
		t.Fatalf("project details = %#v", projectDetails)
	}
	jobDetails, err := client.GetJobDetails(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if jobDetails.Title != "Job: compile" || len(jobDetails.Timeline) != 1 || jobDetails.Timeline[0].Status != "succeeded" {
		t.Fatalf("job details = %#v", jobDetails)
	}
	output, outputErrors, err := client.WatchJobOutput(ctx, "job-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	batch := receiveOutput(t, output, outputErrors)
	if !batch.Terminal || batch.NextEventId != 1 || len(batch.Lines) != 1 || batch.Lines[0].Text != "compiled\n" {
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
}

func TestListenRejectsIncompleteServiceSetBeforeBinding(t *testing.T) {
	if _, err := nativequic.Listen("127.0.0.1:0", nativequic.Services{}); err == nil {
		t.Fatal("Listen() accepted an incomplete service set")
	}
}

func TestWatchChangesStartsWithResyncAndStreamsInvalidations(t *testing.T) {
	changes := application.NewChangeHub()
	server := startServer(t, nativequic.Services{
		Server: serverService{}, Projects: projectService{}, FrontPage: frontPageService{}, ProjectDetails: projectDetailsService{}, JobDetails: jobDetailsService{},
		Pipelines: &pipelineService{}, Changes: changes, Version: "v0.2.0",
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
		Server: serverService{}, Projects: projectService{}, FrontPage: frontPageService{}, ProjectDetails: projectDetailsService{}, JobDetails: jobDetails,
		Pipelines: &pipelineService{}, Changes: changes, Version: "v0.2.0",
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
	if initial.Terminal || initial.NextEventId != 0 || len(initial.Lines) != 0 {
		t.Fatalf("initial output = %#v", initial)
	}
	jobDetails.setReady()
	changes.Publish(application.ChangeQueue)
	next := receiveOutput(t, batches, errorsOut)
	if !next.Terminal || next.NextEventId != 2 || len(next.Lines) != 1 || next.Lines[0].Text != "next\n" {
		t.Fatalf("next output = %#v", next)
	}
}

func TestTypedApplicationErrorCrossesProtocol(t *testing.T) {
	server := startServer(t, nativequic.Services{
		Server: serverService{}, Projects: projectService{}, FrontPage: frontPageService{}, ProjectDetails: projectDetailsService{}, JobDetails: jobDetailsService{},
		Pipelines: failingPipelineService{}, Changes: application.NewChangeHub(), Version: "v0.2.0",
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

func (projectService) ListProjects(context.Context) ([]domain.Project, error) {
	return testProjects(), nil
}

type frontPageService struct{}

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
				ID: "compile", StepsCount: 1,
				Steps: []presentation.ProjectStepView{{Index: 0, Position: 1, Name: "Compile", Type: "run"}},
			}},
		}},
	}, nil
}

type jobDetailsService struct{}

func (jobDetailsService) GetJobDetailsView(context.Context, string) (presentation.JobDetailsView, error) {
	return presentation.JobDetailsView{
		ID: "job-1", Title: "Job: compile", Status: "succeeded", StatusLabel: "Succeeded",
		Timeline: []presentation.JobTimelineView{{ID: "step:1", Kind: "step", Title: "Job step 1/1: Compile", Status: "succeeded", StatusLabel: "Succeeded"}},
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
		Lines: []presentation.JobOutputLineView{{EventID: 2, Text: "next\n"}},
	}, nil
}

func (jobDetailsService) GetJobOutputView(context.Context, string, int64) (presentation.JobOutputView, error) {
	return presentation.JobOutputView{
		JobExecutionID: "job-1", NextEventID: 1, Terminal: true,
		Lines: []presentation.JobOutputLineView{{EventID: 1, Text: "compiled\n"}},
	}, nil
}

func testProjects() []domain.Project {
	return []domain.Project{{
		ID: 7, Name: "ciwi", RepoURL: "https://github.com/izzyreal/ciwi",
		Pipelines:      []domain.Pipeline{{ID: 42, PipelineID: "build", SupportsDryRun: true}},
		PipelineChains: []domain.PipelineChain{{ID: "build+release", Name: "Build and release", Pipelines: []string{"build", "release"}}},
	}}
}

type pipelineService struct {
	mu      sync.Mutex
	request application.RunPipelineRequest
}

func (s *pipelineService) RunPipeline(_ context.Context, request application.RunPipelineRequest) (application.RunPipelineResult, error) {
	s.mu.Lock()
	s.request = request
	s.mu.Unlock()
	return application.RunPipelineResult{ProjectName: "ciwi", PipelineID: "build", Enqueued: 1, JobExecutionIDs: []string{"job-1"}}, nil
}

func (s *pipelineService) lastRequest() application.RunPipelineRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.request
}

type failingPipelineService struct{}

func (failingPipelineService) RunPipeline(context.Context, application.RunPipelineRequest) (application.RunPipelineResult, error) {
	return application.RunPipelineResult{}, application.NewError(application.ErrorNotFound, "pipeline not found", nil)
}
