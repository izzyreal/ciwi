package executionviews

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/requirements"
)

type schedulingSourceStub struct{ agents []requirements.AgentSnapshot }

func (s schedulingSourceStub) ListSchedulingAgents(context.Context) ([]requirements.AgentSnapshot, error) {
	return append([]requirements.AgentSnapshot(nil), s.agents...), nil
}

type executionStoreStub struct {
	jobs       []protocol.JobExecution
	events     map[string][]protocol.JobExecutionEvent
	artifacts  map[string][]protocol.JobExecutionArtifact
	testReport map[string]protocol.JobExecutionTestReport
}

func (s executionStoreStub) GetJobExecution(id string) (protocol.JobExecution, error) {
	for _, job := range s.jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return protocol.JobExecution{}, domain.ErrJobExecutionNotFound
}

func (s executionStoreStub) ListJobExecutionTimelineEvents(id string) ([]protocol.JobExecutionEvent, error) {
	return append([]protocol.JobExecutionEvent(nil), s.events[id]...), nil
}

func (s executionStoreStub) ListJobExecutionEventsPageAfter(id string, after int64, limit int) ([]protocol.JobExecutionEvent, error) {
	events := s.events[id]
	out := make([]protocol.JobExecutionEvent, 0, min(len(events), limit))
	for _, event := range events {
		if event.ID > after && len(out) < limit {
			out = append(out, event)
		}
	}
	return out, nil
}

func (s executionStoreStub) ListJobExecutions() ([]protocol.JobExecution, error) {
	return append([]protocol.JobExecution(nil), s.jobs...), nil
}

func (s executionStoreStub) ListJobExecutionArtifacts(id string) ([]protocol.JobExecutionArtifact, error) {
	return append([]protocol.JobExecutionArtifact(nil), s.artifacts[id]...), nil
}

func (s executionStoreStub) GetJobExecutionTestReport(id string) (protocol.JobExecutionTestReport, bool, error) {
	report, found := s.testReport[id]
	return report, found, nil
}

func TestRepositoryPagesJobOutputAndAdvancesOverLifecycleEvents(t *testing.T) {
	repository := NewRepository(executionStoreStub{
		jobs: []protocol.JobExecution{{ID: "job-1", Status: "running"}},
		events: map[string][]protocol.JobExecutionEvent{"job-1": {
			{ID: 4, Type: protocol.JobExecutionEventTypeStepStarted, Step: &protocol.JobStepPlanItem{Index: 1}},
			{ID: 5, Type: protocol.JobExecutionEventTypeStepOutput, Step: &protocol.JobStepPlanItem{Index: 1}, Output: "hello"},
		}},
	}, 40)
	batch, err := repository.ListJobOutputAfter(t.Context(), "job-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if batch.NextEventID != 5 || batch.Terminal || len(batch.Events) != 2 || batch.Events[1].Type != domain.JobOutputEventOutput {
		t.Fatalf("batch = %+v", batch)
	}
}

func TestRepositoryAddsArtifactsAndReportsToJobDetails(t *testing.T) {
	repository := NewRepository(executionStoreStub{
		jobs:      []protocol.JobExecution{{ID: "job-1", Status: "succeeded"}},
		artifacts: map[string][]protocol.JobExecutionArtifact{"job-1": {{Path: "dist/app.zip", SizeBytes: 2048}}},
		testReport: map[string]protocol.JobExecutionTestReport{"job-1": {
			Total: 1, Passed: 1,
			Suites: []protocol.TestSuiteReport{{Name: "unit", Format: "go-json", Total: 1, Passed: 1}},
			Coverage: &protocol.CoverageReport{Format: "go-coverprofile", TotalStatements: 10, CoveredStatements: 8,
				Files: []protocol.CoverageFileReport{{Path: "main.go", TotalStatements: 10, CoveredStatements: 8}}},
		}},
	}, 40)
	details, err := repository.GetJobExecutionDetails(t.Context(), "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Artifacts) != 1 || details.Artifacts[0].Path != "dist/app.zip" {
		t.Fatalf("artifacts = %+v", details.Artifacts)
	}
	if details.TestReport == nil || len(details.TestReport.Suites) != 1 || details.TestReport.Coverage == nil || len(details.TestReport.Coverage.Files) != 1 {
		t.Fatalf("test report = %+v", details.TestReport)
	}
}

