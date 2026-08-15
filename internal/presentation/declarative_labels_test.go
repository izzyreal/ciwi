package presentation

import (
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
)

func TestDeclarativeExecutionPresentationIsTransportIndependent(t *testing.T) {
	card := domain.ExecutionCard{
		JobExecutionIDs: []string{"job-1", "job-2"},
		Summary:         domain.ExecutionSummary{TotalJobs: 4, Succeeded: 1, Failed: 1, InProgress: 1, Waiting: 1},
	}
	display := PresentExecutionCard(card, true)
	if display.Status != "failed" || display.SummaryTone != "danger" || display.SummaryLabel != "1/4 successful, 1 failed, 1 in progress, 1 waiting" || display.JobExecutionIDsCSV != "job-1,job-2" {
		t.Fatalf("display = %+v", display)
	}
	started := time.Date(2026, 8, 7, 12, 0, 0, 0, time.Local)
	job := PresentExecutionCardJob(domain.ExecutionCardJob{
		Status: "failed", CreatedUTC: started.Add(-time.Minute), StartedUTC: started,
		TestSummary: &domain.JobTestSummary{Total: 20, Passed: 18, Failed: 2},
	}, started.Add(65*time.Second))
	if job.CreatedLabel == "" || job.DurationLabel != "" || job.StatusLabel != "failed (18/20 passed)" {
		t.Fatalf("job display = %+v", job)
	}
	running := PresentExecutionCardJob(domain.ExecutionCardJob{
		Status: "running", CreatedUTC: started.Add(-time.Minute), StartedUTC: started,
	}, started.Add(65*time.Second))
	if running.DurationLabel != "01m 05s" || running.StatusLabel != "running" {
		t.Fatalf("running job display = %+v", running)
	}
	withoutTests := PresentExecutionCardJob(domain.ExecutionCardJob{
		Status: "succeeded", TestSummary: &domain.JobTestSummary{},
	}, started)
	if withoutTests.StatusLabel != "succeeded" {
		t.Fatalf("zero-total job display = %+v", withoutTests)
	}
}

func TestDeclarativeProjectLabelsAreStable(t *testing.T) {
	if got := PipelineSummaryLabel(1, ""); got != "1 job · depends on: none" {
		t.Fatalf("pipeline summary = %q", got)
	}
	if got := PipelineGraphSummaryLabel(2, 1); got != "2 jobs · 1 dependency" {
		t.Fatalf("pipeline graph summary = %q", got)
	}
	if got := ProjectJobSummaryLabel(3, ""); got != "3 steps · runs on: unspecified" {
		t.Fatalf("job summary = %q", got)
	}
	if got := ProjectStepEnvironmentLabel([]string{"CI=true", "", " GOOS=darwin "}); got != "CI=true · GOOS=darwin" {
		t.Fatalf("environment label = %q", got)
	}
}
