package jobhistory

import (
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestSummaryCardsFiltersActivityAndAppliesLimit(t *testing.T) {
	jobs := []struct {
		id, status, created, pipeline string
	}{
		{id: "queued", status: "queued", created: "2026-03-29T10:25:36Z", pipeline: "run-active"},
		{id: "running", status: "running", created: "2026-03-29T10:25:35Z", pipeline: "run-active"},
		{id: "finished-new", status: "succeeded", created: "2026-03-29T10:25:34Z", pipeline: "run-finished-new"},
		{id: "finished-old", status: "failed", created: "2026-03-29T10:20:00Z", pipeline: "run-finished-old"},
	}
	input := makeJobExecutions(jobs)
	active := SummaryCards(input, true, 0)
	if len(active) != 1 || active[0].Summary.Waiting != 1 || active[0].Summary.InProgress != 1 {
		t.Fatalf("active cards = %#v", active)
	}
	finished := SummaryCards(input, false, 1)
	if len(finished) != 1 || finished[0].Summary.Succeeded != 1 {
		t.Fatalf("finished cards = %#v", finished)
	}
}

func makeJobExecutions(values []struct{ id, status, created, pipeline string }) []protocol.JobExecution {
	jobs := make([]protocol.JobExecution, 0, len(values))
	for _, value := range values {
		metadata := map[string]string{
			"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": value.pipeline,
		}
		if value.id == "queued" {
			metadata["needs_blocked"] = "1"
		}
		jobs = append(jobs, job(value.id, value.status, value.created, metadata))
	}
	return jobs
}
