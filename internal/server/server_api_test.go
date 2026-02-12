package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
	"github.com/izzyreal/ciwi/internal/version"
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
		agents:           make(map[string]agentState),
		agentUpdates:     make(map[string]string),
		agentToolRefresh: make(map[string]bool),
		db:               db,
		artifactsDir:     artifactsDir,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/server-info", serverInfoHandler)
	mux.HandleFunc("/api/v1/config/load", s.loadConfigHandler)
	mux.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	mux.HandleFunc("/api/v1/projects/", s.projectByIDHandler)
	mux.HandleFunc("/api/v1/heartbeat", s.heartbeatHandler)
	mux.HandleFunc("/api/v1/agents", s.listAgentsHandler)
	mux.HandleFunc("/api/v1/agents/", s.agentByIDHandler)
	mux.HandleFunc("/api/v1/jobs", s.jobsHandler)
	mux.HandleFunc("/api/v1/jobs/", s.jobByIDHandler)
	mux.HandleFunc("/api/v1/jobs/clear-queue", s.clearQueueHandler)
	mux.HandleFunc("/api/v1/jobs/flush-history", s.flushHistoryHandler)
	mux.HandleFunc("/api/v1/pipelines/", s.pipelineByIDHandler)
	mux.HandleFunc("/api/v1/agent/lease", s.leaseJobHandler)
	mux.HandleFunc("/api/v1/vault/connections", s.vaultConnectionsHandler)
	mux.HandleFunc("/api/v1/vault/connections/", s.vaultConnectionByIDHandler)
	mux.HandleFunc("/api/v1/update/check", s.updateCheckHandler)
	mux.HandleFunc("/api/v1/update/apply", s.updateApplyHandler)
	mux.HandleFunc("/api/v1/update/rollback", s.updateRollbackHandler)
	mux.HandleFunc("/api/v1/update/tags", s.updateTagsHandler)
	mux.HandleFunc("/api/v1/update/status", s.updateStatusHandler)

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

func TestServerPipelineDependsOnGatesRelease(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	cfg := `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: build-job
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo build
  - id: release
    depends_on:
      - build
    source:
      repo: https://github.com/izzyreal/ciwi.git
    jobs:
      - id: release-job
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo release
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
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{"config_path": "ciwi.yaml"})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	var projectsPayload struct {
		Projects []struct {
			Pipelines []struct {
				ID         int64    `json:"id"`
				PipelineID string   `json:"pipeline_id"`
				DependsOn  []string `json:"depends_on"`
			} `json:"pipelines"`
		} `json:"projects"`
	}
	projectsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if projectsResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", projectsResp.StatusCode, readBody(t, projectsResp))
	}
	decodeJSONBody(t, projectsResp, &projectsPayload)
	if len(projectsPayload.Projects) != 1 || len(projectsPayload.Projects[0].Pipelines) != 2 {
		t.Fatalf("expected 1 project with 2 pipelines")
	}

	var buildID, releaseID int64
	for _, p := range projectsPayload.Projects[0].Pipelines {
		if p.PipelineID == "build" {
			buildID = p.ID
		}
		if p.PipelineID == "release" {
			releaseID = p.ID
			if len(p.DependsOn) != 1 || p.DependsOn[0] != "build" {
				t.Fatalf("expected release depends_on build, got %+v", p.DependsOn)
			}
		}
	}
	if buildID <= 0 || releaseID <= 0 {
		t.Fatalf("expected both build and release pipeline IDs")
	}

	releaseBeforeResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(releaseID)+"/run", map[string]any{})
	if releaseBeforeResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("release before build expected 400, got %d body=%s", releaseBeforeResp.StatusCode, readBody(t, releaseBeforeResp))
	}
	if body := readBody(t, releaseBeforeResp); !strings.Contains(body, "dependency") {
		t.Fatalf("expected dependency error, got %q", body)
	}

	buildRunResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(buildID)+"/run", map[string]any{})
	if buildRunResp.StatusCode != http.StatusCreated {
		t.Fatalf("build run status=%d body=%s", buildRunResp.StatusCode, readBody(t, buildRunResp))
	}
	var buildRunPayload struct {
		JobIDs []string `json:"job_ids"`
	}
	decodeJSONBody(t, buildRunResp, &buildRunPayload)
	if len(buildRunPayload.JobIDs) != 1 {
		t.Fatalf("expected 1 build job id, got %d", len(buildRunPayload.JobIDs))
	}
	buildJobID := buildRunPayload.JobIDs[0]

	buildDoneResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+buildJobID+"/status", map[string]any{
		"agent_id": "agent-test",
		"status":   "succeeded",
		"output":   "ok",
	})
	if buildDoneResp.StatusCode != http.StatusOK {
		t.Fatalf("mark build success status=%d body=%s", buildDoneResp.StatusCode, readBody(t, buildDoneResp))
	}
	_ = readBody(t, buildDoneResp)

	releaseAfterResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(releaseID)+"/run", map[string]any{})
	if releaseAfterResp.StatusCode != http.StatusCreated {
		t.Fatalf("release after build expected 201, got %d body=%s", releaseAfterResp.StatusCode, readBody(t, releaseAfterResp))
	}
	var releaseRunPayload struct {
		JobIDs []string `json:"job_ids"`
	}
	decodeJSONBody(t, releaseAfterResp, &releaseRunPayload)
	if len(releaseRunPayload.JobIDs) != 1 {
		t.Fatalf("expected 1 release job id, got %d", len(releaseRunPayload.JobIDs))
	}

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		Jobs []struct {
			ID       string            `json:"id"`
			Metadata map[string]string `json:"metadata"`
		} `json:"jobs"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	foundRelease := false
	for _, j := range jobsPayload.Jobs {
		if j.ID != releaseRunPayload.JobIDs[0] {
			continue
		}
		if strings.TrimSpace(j.Metadata["pipeline_run_id"]) == "" {
			t.Fatalf("expected pipeline_run_id metadata on release job")
		}
		foundRelease = true
	}
	if !foundRelease {
		t.Fatalf("release job not found in list jobs")
	}
}

