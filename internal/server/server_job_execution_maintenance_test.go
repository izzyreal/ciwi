package server

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func TestServerMaintenanceRecoversOrphanedCheckoutJobAfterRestart(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := &stateStore{db: db}

	job, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo hi",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       5,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	leased, err := db.LeaseJobExecution("agent-bhakti", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease job: %v", err)
	}
	if leased == nil || leased.ID != job.ID {
		t.Fatalf("expected leased job %q", job.ID)
	}

	startedAt := time.Now().UTC().Add(-20 * time.Second)
	if _, err := db.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-bhakti",
		Status:       protocol.JobExecutionStatusRunning,
		CurrentStep:  "Checking out source",
		Output:       "[checkout] repo=https://example ref=abc",
		TimestampUTC: startedAt,
	}); err != nil {
		t.Fatalf("mark running: %v", err)
	}

	if err := s.runJobExecutionMaintenancePass(startedAt.Add(20 * time.Second)); err != nil {
		t.Fatalf("maintenance pass: %v", err)
	}

	got, err := db.GetJobExecution(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != protocol.JobExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", got.Status)
	}
	if got.FinishedUTC.IsZero() {
		t.Fatalf("expected finished timestamp to be set")
	}
	if !strings.Contains(got.Error, "timed out") {
		t.Fatalf("expected timed out error, got %q", got.Error)
	}
	if !strings.Contains(got.Output, "[control] "+jobExecutionTimeoutReaperErrorMsg) {
		t.Fatalf("expected timeout control marker in output, got %q", got.Output)
	}
}

func TestServerMaintenanceRequeuesStaleLeasedJobOnStartup(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := &stateStore{db: db}

	job, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo hi",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	leased, err := db.LeaseJobExecution("agent-bhakti", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease job: %v", err)
	}
	if leased == nil || leased.ID != job.ID {
		t.Fatalf("expected leased job %q", job.ID)
	}

	if err := s.runJobExecutionMaintenancePass(leased.LeasedUTC.Add(2 * time.Minute)); err != nil {
		t.Fatalf("maintenance pass: %v", err)
	}

	got, err := db.GetJobExecution(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != protocol.JobExecutionStatusQueued {
		t.Fatalf("expected queued status, got %q", got.Status)
	}
	if got.LeasedByAgentID != "" {
		t.Fatalf("expected lease owner to be cleared, got %q", got.LeasedByAgentID)
	}
}
