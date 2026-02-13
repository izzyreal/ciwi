package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestJobJSONOmitsZeroStartedAndFinishedTimestamps(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	client := ts.Client()

	createResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs", map[string]any{
		"script":          "echo timestamps",
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

	jobResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID, nil)
	if jobResp.StatusCode != http.StatusOK {
		t.Fatalf("get queued job status=%d body=%s", jobResp.StatusCode, readBody(t, jobResp))
	}
	queuedJob := decodeJobByIDPayload(t, jobResp)
	assertTimestampOmitted(t, queuedJob, "started_utc")
	assertTimestampOmitted(t, queuedJob, "finished_utc")

	runningResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"agent_id": "agent-a",
		"status":   "running",
		"output":   "working",
	})
	if runningResp.StatusCode != http.StatusOK {
		t.Fatalf("mark running status=%d body=%s", runningResp.StatusCode, readBody(t, runningResp))
	}
	_ = readBody(t, runningResp)

	jobResp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID, nil)
	if jobResp.StatusCode != http.StatusOK {
		t.Fatalf("get running job status=%d body=%s", jobResp.StatusCode, readBody(t, jobResp))
	}
	runningJob := decodeJobByIDPayload(t, jobResp)
	assertTimestampPresent(t, runningJob, "started_utc")
	assertTimestampOmitted(t, runningJob, "finished_utc")

	doneResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID+"/status", map[string]any{
		"agent_id": "agent-a",
		"status":   "succeeded",
		"output":   "done",
	})
	if doneResp.StatusCode != http.StatusOK {
		t.Fatalf("mark succeeded status=%d body=%s", doneResp.StatusCode, readBody(t, doneResp))
	}
	_ = readBody(t, doneResp)

	jobResp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs/"+createPayload.Job.ID, nil)
	if jobResp.StatusCode != http.StatusOK {
		t.Fatalf("get succeeded job status=%d body=%s", jobResp.StatusCode, readBody(t, jobResp))
	}
	doneJob := decodeJobByIDPayload(t, jobResp)
	assertTimestampPresent(t, doneJob, "started_utc")
	assertTimestampPresent(t, doneJob, "finished_utc")
}

func decodeJobByIDPayload(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var payload struct {
		Job          map[string]any `json:"job"`
		JobExecution map[string]any `json:"job_execution"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.Job != nil {
		return payload.Job
	}
	if payload.JobExecution != nil {
		return payload.JobExecution
	}
	t.Fatalf("job-by-id payload missing both job and job_execution")
	return nil
}

func assertTimestampOmitted(t *testing.T, job map[string]any, key string) {
	t.Helper()
	if value, ok := job[key]; ok {
		t.Fatalf("expected %s omitted, got %v", key, value)
	}
}

func assertTimestampPresent(t *testing.T, job map[string]any, key string) {
	t.Helper()
	value, ok := job[key]
	if !ok {
		t.Fatalf("expected %s present", key)
	}
	if value == nil {
		t.Fatalf("expected %s non-null", key)
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("expected %s to be string, got %T", key, value)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		t.Fatalf("expected %s non-empty", key)
	}
	if text == "0001-01-01T00:00:00Z" {
		t.Fatalf("expected %s to be non-zero timestamp", key)
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
