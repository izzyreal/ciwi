package server

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

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
