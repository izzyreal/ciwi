package store

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	if cleared != 1 {
		t.Fatalf("expected clear queued count 1 (leased), got %d", cleared)
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
	if len(jobs) != 0 {
		t.Fatalf("expected no pending jobs after clear queue, got %d", len(jobs))
	}

	cleared, err = s.ClearQueuedJobs()
	if err != nil {
		t.Fatalf("clear queued second pass: %v", err)
	}
	if cleared != 0 {
		t.Fatalf("expected clear queued count 0 on second pass, got %d", cleared)
	}

	jobs, err = s.ListJobs()
	if err != nil {
		t.Fatalf("list jobs second pass: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no remaining jobs after second clear, got %d", len(jobs))
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
			agentID := "agent-concurrent-" + string(rune('a'+(i%26)))
			for attempt := 0; attempt < 8; attempt++ {
				j, leaseErr := s.LeaseJob(agentID, map[string]string{"os": "linux", "arch": "amd64"})
				if leaseErr != nil {
					if strings.Contains(strings.ToLower(leaseErr.Error()), "database is locked") {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					t.Errorf("lease error: %v", leaseErr)
					return
				}
				if j != nil {
					atomic.AddInt32(&leasedCount, 1)
				}
				return
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

func TestCapabilitiesMatchToolConstraints(t *testing.T) {
	agentCaps := map[string]string{
		"os":         "linux",
		"arch":       "amd64",
		"executor":   "script",
		"shell":      "posix",
		"tool.go":    "1.25.7",
		"tool.git":   "2.44.0",
		"tool.cmake": "3.28.1",
	}
	req := map[string]string{
		"os":                  "linux",
		"requires.tool.go":    ">=1.24",
		"requires.tool.git":   ">=2.30",
		"requires.tool.cmake": "*",
		"requires.tool.clang": "",
	}
	if capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected missing clang tool to fail")
	}
	agentCaps["tool.clang"] = "17.0.1"
	if !capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected constraints to match")
	}
	req["requires.tool.go"] = ">1.26"
	if capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected go constraint >1.26 to fail")
	}
}

func TestCapabilitiesMatchShellsList(t *testing.T) {
	agentCaps := map[string]string{
		"os":       "windows",
		"arch":     "amd64",
		"executor": "script",
		"shell":    "cmd",
		"shells":   "cmd,powershell",
	}
	req := map[string]string{
		"os":       "windows",
		"executor": "script",
		"shell":    "powershell",
	}
	if !capabilitiesMatch(agentCaps, req) {
		t.Fatalf("expected shell requirement to match via shells list")
	}
}

func TestStoreSaveAndGetJobTestReport(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJob(protocol.CreateJobRequest{
		Script:         "echo tests",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	report := protocol.JobTestReport{
		Total:   2,
		Passed:  1,
		Failed:  1,
		Skipped: 0,
		Suites: []protocol.TestSuiteReport{
			{
				Name:    "go-unit",
				Format:  "go-test-json",
				Total:   2,
				Passed:  1,
				Failed:  1,
				Skipped: 0,
				Cases: []protocol.TestCase{
					{Package: "p", Name: "TestA", Status: "pass"},
					{Package: "p", Name: "TestB", Status: "fail"},
				},
			},
		},
	}
	if err := s.SaveJobTestReport(job.ID, report); err != nil {
		t.Fatalf("save test report: %v", err)
	}

	got, found, err := s.GetJobTestReport(job.ID)
	if err != nil {
		t.Fatalf("get test report: %v", err)
	}
	if !found {
		t.Fatal("expected test report to be found")
	}
	if got.Total != 2 || got.Failed != 1 || len(got.Suites) != 1 {
		t.Fatalf("unexpected test report: %+v", got)
	}
}

func TestStoreIgnoresLateRunningAfterSucceeded(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJob(protocol.CreateJobRequest{
		Script:         "echo done",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	done, err := s.UpdateJobStatus(job.ID, protocol.JobStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "succeeded",
		Output:  "final output",
	})
	if err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	if done.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %q", done.Status)
	}

	got, err := s.UpdateJobStatus(job.ID, protocol.JobStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "running",
		Output:  "late running output",
	})
	if err != nil {
		t.Fatalf("late running update: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("expected status to remain succeeded, got %q", got.Status)
	}
	if got.Output != "final output" {
		t.Fatalf("expected output to remain terminal output, got %q", got.Output)
	}
}

func TestStoreAgentHasActiveJob(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJob(protocol.CreateJobRequest{
		Script:               "echo hi",
		RequiredCapabilities: map[string]string{"os": "linux"},
		TimeoutSeconds:       30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	leased, err := s.LeaseJob("agent-a", map[string]string{"os": "linux"})
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if leased == nil || leased.ID != job.ID {
		t.Fatalf("expected leased job %q", job.ID)
	}

	active, err := s.AgentHasActiveJob("agent-a")
	if err != nil {
		t.Fatalf("AgentHasActiveJob leased: %v", err)
	}
	if !active {
		t.Fatalf("expected active job for agent-a")
	}

	if _, err := s.UpdateJobStatus(job.ID, protocol.JobStatusUpdateRequest{
		AgentID: "agent-a",
		Status:  "succeeded",
	}); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	active, err = s.AgentHasActiveJob("agent-a")
	if err != nil {
		t.Fatalf("AgentHasActiveJob succeeded: %v", err)
	}
	if active {
		t.Fatalf("expected no active job after succeeded")
	}
}

func TestStoreConcurrentRunningDoesNotOverrideTerminal(t *testing.T) {
	s := openTestStore(t)

	job, err := s.CreateJob(protocol.CreateJobRequest{
		Script:         "echo hi",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := s.UpdateJobStatus(job.ID, protocol.JobStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  "running",
		Output:  "stream-1",
	}); err != nil {
		t.Fatalf("mark running: %v", err)
	}

	exitCode := 0
	var wg sync.WaitGroup
	var errSucceeded atomic.Value
	var errRunning atomic.Value
	wg.Add(2)
	go func() {
		defer wg.Done()
		for attempt := 0; attempt < 8; attempt++ {
			_, uerr := s.UpdateJobStatus(job.ID, protocol.JobStatusUpdateRequest{
				AgentID:  "agent-1",
				Status:   "succeeded",
				ExitCode: &exitCode,
				Output:   "final",
			})
			if uerr != nil && strings.Contains(strings.ToLower(uerr.Error()), "database is locked") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if uerr != nil {
				errSucceeded.Store(uerr)
			}
			return
		}
		errSucceeded.Store("update retries exhausted due to database lock")
	}()
	go func() {
		defer wg.Done()
		for attempt := 0; attempt < 8; attempt++ {
			_, uerr := s.UpdateJobStatus(job.ID, protocol.JobStatusUpdateRequest{
				AgentID: "agent-1",
				Status:  "running",
				Output:  "late-stream",
			})
			if uerr != nil && strings.Contains(strings.ToLower(uerr.Error()), "database is locked") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if uerr != nil {
				errRunning.Store(uerr)
			}
			return
		}
		errRunning.Store("update retries exhausted due to database lock")
	}()
	wg.Wait()

	if v := errSucceeded.Load(); v != nil {
		t.Fatalf("concurrent succeeded update error: %v", v)
	}
	if v := errRunning.Load(); v != nil {
		t.Fatalf("concurrent running update error: %v", v)
	}

	got, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %q", got.Status)
	}
	if got.FinishedUTC.IsZero() {
		t.Fatalf("expected finished timestamp to be set")
	}
}
