package server

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
		JobExecutionIDs []string `json:"job_execution_ids"`
	}
	decodeJSONBody(t, buildRunResp, &buildRun)
	if len(buildRun.JobExecutionIDs) != 1 {
		t.Fatalf("expected 1 build job")
	}
	buildJobID := buildRun.JobExecutionIDs[0]

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
		} `json:"job_executions"`
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
		JobExecutionIDs []string `json:"job_execution_ids"`
	}
	decodeJSONBody(t, releaseRunResp, &releaseRun)
	if len(releaseRun.JobExecutionIDs) != 1 {
		t.Fatalf("expected 1 release job")
	}

	jobsResp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	for _, j := range jobsPayload.Jobs {
		if j.ID != releaseRun.JobExecutionIDs[0] {
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

func TestServerPipelineVersionPreviewDependencyError(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	cfg := `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    jobs:
      - id: build-job
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo build
  - id: release
    trigger: manual
    depends_on:
      - build
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

	var releaseID int64
	for _, p := range projectsPayload.Projects[0].Pipelines {
		if p.PipelineID == "release" {
			releaseID = p.ID
			break
		}
	}
	if releaseID == 0 {
		t.Fatalf("missing release pipeline id")
	}

	previewResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/pipelines/"+int64ToString(releaseID)+"/version-preview", nil)
	if previewResp.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResp.StatusCode, readBody(t, previewResp))
	}
	var payload struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	decodeJSONBody(t, previewResp, &payload)
	if payload.OK {
		t.Fatalf("expected preview to fail when dependency has not succeeded")
	}
	if !strings.Contains(strings.ToLower(payload.Message), "dependency") {
		t.Fatalf("expected dependency error message, got %q", payload.Message)
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