func TestServerPipelineVersioningInheritsAcrossDependsOn(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	ts := newTestHTTPServer(t)
	defer ts.Close()

	repoDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v output=%s", args, err, string(out))
		}
	}
	run("init", "-q")
	run("config", "user.name", "ciwi-test")
	run("config", "user.email", "ciwi-test@local")
	if err := os.WriteFile(filepath.Join(repoDir, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", "VERSION", "README.md")
	run("commit", "-m", "init")

	headCmd := exec.Command("git", "rev-parse", "HEAD")
	headCmd.Dir = repoDir
	headOut, err := headCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v output=%s", err, string(headOut))
	}
	headSHA := strings.TrimSpace(string(headOut))

	cfg := `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: ` + repoDir + `
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: build-job
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo build
  - id: release
    depends_on:
      - build
    source:
      repo: ` + repoDir + `
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: release-job
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo release
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
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{"config_path": "ciwi.yaml"})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	var projectsPayload struct {
		Projects []struct {
			Pipelines []struct {
				ID         int64  `json:"id"`
				PipelineID string `json:"pipeline_id"`
			} `json:"pipelines"`
		} `json:"projects"`
	}
	projectsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if projectsResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", projectsResp.StatusCode, readBody(t, projectsResp))
	}
	decodeJSONBody(t, projectsResp, &projectsPayload)
	var buildID, releaseID int64
	for _, p := range projectsPayload.Projects[0].Pipelines {
		if p.PipelineID == "build" {
			buildID = p.ID
		}
		if p.PipelineID == "release" {
			releaseID = p.ID
		}
	}
	if buildID == 0 || releaseID == 0 {
		t.Fatalf("missing build/release pipeline ids")
	}

	buildRunResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(buildID)+"/run", map[string]any{})
	if buildRunResp.StatusCode != http.StatusCreated {
		t.Fatalf("build run status=%d body=%s", buildRunResp.StatusCode, readBody(t, buildRunResp))
	}
	var buildRun struct {
		JobIDs []string `json:"job_ids"`
	}
	decodeJSONBody(t, buildRunResp, &buildRun)
	if len(buildRun.JobIDs) != 1 {
		t.Fatalf("expected 1 build job")
	}
	buildJobID := buildRun.JobIDs[0]

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobsPayload struct {
		Jobs []struct {
			ID     string `json:"id"`
			Source struct {
				Ref string `json:"ref"`
			} `json:"source"`
			Metadata map[string]string `json:"metadata"`
		} `json:"jobs"`
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	var buildMeta map[string]string
	var buildRef string
	for _, j := range jobsPayload.Jobs {
		if j.ID == buildJobID {
			buildMeta = j.Metadata
			buildRef = strings.TrimSpace(j.Source.Ref)
			break
		}
	}
	if buildMeta["pipeline_version"] != "v1.2.3" {
		t.Fatalf("expected build pipeline_version v1.2.3, got %q", buildMeta["pipeline_version"])
	}
	if buildRef != headSHA {
		t.Fatalf("expected build source ref %q, got %q", headSHA, buildRef)
	}

	buildDoneResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+buildJobID+"/status", map[string]any{
		"agent_id": "agent-test",
		"status":   "succeeded",
		"output":   "ok",
	})
	if buildDoneResp.StatusCode != http.StatusOK {
		t.Fatalf("mark build success status=%d body=%s", buildDoneResp.StatusCode, readBody(t, buildDoneResp))
	}
	_ = readBody(t, buildDoneResp)

	releaseRunResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(releaseID)+"/run", map[string]any{})
	if releaseRunResp.StatusCode != http.StatusCreated {
		t.Fatalf("release run status=%d body=%s", releaseRunResp.StatusCode, readBody(t, releaseRunResp))
	}
	var releaseRun struct {
		JobIDs []string `json:"job_ids"`
	}
	decodeJSONBody(t, releaseRunResp, &releaseRun)
	if len(releaseRun.JobIDs) != 1 {
		t.Fatalf("expected 1 release job")
	}

	jobsResp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	for _, j := range jobsPayload.Jobs {
		if j.ID != releaseRun.JobIDs[0] {
			continue
		}
		if got := j.Metadata["pipeline_version"]; got != "v1.2.3" {
			t.Fatalf("expected inherited release version v1.2.3, got %q", got)
		}
		if got := strings.TrimSpace(j.Source.Ref); got != headSHA {
			t.Fatalf("expected inherited source ref %q, got %q", headSHA, got)
		}
		return
	}
	t.Fatalf("release job not found")
}

func TestServerPipelineVersionPreview(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	ts := newTestHTTPServer(t)
	defer ts.Close()

	repoDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v output=%s", args, err, string(out))
		}
	}
	run("init", "-q")
	run("config", "user.name", "ciwi-test")
	run("config", "user.email", "ciwi-test@local")
	if err := os.WriteFile(filepath.Join(repoDir, "VERSION"), []byte("2.4.6\n"), 0o644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", "VERSION", "README.md")
	run("commit", "-m", "init")

	cfg := `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: ` + repoDir + `
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: build-job
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo build
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
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{"config_path": "ciwi.yaml"})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	previewResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/pipelines/1/version-preview", nil)
	if previewResp.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResp.StatusCode, readBody(t, previewResp))
	}
	var payload struct {
		OK              bool   `json:"ok"`
		PipelineVersion string `json:"pipeline_version"`
	}
	decodeJSONBody(t, previewResp, &payload)
	if !payload.OK {
		t.Fatalf("expected ok preview response")
	}
	if payload.PipelineVersion != "v2.4.6" {
		t.Fatalf("unexpected pipeline version preview: %q", payload.PipelineVersion)
	}
}

