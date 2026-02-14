package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestServerLeaseRejectsAgentWithActiveJob(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()
	createJob := func(id string) {
		resp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
			"script":                "echo " + id,
			"required_capabilities": map[string]any{"os": "linux"},
			"timeout_seconds":       30,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create job status=%d body=%s", resp.StatusCode, readBody(t, resp))
		}
		_ = readBody(t, resp)
	}
	createJob("a")
	createJob("b")

	lease1 := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-1",
		"capabilities": map[string]any{"os": "linux"},
	})
	if lease1.StatusCode != http.StatusOK {
		t.Fatalf("lease1 status=%d body=%s", lease1.StatusCode, readBody(t, lease1))
	}
	var l1 protocol.LeaseJobExecutionResponse
	decodeJSONBody(t, lease1, &l1)
	if !l1.Assigned || l1.JobExecution == nil {
		t.Fatalf("expected first lease assigned, got %+v", l1)
	}

	lease2 := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-1",
		"capabilities": map[string]any{"os": "linux"},
	})
	if lease2.StatusCode != http.StatusOK {
		t.Fatalf("lease2 status=%d body=%s", lease2.StatusCode, readBody(t, lease2))
	}
	var l2 protocol.LeaseJobExecutionResponse
	decodeJSONBody(t, lease2, &l2)
	if l2.Assigned {
		t.Fatalf("expected second lease to be rejected while active job exists")
	}
	if !strings.Contains(strings.ToLower(l2.Message), "active job") {
		t.Fatalf("expected active-job rejection message, got %q", l2.Message)
	}
}

