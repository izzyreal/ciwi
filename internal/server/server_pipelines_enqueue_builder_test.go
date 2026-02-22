package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

func TestEnqueuePersistedPipelineSelectionMatrixNameFiltersEntries(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: build-matrix
        runs_on:
          os: linux
        matrix:
          include:
            - name: linux-amd64
            - name: linux-arm64
        timeout_seconds: 30
        steps:
          - run: echo {{name}}
`), "matrix-selection")

	resp, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		PipelineJobID: "build-matrix",
		MatrixName:    "linux-arm64",
	})
	if err != nil {
		t.Fatalf("enqueue pipeline: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one selected matrix execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}

	job, err := s.db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get enqueued job: %v", err)
	}
	if got := strings.TrimSpace(job.Metadata["matrix_name"]); got != "linux-arm64" {
		t.Fatalf("expected matrix_name linux-arm64, got %q", got)
	}
	if !strings.Contains(job.Script, "linux-arm64") {
		t.Fatalf("expected rendered script to include selected matrix value, script=%q", job.Script)
	}
}

func TestEnqueuePersistedPipelineSelectionRequiresNeededJob(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    vcs_source:
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
`), "selection-needs")

	_, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{PipelineJobID: "package"})
	if err == nil {
		t.Fatalf("expected selection to fail when required upstream job is excluded")
	}
	if !strings.Contains(err.Error(), `selection excludes required job "smoke" needed by "package"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnqueuePersistedPipelineDryRunSkipDryRunSteps(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: publish
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo build
          - run: echo publish
            skip_dry_run: true
`), "dryrun-skip")

	resp, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		PipelineJobID: "publish",
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("enqueue pipeline dry run: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}

	job, err := s.db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get enqueued job: %v", err)
	}
	if got := strings.TrimSpace(job.Metadata["dry_run"]); got != "1" {
		t.Fatalf("expected dry_run metadata=1, got %q", got)
	}
	if got := strings.TrimSpace(job.Env["CIWI_DRY_RUN"]); got != "1" {
		t.Fatalf("expected CIWI_DRY_RUN=1, got %q", got)
	}
	if strings.Contains(job.Script, "echo publish") {
		t.Fatalf("skip_dry_run step must not be present in rendered script: %q", job.Script)
	}
	if len(job.StepPlan) != 2 {
		t.Fatalf("expected 2 step plan items (run + dryrun_skip), got %d", len(job.StepPlan))
	}
	if got := strings.TrimSpace(job.StepPlan[1].Kind); got != "dryrun_skip" {
		t.Fatalf("expected second step kind=dryrun_skip, got %q", got)
	}
}

func TestEnqueuePersistedPipelineWithoutSourceCreatesArtifactOnlyJob(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: artifact-only
    jobs:
      - id: package
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo package
`), "artifact-only")

	resp, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{PipelineJobID: "package"})
	if err != nil {
		t.Fatalf("enqueue pipeline: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}

	job, err := s.db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get enqueued job: %v", err)
	}
	if job.Source != nil {
		t.Fatalf("expected no source for artifact-only pipeline job, got %+v", job.Source)
	}
}

func loadPipelineForEnqueueBuilderTest(t *testing.T, yaml []byte, source string) (*stateStore, store.PersistedPipeline) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &stateStore{db: db}

	cfg, err := config.Parse(yaml, source)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	p, err := db.GetPipelineByProjectAndID("ciwi", cfg.Pipelines[0].ID)
	if err != nil {
		t.Fatalf("get pipeline: %v", err)
	}
	return s, p
}
