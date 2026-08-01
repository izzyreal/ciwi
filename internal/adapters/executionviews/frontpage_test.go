package executionviews

import (
	"context"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type executionStoreStub struct {
	jobs []protocol.JobExecution
}

func (s executionStoreStub) ListJobExecutions() ([]protocol.JobExecution, error) {
	return append([]protocol.JobExecution(nil), s.jobs...), nil
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