func TestServerForceFailActiveJob(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()
	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
		"script":                "echo hi",
		"required_capabilities": map[string]any{},
		"timeout_seconds":       30,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create job status=%d body=%s", createResp.StatusCode, readBody(t, createResp))
	}
	var createPayload struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, createResp, &createPayload)
	jobID := createPayload.Job.ID
	if jobID == "" {
		t.Fatalf("missing created job id")
	}

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+jobID+"/status", map[string]any{
		"agent_id": "agent-test",
		"status":   "running",
		"output":   "still running",
	})
	if runResp.StatusCode != http.StatusOK {
		t.Fatalf("mark running status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	ffResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+jobID+"/force-fail", map[string]any{})
	if ffResp.StatusCode != http.StatusOK {
		t.Fatalf("force-fail status=%d body=%s", ffResp.StatusCode, readBody(t, ffResp))
	}
	var ffPayload struct {
		Job struct {
			Status string `json:"status"`
			Error  string `json:"error"`
			Output string `json:"output"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, ffResp, &ffPayload)
	if ffPayload.Job.Status != "failed" {
		t.Fatalf("expected failed status, got %q", ffPayload.Job.Status)
	}
	if !strings.Contains(ffPayload.Job.Error, "force-failed") {
		t.Fatalf("expected force-failed error, got %q", ffPayload.Job.Error)
	}
	if !strings.Contains(ffPayload.Job.Output, "[control] job force-failed from UI") {
		t.Fatalf("expected control marker in output, got %q", ffPayload.Job.Output)
	}
}

func TestServerRunPipelineDryRunSetsMetadata(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(testConfigYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{
		"dry_run": true,
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		JobExecutions []struct {
			Metadata map[string]string `json:"metadata"`
			Env      map[string]string `json:"env"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.JobExecutions) == 0 {
		t.Fatalf("expected at least one job execution")
	}
	if jobsPayload.JobExecutions[0].Metadata["dry_run"] != "1" {
		t.Fatalf("expected metadata dry_run=1, got %q", jobsPayload.JobExecutions[0].Metadata["dry_run"])
	}
	if jobsPayload.JobExecutions[0].Env["CIWI_DRY_RUN"] != "1" {
		t.Fatalf("expected env CIWI_DRY_RUN=1, got %q", jobsPayload.JobExecutions[0].Env["CIWI_DRY_RUN"])
	}
}

func TestServerRunPipelineInjectsStepMarkers(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(testConfigYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		JobExecutions []struct {
			Script   string `json:"script"`
			StepPlan []struct {
				Index int    `json:"index"`
				Total int    `json:"total"`
				Name  string `json:"name"`
			} `json:"step_plan"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.JobExecutions) == 0 {
		t.Fatalf("expected at least one job execution")
	}
	if strings.Contains(jobsPayload.JobExecutions[0].Script, "STEP_BEGIN") {
		t.Fatalf("expected script to exclude legacy step marker, got:\n%s", jobsPayload.JobExecutions[0].Script)
	}
	if len(jobsPayload.JobExecutions[0].StepPlan) == 0 {
		t.Fatalf("expected step plan to be populated")
	}
}

func TestServerRunPipelineCmdTestStepCarriesReportMetadata(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	const cfg = `
version: 1
project:
  name: test
pipelines:
  - id: p1
    trigger: manual
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: win-tests
        runs_on:
          executor: script
          shell: cmd
        steps:
          - test:
              name: windows-suite
              command: ctest --output-on-failure --output-junit test-results.xml
              format: junit-xml
              report: test-results.xml
`

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		JobExecutions []struct {
			StepPlan []struct {
				Kind       string `json:"kind"`
				TestFormat string `json:"test_format"`
				TestReport string `json:"test_report"`
			} `json:"step_plan"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.JobExecutions) == 0 {
		t.Fatalf("expected at least one job execution")
	}
	if len(jobsPayload.JobExecutions[0].StepPlan) == 0 {
		t.Fatalf("expected step plan to be populated")
	}
	step := jobsPayload.JobExecutions[0].StepPlan[0]
	if step.Kind != "test" {
		t.Fatalf("expected test step kind, got %q", step.Kind)
	}
	if step.TestFormat != "junit-xml" {
		t.Fatalf("expected step plan to include test format metadata, got %q", step.TestFormat)
	}
	if step.TestReport != "test-results.xml" {
		t.Fatalf("expected step plan to include test report metadata, got %q", step.TestReport)
	}
}

func TestServerLoadConfigRejectsMetadataStep(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	const cfg = `
version: 1
project:
  name: test
pipelines:
  - id: release
    trigger: manual
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: invalid
        runs_on:
          executor: script
          shell: posix
        steps:
          - metadata:
              version: "{{ciwi.version_raw}}"
`
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected config load rejection, status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	if body := readBody(t, loadResp); !strings.Contains(body, "field metadata not found") {
		t.Fatalf("expected metadata parse error, got: %s", body)
	}
}

func TestServerRunPipelineSkipsMarkedStepsDuringDryRun(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	const cfg = `
version: 1
project:
  name: test
pipelines:
  - id: release
    trigger: manual
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: publish
        runs_on:
          executor: script
          shell: posix
        steps:
          - run: echo always
          - run: echo live-only
            skip_dry_run: true
`
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{"dry_run": true})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		JobExecutions []struct {
			StepPlan []struct {
				Script string `json:"script"`
				Kind   string `json:"kind"`
			} `json:"step_plan"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.JobExecutions) == 0 {
		t.Fatalf("expected at least one job execution")
	}
	plan := jobsPayload.JobExecutions[0].StepPlan
	if len(plan) != 2 {
		t.Fatalf("expected two steps during dry-run (including skip note), got %d", len(plan))
	}
	if strings.TrimSpace(plan[0].Script) != "echo always" {
		t.Fatalf("unexpected dry-run step script: %q", plan[0].Script)
	}
	if plan[1].Kind != "dryrun_skip" {
		t.Fatalf("expected second step kind dryrun_skip, got %q", plan[1].Kind)
	}
}

func TestServerRunPipelineDryRunDoesNotRequireSecretsFromSkippedSteps(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	const cfg = `
version: 1
project:
  name: release
  vault:
    connection: home-vault
    secrets:
      - name: github-secret
        path: ciwi
        key: token
pipelines:
  - id: release
    trigger: manual
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: publish
        runs_on:
          executor: script
          shell: posix
        steps:
          - run: echo always
          - run: echo live-only
            skip_dry_run: true
            env:
              GITHUB_TOKEN: "{{ secret.github-secret }}"
`

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{"dry_run": true})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	leaseResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-test",
		"capabilities": map[string]string{"executor": "script", "shells": "posix"},
	})
	if leaseResp.StatusCode != http.StatusOK {
		t.Fatalf("lease status=%d body=%s", leaseResp.StatusCode, readBody(t, leaseResp))
	}
	var payload struct {
		Assigned     bool `json:"assigned"`
		JobExecution struct {
			Env map[string]string `json:"env"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, leaseResp, &payload)
	if !payload.Assigned {
		t.Fatalf("expected dry-run job to lease successfully")
	}
	if _, exists := payload.JobExecution.Env["GITHUB_TOKEN"]; exists {
		t.Fatalf("expected skipped-step secret env to be absent from leased job")
	}
}

func TestServerRunPipelineInjectsDependencyArtifactJobID(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	const cfg = `
version: 1
project:
  name: dep-artifacts
pipelines:
  - id: build
    trigger: manual
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: linux-amd64
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        artifacts:
          - dist/**
        steps:
          - run: mkdir -p dist
          - run: echo ok > dist/app-linux-amd64
  - id: release
    trigger: manual
    depends_on:
      - build
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: release-linux-amd64
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        steps:
          - run: test -n "$CIWI_DEP_ARTIFACT_JOB_ID"
`
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	buildResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{})
	if buildResp.StatusCode != http.StatusCreated {
		t.Fatalf("build run status=%d body=%s", buildResp.StatusCode, readBody(t, buildResp))
	}
	_ = readBody(t, buildResp)

	leaseBuild := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-build",
		"capabilities": map[string]string{"executor": "script", "shells": "posix"},
	})
	if leaseBuild.StatusCode != http.StatusOK {
		t.Fatalf("lease build status=%d body=%s", leaseBuild.StatusCode, readBody(t, leaseBuild))
	}
	var leasedBuild struct {
		Assigned     bool `json:"assigned"`
		JobExecution struct {
			ID string `json:"id"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, leaseBuild, &leasedBuild)
	if !leasedBuild.Assigned || strings.TrimSpace(leasedBuild.JobExecution.ID) == "" {
		t.Fatalf("expected build job lease")
	}
	doneResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+leasedBuild.JobExecution.ID+"/status", map[string]any{
		"agent_id":  "agent-build",
		"status":    "succeeded",
		"exit_code": 0,
		"output":    "[run] done",
	})
	if doneResp.StatusCode != http.StatusOK {
		t.Fatalf("complete build status=%d body=%s", doneResp.StatusCode, readBody(t, doneResp))
	}
	_ = readBody(t, doneResp)

	releaseResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/2/run", map[string]any{})
	if releaseResp.StatusCode != http.StatusCreated {
		t.Fatalf("release run status=%d body=%s", releaseResp.StatusCode, readBody(t, releaseResp))
	}
	_ = readBody(t, releaseResp)

	leaseRelease := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-release",
		"capabilities": map[string]string{"executor": "script", "shells": "posix"},
	})
	if leaseRelease.StatusCode != http.StatusOK {
		t.Fatalf("lease release status=%d body=%s", leaseRelease.StatusCode, readBody(t, leaseRelease))
	}
	var leasedRelease struct {
		Assigned     bool `json:"assigned"`
		JobExecution struct {
			Env map[string]string `json:"env"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, leaseRelease, &leasedRelease)
	if !leasedRelease.Assigned {
		t.Fatalf("expected release job lease")
	}
	if strings.TrimSpace(leasedRelease.JobExecution.Env["CIWI_DEP_ARTIFACT_JOB_ID"]) == "" {
		t.Fatalf("expected CIWI_DEP_ARTIFACT_JOB_ID env on dependent release job")
	}
	if strings.TrimSpace(leasedRelease.JobExecution.Env["CIWI_DEP_ARTIFACT_JOB_IDS"]) == "" {
		t.Fatalf("expected CIWI_DEP_ARTIFACT_JOB_IDS env on dependent release job")
	}
}

