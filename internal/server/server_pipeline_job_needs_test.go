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

func TestPipelineJobNeedsBlocksUntilUpstreamSucceeds(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &stateStore{db: db}

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
`), "needs-block")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	p, err := db.GetPipelineByProjectAndID("ciwi", "build")
	if err != nil {
		t.Fatalf("get pipeline: %v", err)
	}
	if _, err := s.enqueuePersistedPipeline(p, nil); err != nil {
		t.Fatalf("enqueue pipeline: %v", err)
	}

	leased, err := db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease upstream job: %v", err)
	}
	if leased == nil || strings.TrimSpace(leased.Metadata["pipeline_job_id"]) != "smoke" {
		t.Fatalf("expected smoke job to lease first, got %+v", leased)
	}
	updated, err := db.UpdateJobExecutionStatus(leased.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusSucceeded,
		Output:       "ok",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark upstream succeeded: %v", err)
	}
	s.onJobExecutionUpdated(updated)

	next, err := db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease dependent job: %v", err)
	}
	if next == nil || strings.TrimSpace(next.Metadata["pipeline_job_id"]) != "package" {
		t.Fatalf("expected package job after upstream success, got %+v", next)
	}
}

func TestPipelineJobNeedsCancelsDependentJobsOnUpstreamFailure(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &stateStore{db: db}

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
`), "needs-cancel")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	p, err := db.GetPipelineByProjectAndID("ciwi", "build")
	if err != nil {
		t.Fatalf("get pipeline: %v", err)
	}
	if _, err := s.enqueuePersistedPipeline(p, nil); err != nil {
		t.Fatalf("enqueue pipeline: %v", err)
	}

	leased, err := db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease upstream job: %v", err)
	}
	if leased == nil || strings.TrimSpace(leased.Metadata["pipeline_job_id"]) != "smoke" {
		t.Fatalf("expected smoke job to lease first, got %+v", leased)
	}
	updated, err := db.UpdateJobExecutionStatus(leased.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusFailed,
		Error:        "boom",
		Output:       "boom",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark upstream failed: %v", err)
	}
	s.onJobExecutionUpdated(updated)

	jobs, err := db.ListJobExecutions()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for _, j := range jobs {
		if strings.TrimSpace(j.Metadata["pipeline_job_id"]) != "package" {
			continue
		}
		if protocol.NormalizeJobExecutionStatus(j.Status) != protocol.JobExecutionStatusFailed {
			t.Fatalf("expected dependent job to fail, got status=%q", j.Status)
		}
		if !strings.Contains(j.Error, "required job smoke failed") {
			t.Fatalf("unexpected dependent job error: %q", j.Error)
		}
		return
	}
	t.Fatalf("missing dependent package job")
}
