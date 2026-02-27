package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestAgentRunScriptQueuesTargetedJobExecution(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-run",
		"hostname":      "host-run",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-12T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-run/actions", map[string]any{
		"action":          "run-script",
		"shell":           "posix",
		"script":          "echo hello",
		"timeout_seconds": 120,
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run-script status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		Queued         bool   `json:"queued"`
		JobExecutionID string `json:"job_execution_id"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if !runPayload.Queued || strings.TrimSpace(runPayload.JobExecutionID) == "" {
		t.Fatalf("unexpected run-script payload: %+v", runPayload)
	}

	jobResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+runPayload.JobExecutionID, nil)
	if jobResp.StatusCode != http.StatusOK {
		t.Fatalf("get job status=%d body=%s", jobResp.StatusCode, readBody(t, jobResp))
	}
	var jobPayload struct {
		Job struct {
			ID                   string            `json:"id"`
			RequiredCapabilities map[string]string `json:"required_capabilities"`
			Metadata             map[string]string `json:"metadata"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, jobResp, &jobPayload)
	if jobPayload.Job.RequiredCapabilities["agent_id"] != "agent-run" {
		t.Fatalf("expected agent_id targeting, got %+v", jobPayload.Job.RequiredCapabilities)
	}
	if jobPayload.Job.RequiredCapabilities["shell"] != "posix" {
		t.Fatalf("expected shell=posix, got %+v", jobPayload.Job.RequiredCapabilities)
	}
	if jobPayload.Job.Metadata["adhoc"] != "1" {
		t.Fatalf("expected adhoc metadata, got %+v", jobPayload.Job.Metadata)
	}

	leaseOther := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-other",
		"capabilities": map[string]string{"executor": "script", "shells": "posix"},
	})
	if leaseOther.StatusCode != http.StatusOK {
		t.Fatalf("lease other status=%d body=%s", leaseOther.StatusCode, readBody(t, leaseOther))
	}
	var leaseOtherPayload struct {
		Assigned bool `json:"assigned"`
	}
	decodeJSONBody(t, leaseOther, &leaseOtherPayload)
	if leaseOtherPayload.Assigned {
		t.Fatalf("expected other agent lease to be rejected")
	}

	authResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-run/actions", map[string]any{"action": "authorize"})
	if authResp.StatusCode != http.StatusOK {
		t.Fatalf("authorize status=%d body=%s", authResp.StatusCode, readBody(t, authResp))
	}
	_ = readBody(t, authResp)

	leaseTarget := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id":     "agent-run",
		"capabilities": map[string]string{"executor": "script", "shells": "posix"},
	})
	if leaseTarget.StatusCode != http.StatusOK {
		t.Fatalf("lease target status=%d body=%s", leaseTarget.StatusCode, readBody(t, leaseTarget))
	}
	var leaseTargetPayload struct {
		Assigned bool `json:"assigned"`
		Job      struct {
			ID string `json:"id"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, leaseTarget, &leaseTargetPayload)
	if !leaseTargetPayload.Assigned || leaseTargetPayload.Job.ID != runPayload.JobExecutionID {
		t.Fatalf("expected targeted agent to lease queued job, got %+v", leaseTargetPayload)
	}
}

func TestAgentRunScriptRejectsUnsupportedShellForJobExecution(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-run-2",
		"hostname":      "host-run-2",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-12T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-run-2/actions", map[string]any{
		"action": "run-script",
		"shell":  "powershell",
		"script": "Write-Host hi",
	})
	if runResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("run-script unsupported shell status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	if body := readBody(t, runResp); !strings.Contains(body, "does not support requested shell") {
		t.Fatalf("unexpected unsupported shell response: %s", body)
	}
}

func TestAgentLeaseHandlerValidationAndBranches(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	methodResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agent/lease", nil)
	if methodResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET lease, got %d", methodResp.StatusCode)
	}

	invalidJSON := mustRawJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", `{"agent_id":`)
	if invalidJSON.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid lease json, got %d", invalidJSON.StatusCode)
	}

	missingAgent := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{})
	if missingAgent.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing agent_id, got %d", missingAgent.StatusCode)
	}

	noJob := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-empty",
	})
	if noJob.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for no matching job, got %d body=%s", noJob.StatusCode, readBody(t, noJob))
	}
	var noJobPayload struct {
		Assigned bool   `json:"assigned"`
		Message  string `json:"message"`
	}
	decodeJSONBody(t, noJob, &noJobPayload)
	if noJobPayload.Assigned || !strings.Contains(noJobPayload.Message, "not registered") {
		t.Fatalf("unexpected no-job lease payload: %+v", noJobPayload)
	}

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-busy",
		"hostname":      "host-busy",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-12T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	unauthLease := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-busy",
	})
	if unauthLease.StatusCode != http.StatusOK {
		t.Fatalf("expected unauthorized lease 200, got %d body=%s", unauthLease.StatusCode, readBody(t, unauthLease))
	}
	var unauthLeasePayload struct {
		Assigned bool   `json:"assigned"`
		Message  string `json:"message"`
	}
	decodeJSONBody(t, unauthLease, &unauthLeasePayload)
	if unauthLeasePayload.Assigned || !strings.Contains(unauthLeasePayload.Message, "not authorized") {
		t.Fatalf("unexpected unauthorized lease payload: %+v", unauthLeasePayload)
	}

	authResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-busy/actions", map[string]any{"action": "authorize"})
	if authResp.StatusCode != http.StatusOK {
		t.Fatalf("authorize status=%d body=%s", authResp.StatusCode, readBody(t, authResp))
	}
	_ = readBody(t, authResp)

	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-busy/actions", map[string]any{
		"action":          "run-script",
		"shell":           "posix",
		"script":          "echo hi",
		"timeout_seconds": 30,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("run-script create job status=%d body=%s", createResp.StatusCode, readBody(t, createResp))
	}
	var createPayload struct {
		JobExecutionID string `json:"job_execution_id"`
	}
	decodeJSONBody(t, createResp, &createPayload)

	firstLease := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-busy",
	})
	if firstLease.StatusCode != http.StatusOK {
		t.Fatalf("expected first lease 200, got %d body=%s", firstLease.StatusCode, readBody(t, firstLease))
	}
	var firstLeasePayload struct {
		Assigned bool `json:"assigned"`
		Job      struct {
			ID string `json:"id"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, firstLease, &firstLeasePayload)
	if !firstLeasePayload.Assigned || firstLeasePayload.Job.ID != createPayload.JobExecutionID {
		t.Fatalf("unexpected first lease payload: %+v", firstLeasePayload)
	}

	secondLease := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-busy",
	})
	if secondLease.StatusCode != http.StatusOK {
		t.Fatalf("expected second lease 200, got %d body=%s", secondLease.StatusCode, readBody(t, secondLease))
	}
	var secondLeasePayload struct {
		Assigned bool   `json:"assigned"`
		Message  string `json:"message"`
	}
	decodeJSONBody(t, secondLease, &secondLeasePayload)
	if secondLeasePayload.Assigned || !strings.Contains(secondLeasePayload.Message, "already has an active job") {
		t.Fatalf("unexpected second lease payload: %+v", secondLeasePayload)
	}
}

