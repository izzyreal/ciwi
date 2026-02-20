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

func openPipelineChainRuntimeStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func enqueueSingleChain(t *testing.T, s *stateStore, yaml string) {
	t.Helper()
	cfg, err := config.Parse([]byte(yaml), "chain-runtime")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	project, err := s.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	detail, err := s.db.GetProjectDetail(project.ID)
	if err != nil {
		t.Fatalf("get project detail: %v", err)
	}
	if len(detail.PipelineChains) != 1 {
		t.Fatalf("expected exactly one chain, got %+v", detail.PipelineChains)
	}
	ch, err := s.db.GetPipelineChainByDBID(detail.PipelineChains[0].ID)
	if err != nil {
		t.Fatalf("get pipeline chain by id: %v", err)
	}
	if _, err := s.enqueuePersistedPipelineChain(ch, nil); err != nil {
		t.Fatalf("enqueue chain: %v", err)
	}
}

func findPipelineJobExecution(t *testing.T, s *stateStore, pipelineID string) protocol.JobExecution {
	t.Helper()
	jobs, err := s.db.ListJobExecutions()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for _, j := range jobs {
		if strings.TrimSpace(j.Metadata["pipeline_id"]) == pipelineID {
			return j
		}
	}
	t.Fatalf("missing queued job for pipeline %q", pipelineID)
	return protocol.JobExecution{}
}

func TestPipelineChainUnblocksNextPipelineOnSuccess(t *testing.T) {
	s := &stateStore{db: openPipelineChainRuntimeStore(t)}
	enqueueSingleChain(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: compile
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo build
  - id: package
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: pkg
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo package
pipeline_chains:
  - id: build-package
    pipelines:
      - build
      - package
`)

	secondBefore := findPipelineJobExecution(t, s, "package")
	if strings.TrimSpace(secondBefore.Metadata["chain_blocked"]) != "1" {
		t.Fatalf("expected second pipeline to be chain-blocked initially, metadata=%v", secondBefore.Metadata)
	}

	leased, err := s.db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease first job: %v", err)
	}
	if leased == nil || strings.TrimSpace(leased.Metadata["pipeline_id"]) != "build" {
		t.Fatalf("expected build job lease, got %+v", leased)
	}
	updated, err := s.db.UpdateJobExecutionStatus(leased.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusSucceeded,
		Output:       "ok",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark build job succeeded: %v", err)
	}
	s.onJobExecutionUpdated(updated)

	secondAfter, err := s.db.GetJobExecution(secondBefore.ID)
	if err != nil {
		t.Fatalf("get second job after unblock: %v", err)
	}
	if strings.TrimSpace(secondAfter.Metadata["chain_blocked"]) != "" {
		t.Fatalf("expected chain_blocked to clear after upstream success, metadata=%v", secondAfter.Metadata)
	}
	if protocol.NormalizeJobExecutionStatus(secondAfter.Status) != protocol.JobExecutionStatusQueued {
		t.Fatalf("expected second job to remain queued, got status=%q", secondAfter.Status)
	}
}

func TestPipelineChainCancelsNextPipelineOnFailure(t *testing.T) {
	s := &stateStore{db: openPipelineChainRuntimeStore(t)}
	enqueueSingleChain(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: compile
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo build
  - id: package
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: pkg
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo package
pipeline_chains:
  - id: build-package
    pipelines:
      - build
      - package
`)

	second := findPipelineJobExecution(t, s, "package")
	leased, err := s.db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease first job: %v", err)
	}
	updated, err := s.db.UpdateJobExecutionStatus(leased.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusFailed,
		Error:        "boom",
		Output:       "boom",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark build job failed: %v", err)
	}
	s.onJobExecutionUpdated(updated)

	secondAfter, err := s.db.GetJobExecution(second.ID)
	if err != nil {
		t.Fatalf("get second job after cancellation: %v", err)
	}
	if protocol.NormalizeJobExecutionStatus(secondAfter.Status) != protocol.JobExecutionStatusFailed {
		t.Fatalf("expected second job to fail after upstream failure, got %q", secondAfter.Status)
	}
	if strings.TrimSpace(secondAfter.Metadata["chain_cancelled"]) != "1" {
		t.Fatalf("expected chain_cancelled metadata on second job, metadata=%v", secondAfter.Metadata)
	}
	if !strings.Contains(secondAfter.Error, "upstream pipeline build failed") {
		t.Fatalf("unexpected cancellation reason: %q", secondAfter.Error)
	}
}

func TestPipelineChainDependencyBindFailureCancelsBlockedJobs(t *testing.T) {
	s := &stateStore{db: openPipelineChainRuntimeStore(t)}
	enqueueSingleChain(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: compile
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo build
  - id: package
    depends_on:
      - publish
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: pkg
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo package
  - id: publish
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: pub
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo publish
pipeline_chains:
  - id: build-package
    pipelines:
      - build
      - package
`)

	second := findPipelineJobExecution(t, s, "package")
	leased, err := s.db.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease first job: %v", err)
	}
	updated, err := s.db.UpdateJobExecutionStatus(leased.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "agent-1",
		Status:       protocol.JobExecutionStatusSucceeded,
		Output:       "ok",
		TimestampUTC: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mark build job succeeded: %v", err)
	}
	s.onJobExecutionUpdated(updated)

	secondAfter, err := s.db.GetJobExecution(second.ID)
	if err != nil {
		t.Fatalf("get second job after bind failure cancellation: %v", err)
	}
	if protocol.NormalizeJobExecutionStatus(secondAfter.Status) != protocol.JobExecutionStatusFailed {
		t.Fatalf("expected second job failure on dependency bind error, got %q", secondAfter.Status)
	}
	if strings.TrimSpace(secondAfter.Metadata["chain_cancelled"]) != "1" {
		t.Fatalf("expected chain_cancelled metadata after bind failure, metadata=%v", secondAfter.Metadata)
	}
	if !strings.Contains(secondAfter.Error, "dependency") {
		t.Fatalf("unexpected bind failure reason: %q", secondAfter.Error)
	}
}
