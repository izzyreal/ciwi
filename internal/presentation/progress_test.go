package presentation

import (
	"math"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
)

func TestProgressForRunningExecutionUsesEstimateAndSnapshotRate(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	progress := progressForInput(progressInput{
		status: "running", started: now.Add(-3 * time.Second), expectedDurationMS: 10_000,
	}, now)
	if progress.State != domain.ProgressDeterminate || math.Abs(progress.Fraction-.3) > .0001 {
		t.Fatalf("progress = %+v", progress)
	}
	if math.Abs(progress.RatePerMS-.0001) > .0000001 || progress.SnapshotUnixMS != now.UnixMilli() {
		t.Fatalf("interpolation = %+v", progress)
	}
}

func TestAggregateProgressWeightsCompletedRunningAndWaitingJobs(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	progress := aggregateProgress([]progressInput{
		{status: "succeeded", started: now.Add(-2 * time.Second), finished: now, expectedDurationMS: 2_000},
		{status: "running", started: now.Add(-time.Second), expectedDurationMS: 4_000},
		{status: "queued", waiting: true, expectedDurationMS: 2_000},
	}, now)
	// (2000 complete + 1000 elapsed) / 8000 estimated milliseconds.
	if progress.State != domain.ProgressDeterminate || math.Abs(progress.Fraction-.375) > .0001 {
		t.Fatalf("aggregate progress = %+v", progress)
	}
	if math.Abs(progress.RatePerMS-.000125) > .0000001 {
		t.Fatalf("aggregate rate = %g", progress.RatePerMS)
	}
}

func TestAggregateProgressIsIndeterminateWhenActiveEstimateIsUnknown(t *testing.T) {
	progress := aggregateProgress([]progressInput{{status: "running"}}, time.Now().UTC())
	if progress.State != domain.ProgressIndeterminate {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestWaitingOnlyExecutionDoesNotPretendToAdvance(t *testing.T) {
	progress := aggregateProgress([]progressInput{{status: "queued", waiting: true, expectedDurationMS: 1_000}}, time.Now().UTC())
	if progress.State != domain.ProgressNone || progress.Fraction != 0 {
		t.Fatalf("progress = %+v", progress)
	}
}
