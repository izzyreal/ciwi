package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/izzyreal/ciwi/internal/store"
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

func newTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()

	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	artifactsDir := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("create artifacts dir: %v", err)
	}

	s := &stateStore{
		agents:       make(map[string]agentState),
		db:           db,
		artifactsDir: artifactsDir,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/config/load", s.loadConfigHandler)
	mux.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	mux.HandleFunc("/api/v1/projects/", s.projectByIDHandler)
	mux.HandleFunc("/api/v1/jobs", s.jobsHandler)
	mux.HandleFunc("/api/v1/jobs/", s.jobByIDHandler)
	mux.HandleFunc("/api/v1/jobs/clear-queue", s.clearQueueHandler)
	mux.HandleFunc("/api/v1/jobs/flush-history", s.flushHistoryHandler)
	mux.HandleFunc("/api/v1/pipelines/", s.pipelineByIDHandler)

	return httptest.NewServer(mux)
}

func mustJSONRequest(t *testing.T, client *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request JSON: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeJSONBody(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("decode response body: %v, tail=%q", err, string(raw))
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func TestServerLoadListRunAndQueueHistoryEndpoints(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "ciwi.yaml")
	if err := os.WriteFile(configPath, []byte(testConfigYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir to temp: %v", err)
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

	var projectsPayload struct {
		Projects []struct {
			ID        int64 `json:"id"`
			Pipelines []struct {
				ID int64 `json:"id"`
			} `json:"pipelines"`
		} `json:"projects"`
	}
	listResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", listResp.StatusCode, readBody(t, listResp))
	}
	decodeJSONBody(t, listResp, &projectsPayload)
	if len(projectsPayload.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projectsPayload.Projects))
	}
	if len(projectsPayload.Projects[0].Pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(projectsPayload.Projects[0].Pipelines))
	}
	projectID := projectsPayload.Projects[0].ID
	pipelineDBID := projectsPayload.Projects[0].Pipelines[0].ID

	detailResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects/"+int64ToString(projectID), nil)
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("get project detail status=%d body=%s", detailResp.StatusCode, readBody(t, detailResp))
	}
	_ = readBody(t, detailResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineDBID)+"/run", map[string]any{})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run pipeline status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		Enqueued int      `json:"enqueued"`
		JobIDs   []string `json:"job_ids"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if runPayload.Enqueued != 2 {
		t.Fatalf("expected enqueued=2 from matrix, got %d", runPayload.Enqueued)
	}
	if len(runPayload.JobIDs) != 2 {
		t.Fatalf("expected 2 job ids, got %d", len(runPayload.JobIDs))
	}

	var jobsPayload struct {
		Jobs []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"jobs"`
	}
	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("list jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobsPayload.Jobs))
	}

	deleteResp := mustJSONRequest(t, client, http.MethodDelete, ts.URL+"/api/v1/jobs/"+jobsPayload.Jobs[0].ID, nil)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete queued job status=%d body=%s", deleteResp.StatusCode, readBody(t, deleteResp))
	}
	_ = readBody(t, deleteResp)

	clearResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/clear-queue", map[string]any{})
	if clearResp.StatusCode != http.StatusOK {
		t.Fatalf("clear queue status=%d body=%s", clearResp.StatusCode, readBody(t, clearResp))
	}
	_ = readBody(t, clearResp)

	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
		"script":          "echo done",
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

	statusResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"agent_id": "agent-test",
		"status":   "succeeded",
		"output":   "ok",
	})
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status update status=%d body=%s", statusResp.StatusCode, readBody(t, statusResp))
	}
	_ = readBody(t, statusResp)

	testsPostResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/tests", map[string]any{
		"agent_id": "agent-test",
		"report": map[string]any{
			"total":   2,
			"passed":  1,
			"failed":  1,
			"skipped": 0,
			"suites": []map[string]any{
				{"name": "go-unit", "format": "go-test-json", "total": 2, "passed": 1, "failed": 1, "skipped": 0},
			},
		},
	})
	if testsPostResp.StatusCode != http.StatusOK {
		t.Fatalf("upload tests status=%d body=%s", testsPostResp.StatusCode, readBody(t, testsPostResp))
	}
	_ = readBody(t, testsPostResp)

	testsGetResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/tests", nil)
	if testsGetResp.StatusCode != http.StatusOK {
		t.Fatalf("get tests status=%d body=%s", testsGetResp.StatusCode, readBody(t, testsGetResp))
	}
	var testsPayload struct {
		Report struct {
			Total  int `json:"total"`
			Failed int `json:"failed"`
		} `json:"report"`
	}
	decodeJSONBody(t, testsGetResp, &testsPayload)
	if testsPayload.Report.Total != 2 || testsPayload.Report.Failed != 1 {
		t.Fatalf("unexpected tests report: %+v", testsPayload.Report)
	}

	artifactsAfterTestsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/artifacts", nil)
	if artifactsAfterTestsResp.StatusCode != http.StatusOK {
		t.Fatalf("artifacts after tests status=%d body=%s", artifactsAfterTestsResp.StatusCode, readBody(t, artifactsAfterTestsResp))
	}
	var artifactsAfterTests struct {
		Artifacts []struct {
			Path string `json:"path"`
			URL  string `json:"url"`
		} `json:"artifacts"`
	}
	decodeJSONBody(t, artifactsAfterTestsResp, &artifactsAfterTests)
	foundReportArtifact := false
	for _, a := range artifactsAfterTests.Artifacts {
		if a.Path == "test-report.json" {
			foundReportArtifact = true
			if a.URL == "" {
				t.Fatalf("test report artifact URL should not be empty")
			}
			break
		}
	}
	if !foundReportArtifact {
		t.Fatalf("expected test-report.json artifact after tests upload")
	}

	jobsWithSummaryResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsWithSummaryResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs with summary status=%d body=%s", jobsWithSummaryResp.StatusCode, readBody(t, jobsWithSummaryResp))
	}
	var jobsWithSummary struct {
		Jobs []struct {
			ID          string `json:"id"`
			TestSummary *struct {
				Total  int `json:"total"`
				Failed int `json:"failed"`
			} `json:"test_summary"`
		} `json:"jobs"`
	}
	decodeJSONBody(t, jobsWithSummaryResp, &jobsWithSummary)
	foundSummary := false
	for _, j := range jobsWithSummary.Jobs {
		if j.ID != createPayload.Job.ID || j.TestSummary == nil {
			continue
		}
		if j.TestSummary.Total != 2 || j.TestSummary.Failed != 1 {
			t.Fatalf("unexpected test_summary for job %s: %+v", j.ID, j.TestSummary)
		}
		foundSummary = true
	}
	if !foundSummary {
		t.Fatalf("expected test_summary for job %s", createPayload.Job.ID)
	}

	flushResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/flush-history", map[string]any{})
	if flushResp.StatusCode != http.StatusOK {
		t.Fatalf("flush history status=%d body=%s", flushResp.StatusCode, readBody(t, flushResp))
	}
	var flushPayload struct {
		Flushed int64 `json:"flushed"`
	}
	decodeJSONBody(t, flushResp, &flushPayload)
	if flushPayload.Flushed != 1 {
		t.Fatalf("expected flushed=1, got %d", flushPayload.Flushed)
	}

	jobsAfterResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsAfterResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs after flush status=%d body=%s", jobsAfterResp.StatusCode, readBody(t, jobsAfterResp))
	}
	var jobsAfter struct {
		Jobs []any `json:"jobs"`
	}
	decodeJSONBody(t, jobsAfterResp, &jobsAfter)
	if len(jobsAfter.Jobs) != 0 {
		t.Fatalf("expected 0 jobs after clear+flush, got %d", len(jobsAfter.Jobs))
	}
}

func TestServerRunSelectionQueuesSingleMatrixEntry(t *testing.T) {
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
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

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

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineDBID)+"/run-selection", map[string]any{
		"pipeline_job_id": "compile",
		"matrix_name":     "linux-amd64",
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run selection status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		Enqueued int `json:"enqueued"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if runPayload.Enqueued != 1 {
		t.Fatalf("expected enqueued=1, got %d", runPayload.Enqueued)
	}

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		Jobs []struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"jobs"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobsPayload.Jobs))
	}
	if got := jobsPayload.Jobs[0].Metadata["matrix_name"]; got != "linux-amd64" {
		t.Fatalf("expected matrix_name linux-amd64, got %q", got)
	}
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