func TestServerPipelineVersionResolveStream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	ts := newTestHTTPServer(t)
	defer ts.Close()

	repoDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v output=%s", args, err, string(out))
		}
	}
	run("init", "-q")
	run("config", "user.name", "ciwi-test")
	run("config", "user.email", "ciwi-test@local")
	if err := os.WriteFile(filepath.Join(repoDir, "VERSION"), []byte("3.1.4\n"), 0o644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", "VERSION", "README.md")
	run("commit", "-m", "init")

	cfg := `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: ` + repoDir + `
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: build-job
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo build
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
	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{"config_path": "ciwi.yaml"})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	resp, err := client.Get(ts.URL + "/api/v1/pipelines/1/version-resolve")
	if err != nil {
		t.Fatalf("get version-resolve: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("version-resolve status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, `"step":"done"`) || !strings.Contains(body, `"pipeline_version":"v3.1.4"`) {
		t.Fatalf("unexpected version-resolve body: %s", body)
	}
}

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

func TestServerUpdateCheckEndpoint(t *testing.T) {
	asset := expectedAssetName(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		t.Skip("runtime has no configured release asset naming")
	}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/izzyreal/ciwi/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://github.com/izzyreal/ciwi/releases/tag/v0.2.0","assets":[{"name":"` + asset + `","url":"https://example.invalid/asset"}]}`))
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	oldVersion := version.Version
	version.Version = "v0.1.0"
	t.Cleanup(func() { version.Version = oldVersion })
	t.Setenv("CIWI_UPDATE_REQUIRE_CHECKSUM", "false")

	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/check", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update check status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		CurrentVersion  string `json:"current_version"`
		LatestVersion   string `json:"latest_version"`
		UpdateAvailable bool   `json:"update_available"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.CurrentVersion != "v0.1.0" {
		t.Fatalf("unexpected current_version: %q", payload.CurrentVersion)
	}
	if payload.LatestVersion != "v0.2.0" {
		t.Fatalf("unexpected latest_version: %q", payload.LatestVersion)
	}
	if !payload.UpdateAvailable {
		t.Fatalf("expected update_available=true")
	}
}

func TestServerInfoEndpoint(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v0.9.1"
	t.Cleanup(func() { version.Version = oldVersion })
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/server-info", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("server info status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		Name       string `json:"name"`
		APIVersion int    `json:"api_version"`
		Version    string `json:"version"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.Name != "ciwi" {
		t.Fatalf("unexpected name: %q", payload.Name)
	}
	if payload.APIVersion != 1 {
		t.Fatalf("unexpected api_version: %d", payload.APIVersion)
	}
	if payload.Version != "v0.9.1" {
		t.Fatalf("unexpected version: %q", payload.Version)
	}
}

