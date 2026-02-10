package store

import (
	"path/filepath"
	"sync"
	"sync/atomic"
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
    source:
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
	if got := len(detail.Pipelines[0].Jobs[0].MatrixIncludes); got != 2 {
		t.Fatalf("expected 2 matrix includes, got %d", got)
	}
}

func TestStoreJobQueueAndHistoryOperations(t *testing.T) {
	s := openTestStore(t)

	_, err := s.CreateJob(protocol.CreateJobRequest{
		Script:               "echo leased",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create leaseable job: %v", err)
	}

	jQueued, err := s.CreateJob(protocol.CreateJobRequest{
		Script:               "echo queued",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create queued job: %v", err)
	}

	leased, err := s.LeaseJob("agent-1", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease job: %v", err)
	}
	if leased == nil {
		t.Fatal("expected leased job, got nil")
	}

	jHistory, err := s.CreateJob(protocol.CreateJobRequest{
		Script:         "echo done",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create history job: %v", err)
	}

	_, err = s.UpdateJobStatus(jHistory.ID, protocol.JobStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "succeeded",
		Output:  "ok",
	})
	if err != nil {
		t.Fatalf("mark history job succeeded: %v", err)
	}

	if err := s.DeleteQueuedJob(jHistory.ID); err == nil {
		t.Fatal("expected delete queued conflict for succeeded job")
	}

	if err := s.DeleteQueuedJob(jQueued.ID); err != nil {
		t.Fatalf("delete queued job: %v", err)
	}

	cleared, err := s.ClearQueuedJobs()
	if err != nil {
		t.Fatalf("clear queued jobs: %v", err)
	}
	if cleared != 0 {
		t.Fatalf("expected clear queued count 0, got %d", cleared)
	}

	flushed, err := s.FlushJobHistory()
	if err != nil {
		t.Fatalf("flush history: %v", err)
	}
	if flushed != 1 {
		t.Fatalf("expected flush count 1, got %d", flushed)
	}

	jobs, err := s.ListJobs()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 remaining job (leased), got %d", len(jobs))
	}
	if jobs[0].Status != "leased" {
		t.Fatalf("expected remaining job status leased, got %q", jobs[0].Status)
	}
}

func TestStoreLeaseJobConcurrencySingleWinner(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJob(protocol.CreateJobRequest{
		Script:               "echo hello",
		RequiredCapabilities: map[string]string{"os": "linux", "arch": "amd64"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	const workers = 24
	var leasedCount int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			j, leaseErr := s.LeaseJob(
				"agent-concurrent-"+string(rune('a'+(i%26))),
				map[string]string{"os": "linux", "arch": "amd64"},
			)
			if leaseErr != nil {
				t.Errorf("lease error: %v", leaseErr)
				return
			}
			if j != nil {
				atomic.AddInt32(&leasedCount, 1)
			}
		}(i)
	}
	wg.Wait()

	if leasedCount != 1 {
		t.Fatalf("expected exactly one lease winner, got %d", leasedCount)
	}

	got, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != "leased" {
		t.Fatalf("expected job status leased, got %q", got.Status)
	}
	if got.LeasedByAgentID == "" {
		t.Fatal("expected leased_by_agent_id to be set")
	}
}
