package protocol

import (
	"testing"
	"time"
)

func TestLatestJobExecutionAttemptsSelectsNewestRerun(t *testing.T) {
	base := time.Now().UTC()
	jobs := []JobExecution{
		{ID: "job-rerun", CreatedUTC: base.Add(time.Second), Metadata: map[string]string{JobMetadataAttemptRootJobID: "job-original"}},
		{ID: "job-other", CreatedUTC: base},
		{ID: "job-original", CreatedUTC: base},
	}
	got := LatestJobExecutionAttempts(jobs)
	if len(got) != 2 || got[0].ID != "job-rerun" || got[1].ID != "job-other" {
		t.Fatalf("unexpected latest attempts: %+v", got)
	}
}