func TestRepositoryBoundsOutputPageByPayloadSize(t *testing.T) {
	large := strings.Repeat("x", 300*1024)
	repository := NewRepository(executionStoreStub{
		jobs: []protocol.JobExecution{{ID: "job-1", Status: "running"}},
		events: map[string][]protocol.JobExecutionEvent{"job-1": {
			{ID: 1, Type: protocol.JobExecutionEventTypeStepOutput, Output: large},
			{ID: 2, Type: protocol.JobExecutionEventTypeStepOutput, Output: large},
		}},
	}, 40)
	batch, err := repository.ListJobOutputAfter(t.Context(), "job-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Events) != 1 || batch.NextEventID != 1 || !batch.HasMore {
		t.Fatalf("batch events=%d next=%d hasMore=%v", len(batch.Events), batch.NextEventID, batch.HasMore)
	}
}

func TestRepositoryBuildsTransportNeutralJobTimeline(t *testing.T) {
	now := time.Now().UTC()
	exitCode := 1
	step := protocol.JobStepPlanItem{Index: 1, Total: 2, Name: "Compile", YAMLLiteral: "run: go build ./...", Script: "go build ./..."}
	repository := NewRepository(executionStoreStub{
		jobs: []protocol.JobExecution{{
			ID: "job-1", Status: "failed", CreatedUTC: now, StartedUTC: now.Add(time.Second),
			FinishedUTC: now.Add(3 * time.Second), StepPlan: []protocol.JobStepPlanItem{step, {Index: 2, Total: 2, Name: "Package"}},
			Metadata: map[string]string{"project": "ciwi", "pipeline_id": "build", "pipeline_job_id": "macos", "dry_run": "1"},
		}},
		events: map[string][]protocol.JobExecutionEvent{"job-1": {
			{Type: protocol.JobExecutionEventTypeStepStarted, TimestampUTC: now.Add(time.Second), Step: &step},
			{Type: protocol.JobExecutionEventTypeStepFinished, Step: &step, ExitCode: &exitCode, Error: "compile failed", DurationMS: 1200},
		}},
	}, 40)
	details, err := repository.GetJobExecutionDetails(context.Background(), "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if details.ProjectName != "ciwi" || !details.DryRun || details.Status != "failed" {
		t.Fatalf("details = %+v", details)
	}
	if len(details.Timeline) < 4 || details.Timeline[2].Status != "failed" || details.Timeline[3].Status != "not reached" {
		t.Fatalf("timeline = %+v", details.Timeline)
	}
	if !details.Timeline[2].Reached || details.Timeline[2].StartedUTC.IsZero() || details.Timeline[2].YAMLLiteral != "run: go build ./..." || details.Timeline[2].Command != "go build ./..." || details.Timeline[3].Reached {
		t.Fatalf("timeline details = %+v", details.Timeline)
	}
}

func TestRepositoryUsesEstablishedExecutionGrouping(t *testing.T) {
	now := time.Now().UTC()
	repository := NewRepository(executionStoreStub{jobs: []protocol.JobExecution{
		{ID: "running", Status: "running", CurrentStep: "Compile", CreatedUTC: now, LeasedByAgentID: "agent-linux", Metadata: map[string]string{
			"project": "ciwi", "project_id": "41", "pipeline_id": "build", "pipeline_job_id": "linux", "pipeline_run_id": "run-build", "build_version": "v1", "build_target": "linux-amd64",
		}},
		{ID: "queued", Status: "queued", CreatedUTC: now.Add(-time.Second), Metadata: map[string]string{
			"project": "ciwi", "project_id": "41", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_blocked": "1", "chain_depends_on_pipelines": "package",
		}},
		{ID: "finished", Status: "succeeded", CreatedUTC: now.Add(-2 * time.Second), Metadata: map[string]string{
			"project": "ciwi", "project_id": "41", "pipeline_id": "test", "pipeline_run_id": "run-test",
		}},
	}, testReport: map[string]protocol.JobExecutionTestReport{
		"finished": {Total: 20, Passed: 20},
	}}, 40)
	queued, history, err := repository.ListFrontPageExecutionCards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].Summary.TotalJobs != 2 || queued[0].Summary.InProgress != 1 || queued[0].Summary.Waiting != 1 {
		t.Fatalf("queued cards = %+v", queued)
	}
	if len(queued[0].Sections) != 1 || len(queued[0].Sections[0].Jobs) != 2 {
		t.Fatalf("queued card sections = %+v", queued[0].Sections)
	}
	if got := queued[0].Sections[0].Jobs[0]; got.ID != "running" || got.ProjectID != 41 || got.Label != "linux" || got.CurrentStep != "Compile" || got.PipelineID != "build" || got.BuildLabel != "v1 (linux-amd64)" || got.AgentID != "agent-linux" || got.Action != "cancel" {
		t.Fatalf("queued job = %+v", got)
	}
	if got := queued[0].Sections[0].Jobs[1]; got.Action != "remove" || got.Reason != "Waiting for pipeline package" {
		t.Fatalf("waiting job = %+v", got)
	}
	if len(history) != 1 || history[0].Summary.Succeeded != 1 {
		t.Fatalf("history cards = %+v", history)
	}
	if got := history[0].Sections[0].Jobs[0].TestSummary; got == nil || got.Total != 20 || got.Passed != 20 {
		t.Fatalf("history test summary = %+v", got)
	}
}

