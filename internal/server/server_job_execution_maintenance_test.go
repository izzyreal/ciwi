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

	now := time.Now().UTC()
	if _, err := db.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:     "agent-bhakti",
		Status:      protocol.JobExecutionStatusRunning,
		CurrentStep: "Checking out source",
	}); err != nil {
		t.Fatalf("mark running: %v", err)
	}

	if err := s.runJobExecutionMaintenancePass(now.Add(30 * time.Second)); err != nil {
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
	events, err := db.ListJobExecutionEvents(job.ID)
	if err != nil {
		t.Fatalf("list timeout events: %v", err)
	}
	if len(events) != 1 || !strings.Contains(events[0].Message, "[control] "+jobExecutionTimeoutReaperErrorMsg) {
		t.Fatalf("expected timeout control event, got %+v", events)
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

func TestServerMaintenancePropagatesTimeoutToBlockedDependents(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &stateStore{db: db}

	common := map[string]string{
		"project": "ciwi", "pipeline_id": "build", "pipeline_run_id": "run-timeout",
	}
	upstreamMeta := map[string]string{}
	for key, value := range common {
		upstreamMeta[key] = value
	}
	upstreamMeta["pipeline_job_id"] = "unit-tests"
	upstream, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "go test ./...", RequiredCapabilities: map[string]string{"os": "linux"}, TimeoutSeconds: 1, Metadata: upstreamMeta,
	})
	if err != nil {
		t.Fatalf("create upstream: %v", err)
	}
	dependentMeta := map[string]string{}
	for key, value := range common {
		dependentMeta[key] = value
	}
	dependentMeta["pipeline_job_id"] = "package"
	dependentMeta["needs_blocked"] = "1"
	dependentMeta["needs_job_ids"] = "unit-tests"
	dependent, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script: "go build ./...", RequiredCapabilities: map[string]string{"os": "linux"}, TimeoutSeconds: 30, Metadata: dependentMeta,
	})
	if err != nil {
		t.Fatalf("create dependent: %v", err)
	}

	leased, err := db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil || leased == nil || leased.ID != upstream.ID {
		t.Fatalf("lease upstream: job=%+v err=%v", leased, err)
	}
	running, err := db.UpdateJobExecutionStatus(upstream.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1", Status: protocol.JobExecutionStatusRunning,
	})
	if err != nil {
		t.Fatalf("mark upstream running: %v", err)
	}
	if err := s.runJobExecutionMaintenancePass(running.StartedUTC.Add(30 * time.Second)); err != nil {
		t.Fatalf("maintenance pass: %v", err)
	}

	got, err := db.GetJobExecution(dependent.ID)
	if err != nil {
		t.Fatalf("get dependent: %v", err)
	}
	if got.Status != protocol.JobExecutionStatusFailed || !strings.Contains(got.Error, "required job unit-tests failed") {
		t.Fatalf("expected timeout failure to propagate, got status=%q error=%q", got.Status, got.Error)
	}
}
