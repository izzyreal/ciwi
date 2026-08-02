package presentation

import (
	"context"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
)

type jobDetailsSourceStub struct {
	details domain.JobExecutionDetails
}

func (s jobDetailsSourceStub) GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error) {
	return s.details, nil
}

func TestJobDetailsViewFormatsExecutionSnapshot(t *testing.T) {
	started := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	exitCode := 0
	view, err := NewJobDetailsQueries(jobDetailsSourceStub{details: domain.JobExecutionDetails{
		ID: "job-1", ProjectName: "ciwi", PipelineID: "build", PipelineJobID: "macos", MatrixName: "arm64",
		Status: "succeeded", StartedUTC: started, FinishedUTC: started.Add(1500 * time.Millisecond), ExitCode: &exitCode,
		Timeline: []domain.JobTimelineItem{{ID: "step:1", Kind: "step", Name: "Compile", Index: 1, Total: 1, Status: "succeeded", DurationMS: 1200}},
	}}).GetJobDetailsView(t.Context(), "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Title != "Job: macos" || view.Context != "ciwi · pipeline build · arm64 · execution job-1" || view.Duration != "1.5s" {
		t.Fatalf("view = %+v", view)
	}
	if len(view.Timeline) != 1 || view.Timeline[0].Title != "Job step 1/1: Compile" || view.Timeline[0].Duration != "1.2s" {
		t.Fatalf("timeline = %+v", view.Timeline)
	}
}
