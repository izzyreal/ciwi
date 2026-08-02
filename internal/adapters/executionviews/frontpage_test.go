package executionviews

import (
	"context"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type executionStoreStub struct {
	jobs   []protocol.JobExecution
	events map[string][]protocol.JobExecutionEvent
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

func (s executionStoreStub) ListJobExecutions() ([]protocol.JobExecution, error) {
	return append([]protocol.JobExecution(nil), s.jobs...), nil
}

func TestRepositoryBuildsTransportNeutralJobTimeline(t *testing.T) {
	now := time.Now().UTC()
	exitCode := 1
	step := protocol.JobStepPlanItem{Index: 1, Total: 2, Name: "Compile"}
	repository := NewRepository(executionStoreStub{
		jobs: []protocol.JobExecution{{
			ID: "job-1", Status: "failed", CreatedUTC: now, StartedUTC: now.Add(time.Second),
			FinishedUTC: now.Add(3 * time.Second), StepPlan: []protocol.JobStepPlanItem{step, {Index: 2, Total: 2, Name: "Package"}},
			Metadata: map[string]string{"project": "ciwi", "pipeline_id": "build", "pipeline_job_id": "macos", "dry_run": "1"},
		}},
		events: map[string][]protocol.JobExecutionEvent{"job-1": {
			{Type: protocol.JobExecutionEventTypeStepStarted, Step: &step},
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
}

func TestRepositoryUsesEstablishedExecutionGrouping(t *testing.T) {
	now := time.Now().UTC()
	repository := NewRepository(executionStoreStub{jobs: []protocol.JobExecution{
		{ID: "running", Status: "running", CreatedUTC: now, Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": "run-build",
		}},
		{ID: "queued", Status: "queued", CreatedUTC: now.Add(-time.Second), Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": "run-build",
		}},
		{ID: "finished", Status: "succeeded", CreatedUTC: now.Add(-2 * time.Second), Metadata: map[string]string{
			"project": "ciwi", "pipeline_id": "test", "pipeline_run_id": "run-test",
		}},
	}}, 40)
	queued, history, err := repository.ListFrontPageExecutionCards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].Summary.TotalJobs != 2 || queued[0].Summary.InProgress != 2 {
		t.Fatalf("queued cards = %+v", queued)
	}
	if len(history) != 1 || history[0].Summary.Succeeded != 1 {
		t.Fatalf("history cards = %+v", history)
	}
}
