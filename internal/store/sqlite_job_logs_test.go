package store

import (
	"fmt"
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
	if len(descriptor.Streams) != 2 {
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
	result, err := s.SearchJobLog(job.ID, "step:1", "abcdef", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalMatches != 1 || result.Match == nil || result.Match.ItemID != "step:1" || result.Match.StartRune >= 0 {
		t.Fatalf("cross-chunk search = %+v", result)
	}
	if _, err := s.SearchJobLog(job.ID, "step:1", "ab", 0); err == nil {
		t.Fatal("short search unexpectedly succeeded")
	}
}

func TestInteractiveJobLogIsUnconditional(t *testing.T) {
	s := openTestStore(t)
	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendJobExecutionEvents(job.ID, []protocol.JobExecutionEvent{{
		Type: protocol.JobExecutionEventTypeSystemMessage, Message: "indexed output", TimestampUTC: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
	descriptor, err := s.GetJobLogDescriptor(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.LatestChunkID == 0 || len(descriptor.Streams) != 1 {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	var obsoleteColumn int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('job_executions') WHERE name = 'interactive_log_version'`).Scan(&obsoleteColumn); err != nil || obsoleteColumn != 0 {
		t.Fatalf("obsolete log version column count = %d, err = %v", obsoleteColumn, err)
	}
}

func TestInteractiveJobLogSearchUsesFTSFirstAndScopesToSelectedItem(t *testing.T) {
	s := openTestStore(t)
	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "echo test",
		StepPlan: []protocol.JobStepPlanItem{
			{Index: 1, Name: "First"},
			{Index: 2, Name: "Second"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendJobExecutionEvents(job.ID, []protocol.JobExecutionEvent{
		{Type: protocol.JobExecutionEventTypeSystemMessage, Message: "shared needle", TimestampUTC: time.Now().UTC().Add(-time.Nanosecond)},
		{Type: protocol.JobExecutionEventTypeStepOutput, Step: &protocol.JobStepPlanItem{Index: 2}, Output: "shared needle", TimestampUTC: time.Now().UTC()},
		{Type: protocol.JobExecutionEventTypeStepOutput, Step: &protocol.JobStepPlanItem{Index: 1}, Output: "shared needle", TimestampUTC: time.Now().UTC().Add(time.Nanosecond)},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := s.SearchJobLog(job.ID, "step:1", "needle", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalMatches != 1 || result.Match == nil || result.Match.ItemID != "step:1" {
		t.Fatalf("step:1 search = %+v", result)
	}
	second, err := s.SearchJobLog(job.ID, "step:2", "needle", 0)
	if err != nil || second.TotalMatches != 1 || second.Match == nil || second.Match.ItemID != "step:2" {
		t.Fatalf("step:2 search = %+v, err = %v", second, err)
	}

	query := jobLogSearchCandidateQuery()
	rows, err := s.db.Query("EXPLAIN QUERY PLAN "+query,
		`job_execution_id : "missing" AND indexed_text : "needle"`, job.ID, "step:1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(details) == 0 || !strings.Contains(details[0], "SCAN f VIRTUAL TABLE") {
		t.Fatalf("search query plan = %s, want FTS as the outer candidate scan", fmt.Sprint(details))
	}
}
