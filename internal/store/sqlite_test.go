package store

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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
          - name: Prepare dist
            run: mkdir -p dist
          - name: Build binary
            run: GOOS={{goos}} GOARCH={{goarch}} go build -o dist/ciwi-{{goos}}-{{goarch}} ./cmd/ciwi
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

func TestOpenDropsLegacyJobOutputColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ciwi.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open initial store: %v", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE job_executions ADD COLUMN output_text TEXT`); err != nil {
		t.Fatalf("add legacy output column: %v", err)
	}
	if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable foreign keys for legacy fixture: %v", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO job_execution_events (job_execution_id, event_type, timestamp_utc, payload_json, created_utc)
		VALUES ('missing-job', 'system.message', '2026-07-19T00:00:00Z', '{"message":"orphan"}', '2026-07-19T00:00:00Z')
	`); err != nil {
		t.Fatalf("insert orphaned legacy event: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close initial store: %v", err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer s.Close()
	var foreignKeys int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys setting: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign keys enabled, got %d", foreignKeys)
	}
	var orphanedEvents int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM job_execution_events WHERE job_execution_id = 'missing-job'`).Scan(&orphanedEvents); err != nil {
		t.Fatalf("count orphaned events: %v", err)
	}
	if orphanedEvents != 0 {
		t.Fatalf("expected orphaned events removed, got %d", orphanedEvents)
	}
	rows, err := s.db.Query(`PRAGMA table_info(job_executions)`)
	if err != nil {
		t.Fatalf("inspect job columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan job column: %v", err)
		}
		if name == "output_text" {
			t.Fatal("legacy output_text column was not dropped")
		}
	}
}

func TestFlushJobExecutionHistoryCompactsDatabase(t *testing.T) {
	s := openTestStore(t)
	job, err := s.CreateJobExecution(protocol.CreateJobExecutionRequest{Script: "echo done"})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := s.UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("finish job: %v", err)
	}
	if err := s.AppendJobExecutionEvents(job.ID, []protocol.JobExecutionEvent{{
		Type:         protocol.JobExecutionEventTypeSystemMessage,
		TimestampUTC: time.Now().UTC(),
		Message:      strings.Repeat("x", 1024*1024),
	}}); err != nil {
		t.Fatalf("append large event: %v", err)
	}
	var pagesBefore int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pagesBefore); err != nil {
		t.Fatalf("page count before flush: %v", err)
	}

	deleted, err := s.FlushJobExecutionHistory()
	if err != nil {
		t.Fatalf("flush history: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != job.ID {
		t.Fatalf("unexpected deleted jobs: %v", deleted)
	}
	var pagesAfter, freePagesAfter int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pagesAfter); err != nil {
		t.Fatalf("page count after flush: %v", err)
	}
	if err := s.db.QueryRow(`PRAGMA freelist_count`).Scan(&freePagesAfter); err != nil {
		t.Fatalf("freelist count after flush: %v", err)
	}
	if pagesAfter >= pagesBefore {
		t.Fatalf("expected compaction to reduce page count, before=%d after=%d", pagesBefore, pagesAfter)
	}
	if freePagesAfter != 0 {
		t.Fatalf("expected vacuumed freelist, got %d pages", freePagesAfter)
	}
	events, err := s.ListJobExecutionEvents(job.ID)
	if err != nil {
		t.Fatalf("list deleted job events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected cascading event deletion, got %d events", len(events))
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
	steps := detail.Pipelines[0].Jobs[0].Steps
	if len(steps) != 2 || steps[0].Name != "Prepare dist" || steps[1].Name != "Build binary" {
		t.Fatalf("expected step names in project detail, got %+v", steps)
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

func TestStoreLoadConfigPrunesRemovedPipelines(t *testing.T) {
	s := openTestStore(t)

	initial, err := config.Parse([]byte(`
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
          - run: echo build
  - id: package-sign
    jobs:
      - id: macos-codesign
        runs_on:
          os: darwin
        timeout_seconds: 30
        steps:
          - run: echo sign
`), "prune-initial")
	if err != nil {
		t.Fatalf("parse initial config: %v", err)
	}
	if err := s.LoadConfig(initial, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load initial config: %v", err)
	}

	reloaded, err := config.Parse([]byte(`
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
          - run: echo build
`), "prune-reloaded")
	if err != nil {
		t.Fatalf("parse reloaded config: %v", err)
	}
	if err := s.LoadConfig(reloaded, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load reloaded config: %v", err)
	}

	project, err := s.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("get project by name: %v", err)
	}
	detail, err := s.GetProjectDetail(project.ID)
	if err != nil {
		t.Fatalf("get project detail: %v", err)
	}
	if len(detail.Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline after prune, got %d", len(detail.Pipelines))
	}
	if got := detail.Pipelines[0].PipelineID; got != "build" {
		t.Fatalf("unexpected remaining pipeline: %q", got)
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

func TestStoreLoadConfigRejectsDeprecatedProjectVaultConfig(t *testing.T) {
	_, err := config.Parse([]byte(`
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
	if err == nil {
		t.Fatalf("expected parse error for legacy project.vault settings")
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
	if len(flushed) != 1 {
		t.Fatalf("expected flush count 1, got %d", len(flushed))
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
