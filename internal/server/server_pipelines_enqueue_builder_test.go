package server

import (
	"os"
	"os/exec"
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

func TestEnqueuePersistedPipelineDryRunAllStepsSkippedUsesPlaceholderScript(t *testing.T) {
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
          - run: echo publish
            skip_dry_run: true
          - run: echo upload
            skip_dry_run: true
`), "dryrun-all-skipped")

	resp, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		PipelineJobID: "publish",
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("enqueue pipeline dry run all skipped: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}

	job, err := s.db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get enqueued job: %v", err)
	}
	if got := strings.TrimSpace(job.Script); got != "echo [dry-run] all steps skipped" {
		t.Fatalf("expected placeholder script for all-skipped dry-run job, got %q", job.Script)
	}
	if len(job.StepPlan) != 2 {
		t.Fatalf("expected 2 dryrun_skip steps, got %d", len(job.StepPlan))
	}
	if strings.TrimSpace(job.StepPlan[0].Kind) != "dryrun_skip" || strings.TrimSpace(job.StepPlan[1].Kind) != "dryrun_skip" {
		t.Fatalf("expected all step plan items dryrun_skip, got %+v", job.StepPlan)
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

func TestEnqueuePersistedPipelineDependencyVersionFromOtherRepoDoesNotOverrideSourceRef(t *testing.T) {
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
    vcs_source:
      repo: https://github.com/acme/source-a.git
      ref: refs/heads/main
    jobs:
      - id: build-job
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo build
  - id: package
    depends_on:
      - build
    vcs_source:
      repo: https://github.com/acme/source-b.git
      ref: refs/heads/release
    jobs:
      - id: package-job
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo package
`), "cross-repo-dep")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}

	buildExec, err := db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo build",
		TimeoutSeconds: 30,
		Metadata: map[string]string{
			"project":                      "ciwi",
			"pipeline_id":                  "build",
			"pipeline_run_id":              "run-build-1",
			"pipeline_version_raw":         "1.2.3",
			"pipeline_version":             "v1.2.3",
			"pipeline_source_repo":         "https://github.com/acme/source-a.git",
			"pipeline_source_ref_resolved": "d1c73be0f6f2335a3f16a6f706b08755b71c5d9c",
		},
	})
	if err != nil {
		t.Fatalf("create build execution: %v", err)
	}
	if _, err := db.UpdateJobExecutionStatus(buildExec.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("mark build succeeded: %v", err)
	}

	p, err := db.GetPipelineByProjectAndID("ciwi", "package")
	if err != nil {
		t.Fatalf("get package pipeline: %v", err)
	}
	resp, err := s.enqueuePersistedPipeline(p, nil)
	if err != nil {
		t.Fatalf("enqueue package: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one package execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}
	job, err := db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get package execution: %v", err)
	}
	if job.Source == nil {
		t.Fatalf("expected package source to be set")
	}
	if got := strings.TrimSpace(job.Source.Repo); got != "https://github.com/acme/source-b.git" {
		t.Fatalf("unexpected source repo: %q", got)
	}
	if got := strings.TrimSpace(job.Source.Ref); got != "refs/heads/release" {
		t.Fatalf("expected package source ref to remain pipeline ref, got %q", got)
	}
	if got := strings.TrimSpace(job.Metadata["pipeline_source_ref_resolved"]); got != "" {
		t.Fatalf("expected no resolved source ref metadata for cross-repo dependency inheritance, got %q", got)
	}
	if got := strings.TrimSpace(job.Env["CIWI_PIPELINE_SOURCE_REF"]); got != "" {
		t.Fatalf("expected no CIWI_PIPELINE_SOURCE_REF for cross-repo dependency inheritance, got %q", got)
	}
}