func TestRepositoryAddsSchedulingDiagnosisToQueuedViewsAndDetails(t *testing.T) {
	store := executionStoreStub{jobs: []protocol.JobExecution{{
		ID: "windows", Status: protocol.JobExecutionStatusQueued,
		RequiredCapabilities: map[string]string{"os": "windows", "requires.tool.wix": ">=6.0.0"},
	}}}
	repository := NewRepository(store, 40, schedulingSourceStub{agents: []requirements.AgentSnapshot{{
		ID: "linux", OS: "linux", Freshness: "online", Authorized: true,
	}}})
	queued, _, err := repository.ListFrontPageExecutionCards(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	job := queued[0].Sections[0].Jobs[0]
	if job.SchedulingDiagnosis == nil || job.SchedulingDiagnosis.State != requirements.DiagnosisIncompatible {
		t.Fatalf("queued diagnosis = %+v", job.SchedulingDiagnosis)
	}
	details, err := repository.GetJobExecutionDetails(t.Context(), "windows")
	if err != nil {
		t.Fatal(err)
	}
	if details.SchedulingDiagnosis == nil || details.SchedulingDiagnosis.Summary == "" {
		t.Fatalf("details diagnosis = %+v", details.SchedulingDiagnosis)
	}
}

func TestRepositoryCarriesCompletedJobsForActiveCardProgress(t *testing.T) {
	now := time.Now().UTC()
	metadata := map[string]string{
		"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": "run-build",
	}
	repository := NewRepository(executionStoreStub{jobs: []protocol.JobExecution{
		{ID: "done", Status: "succeeded", CreatedUTC: now.Add(-3 * time.Second), StartedUTC: now.Add(-3 * time.Second), FinishedUTC: now.Add(-2 * time.Second), ExpectedDurationMS: 1_000, Metadata: metadata},
		{ID: "active", Status: "running", CreatedUTC: now.Add(-2 * time.Second), StartedUTC: now.Add(-time.Second), ExpectedDurationMS: 4_000, Metadata: metadata},
		{ID: "blocked", Status: "queued", CreatedUTC: now.Add(-time.Second), ExpectedDurationMS: 2_000, Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": "run-build", "chain_blocked": "1",
		}},
	}}, 40)

	queued, _, err := repository.ListFrontPageExecutionCards(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || len(queued[0].ProgressJobs) != 3 {
		t.Fatalf("queued progress jobs = %+v", queued)
	}
	if len(queued[0].Sections) != 1 || len(queued[0].Sections[0].Jobs) != 2 || len(queued[0].Sections[0].ProgressJobs) != 3 {
		t.Fatalf("queued section = %+v", queued[0].Sections)
	}
}

func TestRepositoryPrefersRuntimeSchedulingBlocker(t *testing.T) {
	store := executionStoreStub{jobs: []protocol.JobExecution{{
		ID: "vault-job", Status: protocol.JobExecutionStatusQueued,
		RequiredCapabilities: map[string]string{"os": "darwin"},
		Metadata: map[string]string{
			protocol.JobSchedulingBlockedMetadataKey:       "1",
			protocol.JobSchedulingBlockedReasonMetadataKey: "Waiting for Vault connection home-vault: Vault is sealed",
		},
	}}}
	repository := NewRepository(store, 40, schedulingSourceStub{agents: []requirements.AgentSnapshot{{
		ID: "mac", OS: "darwin", Freshness: "online", Authorized: true,
	}}})
	queued, _, err := repository.ListFrontPageExecutionCards(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	diagnosis := queued[0].Sections[0].Jobs[0].SchedulingDiagnosis
	if diagnosis == nil || diagnosis.State != requirements.DiagnosisWaiting || !strings.Contains(diagnosis.Summary, "Vault is sealed") {
		t.Fatalf("runtime scheduling diagnosis = %+v", diagnosis)
	}
	details, err := repository.GetJobExecutionDetails(t.Context(), "vault-job")
	if err != nil {
		t.Fatal(err)
	}
	if details.SchedulingDiagnosis == nil || !strings.Contains(details.SchedulingDiagnosis.Summary, "Vault is sealed") {
		t.Fatalf("runtime job details diagnosis = %+v", details.SchedulingDiagnosis)
	}
}
