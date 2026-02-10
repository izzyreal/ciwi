package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestServerNegativePathsAndValidation(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()

	resp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/config/load", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /config/load, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	resp = mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing repo_url, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	resp = mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{
		"repo_url":    "https://github.com/izzyreal/ciwi.git",
		"config_file": "nested/ciwi-project.yaml",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for nested config_file, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	resp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects/not-a-number", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid project id, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	resp = mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/nope/run", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid pipeline id, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	resp = mustJSONRequest(t, client, http.MethodDelete, ts.URL+"/api/v1/jobs/missing-job", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleting missing job, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)
}

func TestServerStatusAndRunSelectionValidation(t *testing.T) {
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
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	client := ts.Client()

	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{
		"config_path": "ciwi.yaml",
	})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load config status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
		"script":          "echo hi",
		"timeout_seconds": 30,
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

	resp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"status": "running",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing agent_id, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	resp = mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"agent_id": "agent-x",
		"status":   "queued",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status transition, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	var projectsPayload struct {
		Projects []struct {
			Pipelines []struct {
				ID int64 `json:"id"`
			} `json:"pipelines"`
		} `json:"projects"`
	}
	projectsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if projectsResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", projectsResp.StatusCode, readBody(t, projectsResp))
	}
	decodeJSONBody(t, projectsResp, &projectsPayload)
	pipelineDBID := projectsPayload.Projects[0].Pipelines[0].ID

	resp = mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineDBID)+"/run-selection", map[string]any{
		"pipeline_job_id": "compile",
		"matrix_name":     "does-not-exist",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unmatched run-selection, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)

	resp = mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/tests", map[string]any{
		"report": map[string]any{"total": 1},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing agent_id in tests upload, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	_ = readBody(t, resp)
}