func TestServerUpdateStatusEndpoint(t *testing.T) {
	asset := expectedAssetName(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		t.Skip("runtime has no configured release asset naming")
	}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/izzyreal/ciwi/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://github.com/izzyreal/ciwi/releases/tag/v0.2.0","assets":[{"name":"` + asset + `","url":"https://example.invalid/asset"},{"name":"ciwi-checksums.txt","url":"https://example.invalid/checksums"}]}`))
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	oldVersion := version.Version
	version.Version = "v0.1.0"
	t.Cleanup(func() { version.Version = oldVersion })
	t.Setenv("CIWI_UPDATE_REQUIRE_CHECKSUM", "false")

	ts := newTestHTTPServer(t)
	defer ts.Close()

	checkResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/check", map[string]any{})
	if checkResp.StatusCode != http.StatusOK {
		t.Fatalf("update check status=%d body=%s", checkResp.StatusCode, readBody(t, checkResp))
	}
	_ = readBody(t, checkResp)

	statusResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/update/status", nil)
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("update status status=%d body=%s", statusResp.StatusCode, readBody(t, statusResp))
	}
	var payload struct {
		Status map[string]string `json:"status"`
	}
	decodeJSONBody(t, statusResp, &payload)
	if payload.Status["update_current_version"] != "v0.1.0" {
		t.Fatalf("unexpected update_current_version: %q", payload.Status["update_current_version"])
	}
	if payload.Status["update_latest_version"] != "v0.2.0" {
		t.Fatalf("unexpected update_latest_version: %q", payload.Status["update_latest_version"])
	}
	if payload.Status["update_available"] != "1" {
		t.Fatalf("unexpected update_available: %q", payload.Status["update_available"])
	}
}

func TestServerUpdateTagsEndpoint(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/izzyreal/ciwi/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"v2.0.0"},{"name":"v1.9.0"}]`))
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	oldVersion := version.Version
	version.Version = "v1.8.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/update/tags", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update tags status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		Tags           []string `json:"tags"`
		CurrentVersion string   `json:"current_version"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.CurrentVersion != "v1.8.0" {
		t.Fatalf("unexpected current_version: %q", payload.CurrentVersion)
	}
	if len(payload.Tags) < 3 {
		t.Fatalf("expected current version to be prepended to tags, got %+v", payload.Tags)
	}
	if payload.Tags[0] != "v1.8.0" {
		t.Fatalf("expected current version first, got %+v", payload.Tags)
	}
}

func TestServerRollbackRequiresTargetVersion(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/rollback", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("rollback status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
}

func TestHeartbeatDoesNotRequestAgentUpdate(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()

	hbResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	var hbPayload struct {
		Accepted        bool   `json:"accepted"`
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
	}
	decodeJSONBody(t, hbResp, &hbPayload)
	if !hbPayload.Accepted {
		t.Fatalf("expected accepted=true")
	}
	if hbPayload.UpdateRequested {
		t.Fatalf("expected update_requested=false")
	}
	if hbPayload.UpdateTarget != "" {
		t.Fatalf("unexpected update_target: %q", hbPayload.UpdateTarget)
	}

	agentsResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if agentsResp.StatusCode != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", agentsResp.StatusCode, readBody(t, agentsResp))
	}
	var agentsPayload struct {
		Agents []struct {
			AgentID string `json:"agent_id"`
			Version string `json:"version"`
		} `json:"agents"`
	}
	decodeJSONBody(t, agentsResp, &agentsPayload)
	if len(agentsPayload.Agents) != 1 {
		t.Fatalf("expected exactly one agent, got %d", len(agentsPayload.Agents))
	}
	if agentsPayload.Agents[0].AgentID != "agent-a" || agentsPayload.Agents[0].Version != "v1.1.0" {
		t.Fatalf("unexpected agent payload: %+v", agentsPayload.Agents[0])
	}
}

