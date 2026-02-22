package store

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

const testConfigYAML = `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
      ref: main
    jobs:
      - id: compile
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 300
        artifacts:
          - dist/*
        caches:
          - id: fetchcontent
            env: CIWI_FETCHCONTENT_SOURCES_DIR
        matrix:
          include:
            - name: linux-amd64
              goos: linux
              goarch: amd64
            - name: windows-amd64
              goos: windows
              goarch: amd64
        steps:
          - run: mkdir -p dist
          - run: GOOS={{goos}} GOARCH={{goarch}} go build -o dist/ciwi-{{goos}}-{{goarch}} ./cmd/ciwi
`

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ciwi-test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

func TestListJobExecutionsDeterministicOrderWhenTimestampsTie(t *testing.T) {
	s := openTestStore(t)

	createdIDs := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
			Script:         "echo test",
			TimeoutSeconds: 30,
		})
		if err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
		createdIDs = append(createdIDs, job.ID)
	}

	if _, err := s.db.Exec(`UPDATE job_executions SET created_utc = ?`, "2026-02-15T00:00:00Z"); err != nil {
		t.Fatalf("set tied timestamps: %v", err)
	}

	jobs, err := s.ListJobExecutions()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != len(createdIDs) {
		t.Fatalf("expected %d jobs, got %d", len(createdIDs), len(jobs))
	}

	expected := append([]string(nil), createdIDs...)
	sort.Sort(sort.Reverse(sort.StringSlice(expected)))
	for i := range jobs {
		if jobs[i].ID != expected[i] {
			t.Fatalf("unexpected order at index %d: got %q want %q", i, jobs[i].ID, expected[i])
		}
	}
}

func parseTestConfig(t *testing.T) config.File {
	t.Helper()
	cfg, err := config.Parse([]byte(testConfigYAML), "test")
	if err != nil {
		t.Fatalf("parse test config: %v", err)
	}
	return cfg
}

func TestStoreLoadConfigAndProjectDetail(t *testing.T) {
	s := openTestStore(t)
	cfg := parseTestConfig(t)

	err := s.LoadConfig(
		cfg,
		"https://github.com/izzyreal/ciwi.git:ciwi-project.yaml",
		"https://github.com/izzyreal/ciwi.git",
		"main",
		"ciwi-project.yaml",
	)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	projects, err := s.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if got := projects[0].Name; got != "ciwi" {
		t.Fatalf("unexpected project name: %q", got)
	}
	if len(projects[0].Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(projects[0].Pipelines))
	}
	if got := projects[0].Pipelines[0].PipelineID; got != "build" {
		t.Fatalf("unexpected pipeline id: %q", got)
	}

	detail, err := s.GetProjectDetail(projects[0].ID)
	if err != nil {
		t.Fatalf("get project detail: %v", err)
	}
	if len(detail.Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline in detail, got %d", len(detail.Pipelines))
	}
	if len(detail.Pipelines[0].Jobs) != 1 {
		t.Fatalf("expected 1 pipeline job, got %d", len(detail.Pipelines[0].Jobs))
	}
	if got := detail.Pipelines[0].Jobs[0].ID; got != "compile" {
		t.Fatalf("unexpected pipeline job id: %q", got)
	}
	if got := len(detail.Pipelines[0].Jobs[0].Caches); got != 1 {
		t.Fatalf("expected 1 cache in pipeline job detail, got %d", got)
	}
	if got := detail.Pipelines[0].Jobs[0].Caches[0].ID; got != "fetchcontent" {
		t.Fatalf("unexpected cache id: %q", got)
	}
	if got := len(detail.Pipelines[0].Jobs[0].MatrixIncludes); got != 2 {
		t.Fatalf("expected 2 matrix includes, got %d", got)
	}
}

func TestStoreLoadConfigExpandsManagedGoCaches(t *testing.T) {
	s := openTestStore(t)
	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: compile
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 120
        go_cache: {}
        caches:
          - id: fetchcontent
            env: FETCHCONTENT_BASE_DIR
        steps:
          - run: go build ./...
`), "go-cache-store")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if err := s.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}

	projects, err := s.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	detail, err := s.GetProjectDetail(projects[0].ID)
	if err != nil {
		t.Fatalf("get project detail: %v", err)
	}
	caches := detail.Pipelines[0].Jobs[0].Caches
	if len(caches) != 3 {
		t.Fatalf("expected 3 caches, got %d", len(caches))
	}
	if caches[1].ID != config.ManagedGoBuildCacheID || caches[1].Env != config.ManagedGoBuildCacheEnv {
		t.Fatalf("unexpected go build cache: %+v", caches[1])
	}
	if caches[2].ID != config.ManagedGoModCacheID || caches[2].Env != config.ManagedGoModCacheEnv {
		t.Fatalf("unexpected go mod cache: %+v", caches[2])
	}
}

func TestStoreLoadConfigReloadDoesNotAdvanceProjectIDSequence(t *testing.T) {
	s := openTestStore(t)
	ciwiCfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: unit
        timeout_seconds: 30
        steps:
          - run: go test ./...
`), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse ciwi config: %v", err)
	}
	for i := 0; i < 8; i++ {
		if err := s.LoadConfig(ciwiCfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
			t.Fatalf("load ciwi config (iteration %d): %v", i, err)
		}
	}
	ciwiProject, err := s.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("get ciwi project: %v", err)
	}
	if ciwiProject.ID != 1 {
		t.Fatalf("expected ciwi project id=1, got %d", ciwiProject.ID)
	}

	fooCfg, err := config.Parse([]byte(`
version: 1
project:
  name: foo
pipelines:
  - id: build
    jobs:
      - id: unit
        timeout_seconds: 30
        steps:
          - run: echo foo
`), "foo-project.yaml")
	if err != nil {
		t.Fatalf("parse foo config: %v", err)
	}
	if err := s.LoadConfig(fooCfg, "foo-project.yaml", "https://github.com/example/foo.git", "main", "foo-project.yaml"); err != nil {
		t.Fatalf("load foo config: %v", err)
	}
	fooProject, err := s.GetProjectByName("foo")
	if err != nil {
		t.Fatalf("get foo project: %v", err)
	}
	if fooProject.ID != 2 {
		t.Fatalf("expected foo project id=2 after ciwi reloads, got %d", fooProject.ID)
	}
}