func TestEnqueuePersistedPipelineWithSourceRefOverrideResolvesCommit(t *testing.T) {
	repoURL, featureRef, featureSHA := createTestRemoteGitRepo(t)
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    vcs_source:
      repo: `+repoURL+`
      ref: refs/heads/main
    jobs:
      - id: compile
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo compile
`), "source-ref-override")

	resp, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		SourceRef: featureRef,
	})
	if err != nil {
		t.Fatalf("enqueue pipeline with source_ref override: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}
	job, err := s.db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get job execution: %v", err)
	}
	if job.Source == nil {
		t.Fatalf("expected source to be set")
	}
	if got := strings.TrimSpace(job.Source.Ref); got != featureSHA {
		t.Fatalf("expected source ref to be resolved sha %q, got %q", featureSHA, got)
	}
	if got := strings.TrimSpace(job.Metadata["pipeline_source_ref_raw"]); got != featureRef {
		t.Fatalf("expected pipeline_source_ref_raw %q, got %q", featureRef, got)
	}
	if got := strings.TrimSpace(job.Metadata["pipeline_source_ref_resolved"]); got != featureSHA {
		t.Fatalf("expected pipeline_source_ref_resolved %q, got %q", featureSHA, got)
	}
	if got := strings.TrimSpace(job.Env["CIWI_PIPELINE_SOURCE_REF_RAW"]); got != featureRef {
		t.Fatalf("expected CIWI_PIPELINE_SOURCE_REF_RAW %q, got %q", featureRef, got)
	}
	if got := strings.TrimSpace(job.Env["CIWI_PIPELINE_SOURCE_REF"]); got != featureSHA {
		t.Fatalf("expected CIWI_PIPELINE_SOURCE_REF %q, got %q", featureSHA, got)
	}
}

func TestEnqueuePersistedPipelineWithSourceRefOverrideRequiresSourceRepo(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: no-source
    jobs:
      - id: build
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo hi
`), "source-ref-override-no-source")
	_, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		SourceRef: "refs/heads/main",
	})
	if err == nil {
		t.Fatalf("expected source_ref override to fail for pipeline without source repo")
	}
	if !strings.Contains(err.Error(), "source_ref override requires pipeline vcs_source.repo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnqueuePersistedPipelineSelectionAgentIDSetsRequiredCapability(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo hi
`), "selection-agent-id")
	resp, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		AgentID: "agent-linux-1",
	})
	if err != nil {
		t.Fatalf("enqueue with agent_id selection: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}
	job, err := s.db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if got := strings.TrimSpace(job.RequiredCapabilities["agent_id"]); got != "agent-linux-1" {
		t.Fatalf("expected required agent_id=agent-linux-1, got %q", got)
	}
}

func TestEnqueuePersistedPipelineOfflineCachedUsesPinnedCachedCommit(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    vcs_source:
      repo: https://github.com/acme/repo.git
      ref: refs/heads/main
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: publish
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo publish
`), "offline-cached-success")
	exec, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo previous",
		TimeoutSeconds: 30,
		Metadata: map[string]string{
			"project":                      "ciwi",
			"pipeline_id":                  "release",
			"pipeline_run_id":              "run-release-prev",
			"pipeline_version_raw":         "1.2.3",
			"pipeline_version":             "v1.2.3",
			"pipeline_source_repo":         "https://github.com/acme/repo.git",
			"pipeline_source_ref_raw":      "refs/heads/main",
			"pipeline_source_ref_resolved": "0123456789abcdef0123456789abcdef01234567",
		},
	})
	if err != nil {
		t.Fatalf("create previous execution: %v", err)
	}
	if _, err := s.db.UpdateJobExecutionStatus(exec.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("mark previous execution succeeded: %v", err)
	}
	resp, err := s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		ExecutionMode: executionModeOfflineCached,
	})
	if err != nil {
		t.Fatalf("enqueue offline_cached: %v", err)
	}
	if resp.Enqueued != 1 || len(resp.JobExecutionIDs) != 1 {
		t.Fatalf("expected one execution, got enqueued=%d ids=%d", resp.Enqueued, len(resp.JobExecutionIDs))
	}
	job, err := s.db.GetJobExecution(resp.JobExecutionIDs[0])
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if job.Source == nil {
		t.Fatalf("expected source to be set")
	}
	if got := strings.TrimSpace(job.Source.Ref); got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("expected pinned cached sha, got %q", got)
	}
}

func TestEnqueuePersistedPipelineOfflineCachedBlocksSkipDryRunStepsWhenNotDryRun(t *testing.T) {
	s, p := loadPipelineForEnqueueBuilderTest(t, []byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    vcs_source:
      repo: https://github.com/acme/repo.git
      ref: refs/heads/main
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: publish
        runs_on:
          os: linux
        timeout_seconds: 30
        steps:
          - run: echo safe
          - run: echo wet
            skip_dry_run: true
`), "offline-cached-guard")
	exec, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo previous",
		TimeoutSeconds: 30,
		Metadata: map[string]string{
			"project":                      "ciwi",
			"pipeline_id":                  "release",
			"pipeline_run_id":              "run-release-prev",
			"pipeline_version_raw":         "1.2.3",
			"pipeline_version":             "v1.2.3",
			"pipeline_source_repo":         "https://github.com/acme/repo.git",
			"pipeline_source_ref_raw":      "refs/heads/main",
			"pipeline_source_ref_resolved": "0123456789abcdef0123456789abcdef01234567",
		},
	})
	if err != nil {
		t.Fatalf("create previous execution: %v", err)
	}
	if _, err := s.db.UpdateJobExecutionStatus(exec.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("mark previous execution succeeded: %v", err)
	}
	_, err = s.enqueuePersistedPipeline(p, &protocol.RunPipelineSelectionRequest{
		ExecutionMode: executionModeOfflineCached,
		DryRun:        false,
	})
	if err == nil {
		t.Fatalf("expected offline_cached non-dry run to fail for skip_dry_run steps")
	}
	if !strings.Contains(err.Error(), "contains skip_dry_run step") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func createTestRemoteGitRepo(t *testing.T) (repoURL, featureRef, featureSHA string) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	runGit(t, "", "init", "--bare", remote)
	runGit(t, "", "clone", remote, work)
	runGit(t, work, "config", "user.name", "ciwi-test")
	runGit(t, work, "config", "user.email", "ciwi-test@local")
	runGit(t, work, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "main commit")
	runGit(t, work, "push", "-u", "origin", "main")
	runGit(t, work, "checkout", "-b", "feature/one-off")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write README feature: %v", err)
	}
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "feature commit")
	runGit(t, work, "push", "-u", "origin", "feature/one-off")
	featureSHA = strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	return remote, "refs/heads/feature/one-off", featureSHA
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
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
