package store

import (
	"sort"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestFlushJobExecutionHistoryByAgent(t *testing.T) {
	s := openTestStore(t)

	makeJob := func(script string) protocol.JobExecution {
		t.Helper()
		j, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
			Script:         script,
			TimeoutSeconds: 30,
		})
		if err != nil {
			t.Fatalf("CreateJobExecution: %v", err)
		}
		return j
	}

	// Terminal + leased by target agent => should be deleted.
	leasedTerminal := makeJob("echo leased-terminal")
	if _, err := s.LeaseJobExecution("agent-x", nil); err != nil {
		t.Fatalf("LeaseJobExecution: %v", err)
	}
	if _, err := s.UpdateJobExecutionStatus(leasedTerminal.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-x",
		Status:  protocol.JobExecutionStatusFailed,
		Error:   "boom",
	}); err != nil {
		t.Fatalf("UpdateJobExecutionStatus leasedTerminal: %v", err)
	}

	// Terminal + adhoc_agent_id metadata => should be deleted.
	adhocTerminal := makeJob("echo adhoc-terminal")
	if _, err := s.MergeJobExecutionMetadata(adhocTerminal.ID, map[string]string{"adhoc_agent_id": "agent-x"}); err != nil {
		t.Fatalf("MergeJobExecutionMetadata: %v", err)
	}
	if _, err := s.UpdateJobExecutionStatus(adhocTerminal.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-y",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("UpdateJobExecutionStatus adhocTerminal: %v", err)
	}

	// Active job should never be deleted even if metadata matches.
	active := makeJob("echo running")
	if _, err := s.MergeJobExecutionMetadata(active.ID, map[string]string{"adhoc_agent_id": "agent-x"}); err != nil {
		t.Fatalf("MergeJobExecutionMetadata active: %v", err)
	}
	if _, err := s.UpdateJobExecutionStatus(active.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-z",
		Status:  protocol.JobExecutionStatusRunning,
	}); err != nil {
		t.Fatalf("UpdateJobExecutionStatus active: %v", err)
	}

	// Different agent terminal history should remain.
	otherAgentTerminal := makeJob("echo other-terminal")
	if _, err := s.UpdateJobExecutionStatus(otherAgentTerminal.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-y",
		Status:  protocol.JobExecutionStatusFailed,
		Error:   "boom",
	}); err != nil {
		t.Fatalf("UpdateJobExecutionStatus otherAgentTerminal: %v", err)
	}

	deleted, err := s.FlushJobExecutionHistoryByAgent("agent-x")
	if err != nil {
		t.Fatalf("FlushJobExecutionHistoryByAgent: %v", err)
	}
	sort.Strings(deleted)
	wantDeleted := []string{adhocTerminal.ID, leasedTerminal.ID}
	sort.Strings(wantDeleted)
	if len(deleted) != len(wantDeleted) || deleted[0] != wantDeleted[0] || deleted[1] != wantDeleted[1] {
		t.Fatalf("unexpected deleted ids: got=%v want=%v", deleted, wantDeleted)
	}

	if _, err := s.GetJobExecution(leasedTerminal.ID); err == nil {
		t.Fatalf("expected leasedTerminal to be deleted")
	}
	if _, err := s.GetJobExecution(adhocTerminal.ID); err == nil {
		t.Fatalf("expected adhocTerminal to be deleted")
	}
	if _, err := s.GetJobExecution(active.ID); err != nil {
		t.Fatalf("expected active job to remain, got err=%v", err)
	}
	if _, err := s.GetJobExecution(otherAgentTerminal.ID); err != nil {
		t.Fatalf("expected other agent terminal job to remain, got err=%v", err)
	}
}

func TestFlushJobExecutionHistoryByAgentValidationAndNoop(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.FlushJobExecutionHistoryByAgent("   "); err == nil {
		t.Fatalf("expected empty agent id to fail")
	}

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo only-other-agent",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("CreateJobExecution: %v", err)
	}
	if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-a",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("UpdateJobExecutionStatus: %v", err)
	}

	deleted, err := s.FlushJobExecutionHistoryByAgent("agent-x")
	if err != nil {
		t.Fatalf("FlushJobExecutionHistoryByAgent noop: %v", err)
	}
	if deleted != nil {
		t.Fatalf("expected nil deleted list for noop, got %v", deleted)
	}
}

func TestMergeJobExecutionEnvAndMetadataInputValidation(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.MergeJobExecutionEnv("", map[string]string{"A": "1"}); err == nil {
		t.Fatalf("expected MergeJobExecutionEnv to reject empty job id")
	}
	if _, err := s.MergeJobExecutionMetadata("", map[string]string{"A": "1"}); err == nil {
		t.Fatalf("expected MergeJobExecutionMetadata to reject empty job id")
	}
}