func TestStorePersistsPipelineJobNeeds(t *testing.T) {
	s := openTestStore(t)
	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: smoke
        timeout_seconds: 30
        steps:
          - run: echo smoke
      - id: package
        needs:
          - smoke
        timeout_seconds: 30
        steps:
          - run: echo package
`), "needs-config")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := s.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	projects, err := s.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	detail, err := s.GetProjectDetail(projects[0].ID)
	if err != nil {
		t.Fatalf("get project detail: %v", err)
	}
	if len(detail.Pipelines) != 1 || len(detail.Pipelines[0].Jobs) != 2 {
		t.Fatalf("unexpected pipeline job count: %+v", detail.Pipelines)
	}
	if got := detail.Pipelines[0].Jobs[1].Needs; len(got) != 1 || got[0] != "smoke" {
		t.Fatalf("unexpected needs for package job: %+v", got)
	}
}

func TestStoreLoadConfigAppliesProjectVaultSettings(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.UpsertVaultConnection(protocol.UpsertVaultConnectionRequest{
		Name:         "home-vault",
		URL:          "http://vault.local:8200",
		AuthMethod:   "approle",
		AppRoleMount: "approle",
		RoleID:       "role-1",
		SecretIDEnv:  "CIWI_VAULT_SECRET_ID",
	}); err != nil {
		t.Fatalf("upsert vault connection: %v", err)
	}

	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: ciwi
  vault:
    connection: home-vault
    secrets:
      - name: github-secret
        mount: kv
        path: gh
        key: token
pipelines:
  - id: build
    jobs:
      - id: unit
        timeout_seconds: 60
        steps:
          - run: go test ./...
`), "vault-config")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if err := s.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}

	project, err := s.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	settings, err := s.GetProjectVaultSettings(project.ID)
	if err != nil {
		t.Fatalf("get project vault settings: %v", err)
	}
	if settings.VaultConnectionName != "home-vault" {
		t.Fatalf("expected connection name home-vault, got %q", settings.VaultConnectionName)
	}
	if settings.VaultConnectionID <= 0 {
		t.Fatalf("expected resolved connection id > 0, got %d", settings.VaultConnectionID)
	}
	if len(settings.Secrets) != 1 || settings.Secrets[0].Name != "github-secret" || settings.Secrets[0].Path != "gh" || settings.Secrets[0].Key != "token" {
		t.Fatalf("unexpected project vault secrets: %+v", settings.Secrets)
	}
}

func TestStoreJobQueueAndHistoryOperations(t *testing.T) {
	s := openTestStore(t)

	_, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo leased",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create leaseable job: %v", err)
	}

	jQueued, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "echo queued",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create queued job: %v", err)
	}

	leased, err := s.LeaseJobExecution("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease job: %v", err)
	}
	if leased == nil {
		t.Fatal("expected leased job, got nil")
	}

	jHistory, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo done",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create history job: %v", err)
	}

	_, err = s.UpdateJobExecutionStatus(jHistory.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "succeeded",
		Output:  "ok",
	})
	if err != nil {
		t.Fatalf("mark history job succeeded: %v", err)
	}

	if err := s.DeleteQueuedJobExecution(jHistory.ID); err == nil {
		t.Fatal("expected delete queued conflict for succeeded job")
	}

	if err := s.DeleteQueuedJobExecution(jQueued.ID); err != nil {
		t.Fatalf("delete queued job: %v", err)
	}

	cleared, err := s.ClearQueuedJobExecutions()
	if err != nil {
		t.Fatalf("clear queued jobs: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("expected clear queued count 1 (leased), got %d", cleared)
	}

	flushed, err := s.FlushJobExecutionHistory()
	if err != nil {
		t.Fatalf("flush history: %v", err)
	}
	if flushed != 1 {
		t.Fatalf("expected flush count 1, got %d", flushed)
	}

	jobs, err := s.ListJobExecutions()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no pending jobs after clear queue, got %d", len(jobs))
	}

	cleared, err = s.ClearQueuedJobExecutions()
	if err != nil {
		t.Fatalf("clear queued second pass: %v", err)
	}
	if cleared != 0 {
		t.Fatalf("expected clear queued count 0 on second pass, got %d", cleared)
	}

	jobs, err = s.ListJobExecutions()
	if err != nil {
		t.Fatalf("list jobs second pass: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no remaining jobs after second clear, got %d", len(jobs))
	}
}
