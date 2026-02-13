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
	var l1 protocol.LeaseJobResponse
	decodeJSONBody(t, lease1, &l1)
	if !l1.Assigned || l1.Job == nil {
		t.Fatalf("expected first lease assigned, got %+v", l1)
	}

	lease2 := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-1",
		"capabilities": map[string]any{"os": "linux"},
	})
	if lease2.StatusCode != http.StatusOK {
		t.Fatalf("lease2 status=%d body=%s", lease2.StatusCode, readBody(t, lease2))
	}
	var l2 protocol.LeaseJobResponse
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
		} `json:"job"`
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
		} `json:"job"`
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
			Script string `json:"script"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.JobExecutions) == 0 {
		t.Fatalf("expected at least one job execution")
	}
	if !strings.Contains(jobsPayload.JobExecutions[0].Script, "__CIWI_STEP_BEGIN__") {
		t.Fatalf("expected script to include step marker, got:\n%s", jobsPayload.JobExecutions[0].Script)
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
			Script string `json:"script"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.JobExecutions) == 0 {
		t.Fatalf("expected at least one job execution")
	}
	script := jobsPayload.JobExecutions[0].Script
	if !strings.Contains(script, "kind=test") {
		t.Fatalf("expected step marker to include kind=test metadata, got:\n%s", script)
	}
	if !strings.Contains(script, "test_format=junit-xml") {
		t.Fatalf("expected step marker to include test format metadata, got:\n%s", script)
	}
	if !strings.Contains(script, "test_report=test-results.xml") {
		t.Fatalf("expected step marker to include test report metadata, got:\n%s", script)
	}
}