func TestPipelineChainBlocksAndCancelsDownstreamOnFailure(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	const cfg = `
version: 1
project:
  name: chain-test
pipelines:
  - id: build
    trigger: manual
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: build-job
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        steps:
          - run: echo build
  - id: release
    trigger: manual
    source:
      repo: https://example.com/repo.git
    jobs:
      - id: release-job
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        steps:
          - run: echo release
pipeline_chains:
  - id: build-release
    pipelines:
      - build
      - release
`
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ciwi.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	client := ts.Client()
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	projectsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if projectsResp.StatusCode != http.StatusOK {
		t.Fatalf("projects status=%d body=%s", projectsResp.StatusCode, readBody(t, projectsResp))
	}
	var projectsPayload struct {
		Projects []struct {
			PipelineChains []struct {
				ID int64 `json:"id"`
			} `json:"pipeline_chains"`
		} `json:"projects"`
	}
	decodeJSONBody(t, projectsResp, &projectsPayload)
	if len(projectsPayload.Projects) == 0 || len(projectsPayload.Projects[0].PipelineChains) == 0 {
		t.Fatalf("expected pipeline chain in projects response")
	}
	chainID := projectsPayload.Projects[0].PipelineChains[0].ID

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/run", map[string]any{})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("chain run status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		JobExecutionIDs []string `json:"job_execution_ids"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if len(runPayload.JobExecutionIDs) < 2 {
		t.Fatalf("expected at least two enqueued jobs, got %d", len(runPayload.JobExecutionIDs))
	}

	leaseBuild := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-build",
		"capabilities": map[string]string{"executor": "script", "shells": "posix"},
	})
	if leaseBuild.StatusCode != http.StatusOK {
		t.Fatalf("lease build status=%d body=%s", leaseBuild.StatusCode, readBody(t, leaseBuild))
	}
	var leaseBuildPayload struct {
		Assigned     bool `json:"assigned"`
		JobExecution struct {
			ID string `json:"id"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, leaseBuild, &leaseBuildPayload)
	if !leaseBuildPayload.Assigned {
		t.Fatalf("expected first chain pipeline job to be leaseable")
	}

	leaseBlocked := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-release",
		"capabilities": map[string]string{"executor": "script", "shells": "posix"},
	})
	if leaseBlocked.StatusCode != http.StatusOK {
		t.Fatalf("lease blocked status=%d body=%s", leaseBlocked.StatusCode, readBody(t, leaseBlocked))
	}
	var leaseBlockedPayload struct {
		Assigned bool `json:"assigned"`
	}
	decodeJSONBody(t, leaseBlocked, &leaseBlockedPayload)
	if leaseBlockedPayload.Assigned {
		t.Fatalf("expected downstream chain job to remain blocked before upstream completion")
	}

	failResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+leaseBuildPayload.JobExecution.ID+"/status", map[string]any{
		"agent_id": "agent-build",
		"status":   "failed",
		"error":    "build failed",
		"output":   "boom",
	})
	if failResp.StatusCode != http.StatusOK {
		t.Fatalf("build fail status=%d body=%s", failResp.StatusCode, readBody(t, failResp))
	}
	_ = readBody(t, failResp)

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		JobExecutions []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	seenCancelled := false
	for _, j := range jobsPayload.JobExecutions {
		if strings.Contains(j.Error, "cancelled: upstream pipeline build failed") {
			if protocol.NormalizeJobExecutionStatus(j.Status) != protocol.JobExecutionStatusFailed {
				t.Fatalf("expected cancelled downstream job to be failed, got %q", j.Status)
			}
			seenCancelled = true
		}
	}
	if !seenCancelled {
		t.Fatalf("expected at least one downstream cancelled chain job")
	}
}
