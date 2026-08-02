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

func (s jobDetailsSourceStub) GetJobOutput(context.Context, string, int64) (domain.JobOutputBatch, error) {
	exitCode := 2
	return domain.JobOutputBatch{
		JobExecutionID: "job-1", NextEventID: 3, Terminal: true,
		Events: []domain.JobOutputEvent{
			{ID: 1, Type: domain.JobOutputEventOutput, Output: "\x1b[31mcompile output\x1b[0m"},
			{ID: 2, Type: domain.JobOutputEventFinished, ItemKind: "step", ItemName: "Compile", ItemIndex: 1, ItemTotal: 1, ExitCode: &exitCode, Error: "exit=2"},
		},
	}, nil
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
	if !view.CanRerun || view.CanCancel {
		t.Fatalf("execution controls = can rerun %v, can cancel %v", view.CanRerun, view.CanCancel)
	}
	if len(view.Timeline) != 1 || view.Timeline[0].Title != "Job step 1/1: Compile" || view.Timeline[0].Duration != "1.2s" {
		t.Fatalf("timeline = %+v", view.Timeline)
	}
}

func TestJobDetailsViewExposesEligibleControls(t *testing.T) {
	queued := presentJobDetails(domain.JobExecutionDetails{ID: "queued", Status: "queued"})
	if !queued.CanCancel || queued.CanRerun {
		t.Fatalf("queued controls = %+v", queued)
	}
	blocked := presentJobDetails(domain.JobExecutionDetails{
		ID: "blocked", Status: "failed", Error: "cancelled: upstream pipeline build failed",
	})
	if blocked.CanCancel || !blocked.CanRerun {
		t.Fatalf("blocked controls = %+v", blocked)
	}
}

func TestJobOutputViewRendersSanitizedIncrementalLines(t *testing.T) {
	view, err := NewJobDetailsQueries(jobDetailsSourceStub{}).GetJobOutputView(t.Context(), "job-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextEventID != 3 || !view.Terminal || len(view.Lines) != 2 {
		t.Fatalf("view = %+v", view)
	}
	if view.Lines[0].Text != "compile output\n" || view.Lines[1].Text != "[step] failed: 1/1: Compile (exit=2)\n" {
		t.Fatalf("lines = %+v", view.Lines)
	}
}
