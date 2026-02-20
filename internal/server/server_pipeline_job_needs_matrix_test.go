package server

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func openNeedsMatrixStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func enqueueNeedsMatrixPipeline(t *testing.T, s *stateStore) {
	t.Helper()
	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: smoke
        runs_on:
          os: linux
        matrix:
          include:
            - name: linux-a
            - name: linux-b
        timeout_seconds: 30
        steps:
          - run: echo smoke
      - id: package
        needs:
          - smoke
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo package
`), "needs-matrix")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	p, err := s.db.GetPipelineByProjectAndID("ciwi", "build")
	if err != nil {
		t.Fatalf("get pipeline: %v", err)
	}
	if _, err := s.enqueuePersistedPipeline(p, nil); err != nil {
		t.Fatalf("enqueue pipeline: %v", err)
	}
}

func findNeedsPackageJob(t *testing.T, s *stateStore) protocol.JobExecution {
	t.Helper()
	jobs, err := s.db.ListJobExecutions()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for _, j := range jobs {
		if strings.TrimSpace(j.Metadata["pipeline_job_id"]) == "package" {
			return j
		}
	}
	t.Fatalf("missing package job")
	return protocol.JobExecution{}
}

func TestNeedsMatrixUnblocksOnlyAfterAllUpstreamVariantsSucceed(t *testing.T) {
	s := &stateStore{db: openNeedsMatrixStore(t)}
	enqueueNeedsMatrixPipeline(t, s)

	pkg := findNeedsPackageJob(t, s)
	if strings.TrimSpace(pkg.Metadata["needs_blocked"]) != "1" {
		t.Fatalf("expected package job to start blocked, metadata=%v", pkg.Metadata)
	}

	first, err := s.db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil || first == nil {
		t.Fatalf("lease first smoke: job=%+v err=%v", first, err)
	}
	if strings.TrimSpace(first.Metadata["pipeline_job_id"]) != "smoke" {
		t.Fatalf("expected smoke first, got %+v", first.Metadata)
	}
	firstDone, err := s.db.UpdateJobExecutionStatus(first.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusSucceeded,
		Output:       "ok",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark first smoke success: %v", err)
	}
	s.onJobExecutionUpdated(firstDone)

	pkgAfterFirst, err := s.db.GetJobExecution(pkg.ID)
	if err != nil {
		t.Fatalf("get package after first smoke: %v", err)
	}
	if strings.TrimSpace(pkgAfterFirst.Metadata["needs_blocked"]) != "1" {
		t.Fatalf("expected package to remain blocked until all smoke variants succeed, metadata=%v", pkgAfterFirst.Metadata)
	}

	second, err := s.db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil || second == nil {
		t.Fatalf("lease second smoke: job=%+v err=%v", second, err)
	}
	secondDone, err := s.db.UpdateJobExecutionStatus(second.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusSucceeded,
		Output:       "ok",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark second smoke success: %v", err)
	}
	s.onJobExecutionUpdated(secondDone)

	pkgAfterSecond, err := s.db.GetJobExecution(pkg.ID)
	if err != nil {
		t.Fatalf("get package after second smoke: %v", err)
	}
	if strings.TrimSpace(pkgAfterSecond.Metadata["needs_blocked"]) != "" {
		t.Fatalf("expected package unblock after all smoke variants succeed, metadata=%v", pkgAfterSecond.Metadata)
	}
}

func TestNeedsMatrixCancelsOnlyAfterAllUpstreamVariantsTerminal(t *testing.T) {
	s := &stateStore{db: openNeedsMatrixStore(t)}
	enqueueNeedsMatrixPipeline(t, s)

	pkg := findNeedsPackageJob(t, s)

	first, err := s.db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil || first == nil {
		t.Fatalf("lease first smoke: job=%+v err=%v", first, err)
	}
	firstDone, err := s.db.UpdateJobExecutionStatus(first.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusFailed,
		Error:        "boom",
		Output:       "boom",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark first smoke failure: %v", err)
	}
	s.onJobExecutionUpdated(firstDone)

	pkgAfterFirst, err := s.db.GetJobExecution(pkg.ID)
	if err != nil {
		t.Fatalf("get package after first failure: %v", err)
	}
	if protocol.NormalizeJobExecutionStatus(pkgAfterFirst.Status) != protocol.JobExecutionStatusQueued {
		t.Fatalf("expected package to remain queued while another smoke variant not terminal, got %q", pkgAfterFirst.Status)
	}

	second, err := s.db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil || second == nil {
		t.Fatalf("lease second smoke: job=%+v err=%v", second, err)
	}
	secondDone, err := s.db.UpdateJobExecutionStatus(second.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusSucceeded,
		Output:       "ok",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark second smoke success: %v", err)
	}
	s.onJobExecutionUpdated(secondDone)

	pkgAfterSecond, err := s.db.GetJobExecution(pkg.ID)
	if err != nil {
		t.Fatalf("get package after mixed terminal states: %v", err)
	}
	if protocol.NormalizeJobExecutionStatus(pkgAfterSecond.Status) != protocol.JobExecutionStatusFailed {
		t.Fatalf("expected package cancellation after all smoke variants terminal with one failure, got %q", pkgAfterSecond.Status)
	}
	if !strings.Contains(pkgAfterSecond.Error, "required job smoke failed") {
		t.Fatalf("unexpected package cancellation reason: %q", pkgAfterSecond.Error)
	}
}
