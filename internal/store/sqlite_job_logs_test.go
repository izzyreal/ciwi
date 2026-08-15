package store

import (
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/logtext"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestInteractiveJobLogPagesAndSearchesCompleteCleanOutput(t *testing.T) {
	s := openTestStore(t)
	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "echo test", StepPlan: []protocol.JobStepPlanItem{{Index: 1, Name: "Build"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	step := &protocol.JobStepPlanItem{Index: 1, Name: "Build"}
	prefix := strings.Repeat("x", logtext.ChunkBytes-2)
	events := []protocol.JobExecutionEvent{
		{Type: protocol.JobExecutionEventTypeSystemMessage, Message: "\x1b[31msystem\x1b[0m\r\n", TimestampUTC: time.Now().UTC()},
		{Type: protocol.JobExecutionEventTypeStepOutput, Step: step, Output: prefix + "abcdef tail", TimestampUTC: time.Now().UTC().Add(time.Nanosecond)},
	}
	if err := s.AppendJobExecutionEvents(job.ID, events); err != nil {
		t.Fatal(err)
	}
	descriptor, err := s.GetJobLogDescriptor(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !descriptor.Available || descriptor.Version != domain.InteractiveJobLogVersion || len(descriptor.Streams) != 2 {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	system, err := s.GetJobLogPage(job.ID, "", domain.JobLogPageHead, 0)
	if err != nil || len(system.Chunks) != 1 || system.Chunks[0].Text != "system\n" {
		t.Fatalf("system page = %+v, err = %v", system, err)
	}
	stepPage, err := s.GetJobLogPage(job.ID, "step:1", domain.JobLogPageHead, 0)
	if err != nil || len(stepPage.Chunks) != 2 || stepPage.HasBefore || stepPage.HasAfter {
		t.Fatalf("step page = %+v, err = %v", stepPage, err)
	}
	result, err := s.SearchJobLog(job.ID, "abcdef", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalMatches != 1 || result.Match == nil || result.Match.ItemID != "step:1" || result.Match.StartRune >= 0 {
		t.Fatalf("cross-chunk search = %+v", result)
	}
	if _, err := s.SearchJobLog(job.ID, "ab", 0); err == nil {
		t.Fatal("short search unexpectedly succeeded")
	}
}

func TestInteractiveJobLogDoesNotBackfillLegacyExecution(t *testing.T) {
	s := openTestStore(t)
	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE job_executions SET interactive_log_version = 0 WHERE id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendJobExecutionEvents(job.ID, []protocol.JobExecutionEvent{{
		Type: protocol.JobExecutionEventTypeSystemMessage, Message: "legacy output", TimestampUTC: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
	descriptor, err := s.GetJobLogDescriptor(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Available || descriptor.LatestChunkID != 0 || len(descriptor.Streams) != 0 {
		t.Fatalf("legacy descriptor = %+v", descriptor)
	}
}