func TestAgentDeactivationBlocksLeaseUntilActivated(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-toggle",
		"hostname":      "host-toggle",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-12T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	authResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-toggle/actions", map[string]any{"action": "authorize"})
	if authResp.StatusCode != http.StatusOK {
		t.Fatalf("authorize status=%d body=%s", authResp.StatusCode, readBody(t, authResp))
	}
	_ = readBody(t, authResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-toggle/actions", map[string]any{
		"action": "run-script",
		"shell":  "posix",
		"script": "echo queued",
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run-script status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		JobExecutionID string `json:"job_execution_id"`
	}
	decodeJSONBody(t, runResp, &runPayload)

	deactivateResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-toggle/actions", map[string]any{"action": "deactivate"})
	if deactivateResp.StatusCode != http.StatusOK {
		t.Fatalf("deactivate status=%d body=%s", deactivateResp.StatusCode, readBody(t, deactivateResp))
	}
	_ = readBody(t, deactivateResp)

	leaseBlocked := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-toggle",
	})
	if leaseBlocked.StatusCode != http.StatusOK {
		t.Fatalf("blocked lease status=%d body=%s", leaseBlocked.StatusCode, readBody(t, leaseBlocked))
	}
	var leaseBlockedPayload struct {
		Assigned bool   `json:"assigned"`
		Message  string `json:"message"`
	}
	decodeJSONBody(t, leaseBlocked, &leaseBlockedPayload)
	if leaseBlockedPayload.Assigned || !strings.Contains(leaseBlockedPayload.Message, "deactivated") {
		t.Fatalf("unexpected blocked lease payload: %+v", leaseBlockedPayload)
	}

	activateResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-toggle/actions", map[string]any{"action": "activate"})
	if activateResp.StatusCode != http.StatusOK {
		t.Fatalf("activate status=%d body=%s", activateResp.StatusCode, readBody(t, activateResp))
	}
	_ = readBody(t, activateResp)

	leaseAllowed := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-toggle",
	})
	if leaseAllowed.StatusCode != http.StatusOK {
		t.Fatalf("allowed lease status=%d body=%s", leaseAllowed.StatusCode, readBody(t, leaseAllowed))
	}
	var leaseAllowedPayload struct {
		Assigned bool `json:"assigned"`
		Job      struct {
			ID string `json:"id"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, leaseAllowed, &leaseAllowedPayload)
	if !leaseAllowedPayload.Assigned || leaseAllowedPayload.Job.ID != runPayload.JobExecutionID {
		t.Fatalf("unexpected allowed lease payload: %+v", leaseAllowedPayload)
	}
}

func TestAgentDeactivationCancelsActiveJob(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-cancel-on-deactivate",
		"hostname":      "host-cancel-on-deactivate",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-12T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	authResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-cancel-on-deactivate/actions", map[string]any{"action": "authorize"})
	if authResp.StatusCode != http.StatusOK {
		t.Fatalf("authorize status=%d body=%s", authResp.StatusCode, readBody(t, authResp))
	}
	_ = readBody(t, authResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-cancel-on-deactivate/actions", map[string]any{
		"action": "run-script",
		"shell":  "posix",
		"script": "echo active",
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run-script status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		JobExecutionID string `json:"job_execution_id"`
	}
	decodeJSONBody(t, runResp, &runPayload)

	firstLease := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-cancel-on-deactivate",
	})
	if firstLease.StatusCode != http.StatusOK {
		t.Fatalf("first lease status=%d body=%s", firstLease.StatusCode, readBody(t, firstLease))
	}
	var firstLeasePayload struct {
		Assigned bool `json:"assigned"`
		Job      struct {
			ID string `json:"id"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, firstLease, &firstLeasePayload)
	if !firstLeasePayload.Assigned || firstLeasePayload.Job.ID != runPayload.JobExecutionID {
		t.Fatalf("unexpected first lease payload: %+v", firstLeasePayload)
	}

	deactivateResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-cancel-on-deactivate/actions", map[string]any{"action": "deactivate"})
	if deactivateResp.StatusCode != http.StatusOK {
		t.Fatalf("deactivate status=%d body=%s", deactivateResp.StatusCode, readBody(t, deactivateResp))
	}
	var deactivatePayload struct {
		Message string `json:"message"`
	}
	decodeJSONBody(t, deactivateResp, &deactivatePayload)
	if !strings.Contains(deactivatePayload.Message, "cancelled active jobs=1") {
		t.Fatalf("unexpected deactivate response message: %q", deactivatePayload.Message)
	}

	jobResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+runPayload.JobExecutionID, nil)
	if jobResp.StatusCode != http.StatusOK {
		t.Fatalf("get job status=%d body=%s", jobResp.StatusCode, readBody(t, jobResp))
	}
	var jobPayload struct {
		Job struct {
			Status string `json:"status"`
			Error  string `json:"error"`
			Output string `json:"output"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, jobResp, &jobPayload)
	if jobPayload.Job.Status != "failed" {
		t.Fatalf("expected failed status, got %q", jobPayload.Job.Status)
	}
	if jobPayload.Job.Error != "cancelled by user" {
		t.Fatalf("expected cancelled by user error, got %q", jobPayload.Job.Error)
	}
	if !strings.Contains(jobPayload.Job.Output, "[control] job cancelled by user") {
		t.Fatalf("expected cancel marker in output, got %q", jobPayload.Job.Output)
	}
}
