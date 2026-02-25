package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestRetrySQLiteBusyBranches(t *testing.T) {
	attempts := 0
	err := retrySQLiteBusy(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}

	attempts = 0
	err = retrySQLiteBusy(func() error {
		attempts++
		return errors.New("database is locked")
	})
	if err == nil {
		t.Fatalf("expected retry exhaustion error")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts before giving up, got %d", attempts)
	}

	attempts = 0
	err = retrySQLiteBusy(func() error {
		attempts++
		return errors.New("permanent failure")
	})
	if err == nil || !strings.Contains(err.Error(), "permanent failure") {
		t.Fatalf("expected permanent failure passthrough, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected non-busy errors to avoid retries, got %d attempts", attempts)
	}
}

func TestDeleteQueuedJobExecutionBranches(t *testing.T) {
	s := openTestStore(t)

	if err := s.DeleteQueuedJobExecution("missing-id"); err == nil || !strings.Contains(err.Error(), "job not found") {
		t.Fatalf("expected missing job error, got %v", err)
	}

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo running",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("CreateJobExecution: %v", err)
	}
	if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-a",
		Status:  protocol.JobExecutionStatusRunning,
	}); err != nil {
		t.Fatalf("UpdateJobExecutionStatus running: %v", err)
	}
	if err := s.DeleteQueuedJobExecution(job.ID); err == nil || !strings.Contains(err.Error(), "job is not pending") {
		t.Fatalf("expected not pending error, got %v", err)
	}
}

func TestMergeJobExecutionEnvAndMetadataBehavior(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo env-meta",
		Env:            map[string]string{"KEEP": "1", "DROP": "x"},
		Metadata:       map[string]string{"MKEEP": "1", "MDROP": "x"},
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("CreateJobExecution: %v", err)
	}

	env, err := s.MergeJobExecutionEnv(job.ID, map[string]string{
		"  ADD  ": "2",
		"DROP":    "   ",
		"   ":     "ignored",
	})
	if err != nil {
		t.Fatalf("MergeJobExecutionEnv: %v", err)
	}
	if env["KEEP"] != "1" || env["ADD"] != "2" {
		t.Fatalf("unexpected merged env: %#v", env)
	}
	if _, ok := env["DROP"]; ok {
		t.Fatalf("expected DROP to be removed, env=%#v", env)
	}

	meta, err := s.MergeJobExecutionMetadata(job.ID, map[string]string{
		"NEW":   "2",
		"MDROP": "",
	})
	if err != nil {
		t.Fatalf("MergeJobExecutionMetadata: %v", err)
	}
	if meta["MKEEP"] != "1" || meta["NEW"] != "2" {
		t.Fatalf("unexpected merged metadata: %#v", meta)
	}
	if _, ok := meta["MDROP"]; ok {
		t.Fatalf("expected MDROP removed, meta=%#v", meta)
	}

	// Empty patch should return a clone of persisted values, not a live map.
	envSnapshot, err := s.MergeJobExecutionEnv(job.ID, nil)
	if err != nil {
		t.Fatalf("MergeJobExecutionEnv empty patch: %v", err)
	}
	envSnapshot["MUTATE"] = "x"
	again, err := s.MergeJobExecutionEnv(job.ID, nil)
	if err != nil {
		t.Fatalf("MergeJobExecutionEnv empty patch second read: %v", err)
	}
	if _, ok := again["MUTATE"]; ok {
		t.Fatalf("expected returned env map clone, got %#v", again)
	}

	if _, err := s.MergeJobExecutionEnv("missing-id", map[string]string{"A": "1"}); err == nil || !strings.Contains(err.Error(), "job not found") {
		t.Fatalf("expected missing job env error, got %v", err)
	}
	if _, err := s.MergeJobExecutionMetadata("missing-id", map[string]string{"A": "1"}); err == nil || !strings.Contains(err.Error(), "job not found") {
		t.Fatalf("expected missing job metadata error, got %v", err)
	}
}

func TestFailTimedOutRunningJobExecutionsDefaults(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo timeout",
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("CreateJobExecution: %v", err)
	}
	started := time.Now().UTC().Add(-10 * time.Second)
	if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-a",
		Status:  protocol.JobExecutionStatusRunning,
	}); err != nil {
		t.Fatalf("UpdateJobExecutionStatus running: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE job_executions SET started_utc = ? WHERE id = ?`, started.Format(time.RFC3339Nano), job.ID); err != nil {
		t.Fatalf("backdate started_utc: %v", err)
	}

	failed, err := s.FailTimedOutRunningJobExecutions(time.Time{}, -1*time.Second, "")
	if err != nil {
		t.Fatalf("FailTimedOutRunningJobExecutions: %v", err)
	}
	if failed != 1 {
		t.Fatalf("expected one timed out job failed, got %d", failed)
	}
	got, err := s.GetJobExecution(job.ID)
	if err != nil {
		t.Fatalf("GetJobExecution: %v", err)
	}
	if protocol.NormalizeJobExecutionStatus(got.Status) != protocol.JobExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", got.Status)
	}
	if !strings.Contains(got.Error, "job timed out while running") {
		t.Fatalf("expected default timeout reason, got %q", got.Error)
	}
	if !strings.Contains(got.Output, "[control] job timed out while running") {
		t.Fatalf("expected control marker in output, got %q", got.Output)
	}
}
