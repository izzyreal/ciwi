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
	events, err := s.ListJobExecutionEvents(job.ID)
	if err != nil {
		t.Fatalf("ListJobExecutionEvents: %v", err)
	}
	if len(events) != 1 || !strings.Contains(events[0].Message, "[control] job timed out while running") {
		t.Fatalf("expected timeout control event, got %+v", events)
	}
}

func TestCancellationTerminalResultIsSticky(t *testing.T) {
	for _, initial := range []string{"queued", "leased", "running"} {
		for _, first := range []string{"cancelled", "succeeded", "failed"} {
			t.Run(initial+"/"+first, func(t *testing.T) {
				s := openTestStore(t)
				job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo test", TimeoutSeconds: 30})
				if err != nil {
					t.Fatal(err)
				}
				if initial == "leased" {
					if _, err := s.LeaseJobExecution("agent", nil); err != nil {
						t.Fatal(err)
					}
				} else if initial == "running" {
					if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{AgentID: "agent", Status: initial}); err != nil {
						t.Fatal(err)
					}
				}
				winner, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{AgentID: "agent", Status: first, Error: "original reason"})
				if err != nil {
					t.Fatal(err)
				}
				if winner.FinishedUTC.IsZero() || winner.CurrentStep != "" {
					t.Fatalf("invalid terminal lifecycle: %+v", winner)
				}
				for _, late := range []string{"running", "failed", "succeeded", "cancelled"} {
					got, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{AgentID: "agent", Status: late, Error: "late reason", CurrentStep: "late step"})
					if err != nil {
						t.Fatal(err)
					}
					if got.Status != winner.Status || got.Error != winner.Error || !got.FinishedUTC.Equal(winner.FinishedUTC) || !got.StartedUTC.Equal(winner.StartedUTC) || got.CurrentStep != "" {
						t.Fatalf("late %s changed terminal result: %+v", late, got)
					}
				}
			})
		}
	}
}

func TestCancellationRacesWithCompletion(t *testing.T) {
	s := openTestStore(t)
	for attempt := 0; attempt < 20; attempt++ {
		job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo test", TimeoutSeconds: 30})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan protocol.JobExecution, 2)
		errs := make(chan error, 2)
		for _, status := range []string{"cancelled", "succeeded"} {
			go func(status string) {
				<-start
				result, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{AgentID: "agent", Status: status})
				results <- result
				errs <- err
			}(status)
		}
		close(start)
		a, b := <-results, <-results
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
		if a.Status != b.Status || !a.FinishedUTC.Equal(b.FinishedUTC) {
			t.Fatalf("conflicting terminal results: %s / %s", a.Status, b.Status)
		}
	}
}
