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

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-run/run-script", map[string]any{
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

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-run-2/run-script", map[string]any{
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

func TestQueuedJobExecutionIncludesUnmetRequirements(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
		"script": "echo hi",
		"required_capabilities": map[string]string{
			"requires.tool.go": ">=9.0",
		},
		"timeout_seconds": 30,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create job status=%d body=%s", createResp.StatusCode, readBody(t, createResp))
	}
	_ = readBody(t, createResp)

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var payload struct {
		Jobs []struct {
			ID                string   `json:"id"`
			UnmetRequirements []string `json:"unmet_requirements"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &payload)
	if len(payload.Jobs) == 0 {
		t.Fatalf("expected at least one job")
	}
	if len(payload.Jobs[0].UnmetRequirements) == 0 {
		t.Fatalf("expected unmet requirements on queued job")
	}
}

func TestJobExecutionStatusAppliesMetadataPatchEvent(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()

	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
		"script":          "echo build",
		"timeout_seconds": 60,
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

	statusResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"agent_id": "agent-test",
		"status":   "running",
		"events": []map[string]any{
			{
				"type": "metadata.patch",
				"metadata": map[string]string{
					"build_target":  "darwin-arm64",
					"build_version": "v2.3.4",
				},
			},
		},
	})
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status update status=%d body=%s", statusResp.StatusCode, readBody(t, statusResp))
	}
	_ = readBody(t, statusResp)

	jobResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID, nil)
	if jobResp.StatusCode != http.StatusOK {
		t.Fatalf("get job status=%d body=%s", jobResp.StatusCode, readBody(t, jobResp))
	}
	var payload struct {
		Job struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, jobResp, &payload)
	if payload.Job.Metadata["build_version"] != "v2.3.4" {
		t.Fatalf("unexpected build_version: %q", payload.Job.Metadata["build_version"])
	}
	if payload.Job.Metadata["build_target"] != "darwin-arm64" {
		t.Fatalf("unexpected build_target: %q", payload.Job.Metadata["build_target"])
	}
}

func TestJobExecutionEventsEndpointReturnsStoredEvents(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()
	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
		"script":          "echo build",
		"timeout_seconds": 60,
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

	statusResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"agent_id": "agent-test",
		"status":   "running",
		"events": []map[string]any{
			{
				"type": "step.started",
				"step": map[string]any{
					"index": 1,
					"total": 2,
					"name":  "build",
				},
			},
		},
	})
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status update status=%d body=%s", statusResp.StatusCode, readBody(t, statusResp))
	}
	_ = readBody(t, statusResp)

	eventsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/events", nil)
	if eventsResp.StatusCode != http.StatusOK {
		t.Fatalf("events status=%d body=%s", eventsResp.StatusCode, readBody(t, eventsResp))
	}
	var payload struct {
		Events []struct {
			Type string `json:"type"`
			Step struct {
				Index int    `json:"index"`
				Name  string `json:"name"`
			} `json:"step"`
		} `json:"events"`
	}
	decodeJSONBody(t, eventsResp, &payload)
	if len(payload.Events) != 1 {
		t.Fatalf("expected one event, got %d", len(payload.Events))
	}
	if payload.Events[0].Type != "step.started" {
		t.Fatalf("unexpected event type: %q", payload.Events[0].Type)
	}
	if payload.Events[0].Step.Index != 1 || payload.Events[0].Step.Name != "build" {
		t.Fatalf("unexpected event step payload: %+v", payload.Events[0].Step)
	}
}