func TestGetAgentByIDEndpoint(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-by-id",
		"hostname":      "host-z",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	getResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/agent-by-id", nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get agent status=%d body=%s", getResp.StatusCode, readBody(t, getResp))
	}
	var payload struct {
		Agent struct {
			AgentID     string `json:"agent_id"`
			Hostname    string `json:"hostname"`
			Version     string `json:"version"`
			NeedsUpdate bool   `json:"needs_update"`
		} `json:"agent"`
	}
	decodeJSONBody(t, getResp, &payload)
	if payload.Agent.AgentID != "agent-by-id" {
		t.Fatalf("unexpected agent id: %q", payload.Agent.AgentID)
	}
	if payload.Agent.Hostname != "host-z" {
		t.Fatalf("unexpected hostname: %q", payload.Agent.Hostname)
	}
	if payload.Agent.Version != "v1.1.0" {
		t.Fatalf("unexpected version: %q", payload.Agent.Version)
	}
	if !payload.Agent.NeedsUpdate {
		t.Fatalf("expected needs_update=true")
	}

	missingResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/does-not-exist", nil)
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing agent status=%d body=%s", missingResp.StatusCode, readBody(t, missingResp))
	}
	_ = readBody(t, missingResp)
}

func TestManualAgentUpdateRequestTriggersHeartbeatUpdate(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "darwin",
		"arch":          "arm64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	manualResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-a/update", map[string]any{})
	if manualResp.StatusCode != http.StatusOK {
		t.Fatalf("manual update status=%d body=%s", manualResp.StatusCode, readBody(t, manualResp))
	}
	_ = readBody(t, manualResp)

	agentsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if agentsResp.StatusCode != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", agentsResp.StatusCode, readBody(t, agentsResp))
	}
	var agentsPayload struct {
		Agents []struct {
			AgentID         string `json:"agent_id"`
			UpdateRequested bool   `json:"update_requested"`
			UpdateTarget    string `json:"update_target"`
		} `json:"agents"`
	}
	decodeJSONBody(t, agentsResp, &agentsPayload)
	if len(agentsPayload.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agentsPayload.Agents))
	}
	if !agentsPayload.Agents[0].UpdateRequested {
		t.Fatalf("expected update_requested=true on agents list")
	}
	if agentsPayload.Agents[0].UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected update_target in agents list: %q", agentsPayload.Agents[0].UpdateTarget)
	}

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "darwin",
		"arch":          "arm64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var hbPayload struct {
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
	}
	decodeJSONBody(t, secondHB, &hbPayload)
	if !hbPayload.UpdateRequested {
		t.Fatalf("expected update_requested=true")
	}
	if hbPayload.UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected update_target: %q", hbPayload.UpdateTarget)
	}
}

func TestManualRefreshToolsRequest(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-refresh",
		"hostname":      "host-r",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	refreshResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-refresh/refresh-tools", map[string]any{})
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("refresh-tools status=%d body=%s", refreshResp.StatusCode, readBody(t, refreshResp))
	}
	_ = readBody(t, refreshResp)

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-refresh",
		"hostname":      "host-r",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var hbPayload struct {
		RefreshToolsRequested bool `json:"refresh_tools_requested"`
	}
	decodeJSONBody(t, secondHB, &hbPayload)
	if !hbPayload.RefreshToolsRequested {
		t.Fatalf("expected refresh_tools_requested=true")
	}
}

func TestAgentRunScriptQueuesTargetedJob(t *testing.T) {
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
		Queued bool   `json:"queued"`
		JobID  string `json:"job_id"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if !runPayload.Queued || strings.TrimSpace(runPayload.JobID) == "" {
		t.Fatalf("unexpected run-script payload: %+v", runPayload)
	}

	jobResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+runPayload.JobID, nil)
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
		} `json:"job"`
	}
	decodeJSONBody(t, leaseTarget, &leaseTargetPayload)
	if !leaseTargetPayload.Assigned || leaseTargetPayload.Job.ID != runPayload.JobID {
		t.Fatalf("expected targeted agent to lease queued job, got %+v", leaseTargetPayload)
	}
}

func TestAgentRunScriptRejectsUnsupportedShell(t *testing.T) {
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

func TestQueuedJobIncludesUnmetRequirements(t *testing.T) {
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

func TestJobStatusParsesBuildSummaryIntoMetadata(t *testing.T) {
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
		} `json:"job"`
	}
	decodeJSONBody(t, createResp, &createPayload)

	statusResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"agent_id": "agent-test",
		"status":   "running",
		"output":   "__CIWI_BUILD_SUMMARY__ target=darwin-arm64 version=v2.3.4 output=dist/ciwi-darwin-arm64\n",
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

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
