package jobexecution

import (
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestCapDisplayJobsKeepsRunGroupWhole(t *testing.T) {
	jobs := []protocol.JobExecution{
		{ID: "job-1", Metadata: map[string]string{"pipeline_run_id": "run-a"}},
		{ID: "job-2", Metadata: map[string]string{"pipeline_run_id": "run-a"}},
		{ID: "job-3", Metadata: map[string]string{"pipeline_run_id": "run-b"}},
		{ID: "job-4", Metadata: map[string]string{"pipeline_run_id": "run-b"}},
		{ID: "job-5", Metadata: map[string]string{"pipeline_run_id": "run-c"}},
	}

	got := CapDisplayJobs(jobs, 3)
	if len(got) != 2 {
		t.Fatalf("expected 2 jobs (only first complete run fits), got %d", len(got))
	}
	if got[0].ID != "job-1" || got[1].ID != "job-2" {
		t.Fatalf("unexpected jobs kept: %#v", []string{got[0].ID, got[1].ID})
	}
}

func TestCapDisplayJobsDoesNotPullNewGroupsPastCap(t *testing.T) {
	jobs := []protocol.JobExecution{
		{ID: "job-1", Metadata: map[string]string{"pipeline_run_id": "run-a"}},
		{ID: "job-2", Metadata: map[string]string{"pipeline_run_id": "run-a"}},
		{ID: "job-3", Metadata: map[string]string{}},
		{ID: "job-4", Metadata: map[string]string{"pipeline_run_id": "run-b"}},
		{ID: "job-5", Metadata: map[string]string{"pipeline_run_id": "run-b"}},
	}

	got := CapDisplayJobs(jobs, 3)
	if len(got) != 3 {
		t.Fatalf("expected capped size 3, got %d", len(got))
	}
	if got[2].ID != "job-3" {
		t.Fatalf("expected standalone third job retained, got %q", got[2].ID)
	}
}

func TestCapDisplayJobsIncludesFirstGroupWhenItExceedsCap(t *testing.T) {
	jobs := []protocol.JobExecution{
		{ID: "job-1", Metadata: map[string]string{"pipeline_run_id": "run-a"}},
		{ID: "job-2", Metadata: map[string]string{"pipeline_run_id": "run-a"}},
		{ID: "job-3", Metadata: map[string]string{"pipeline_run_id": "run-a"}},
		{ID: "job-4", Metadata: map[string]string{"pipeline_run_id": "run-b"}},
	}

	got := CapDisplayJobs(jobs, 2)
	if len(got) != 3 {
		t.Fatalf("expected full first run kept, got %d jobs", len(got))
	}
	if got[0].ID != "job-1" || got[1].ID != "job-2" || got[2].ID != "job-3" {
		t.Fatalf("unexpected jobs kept: %#v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}
