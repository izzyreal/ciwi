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
	if len(got) != 4 {
		t.Fatalf("expected 4 jobs (run-b kept whole), got %d", len(got))
	}
	if got[3].ID != "job-4" {
		t.Fatalf("expected run-b tail job preserved, got %q", got[3].ID)
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
