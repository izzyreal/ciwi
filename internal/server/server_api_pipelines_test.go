package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
)

func loadPipelineTestConfig(t *testing.T, s *stateStore, yaml string) {
	t.Helper()
	cfg, err := config.Parse([]byte(yaml), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func firstPipelineAndChainIDs(t *testing.T, s *stateStore, projectName string) (int64, int64) {
	t.Helper()
	project, err := s.db.GetProjectByName(projectName)
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	detail, err := s.db.GetProjectDetail(project.ID)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if len(detail.Pipelines) == 0 {
		t.Fatalf("expected at least one pipeline")
	}
	var chainID int64
	if len(detail.PipelineChains) > 0 {
		chainID = detail.PipelineChains[0].ID
	}
	return detail.Pipelines[0].ID, chainID
}

func TestPipelineByIDHandler(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
      ref: main
    jobs:
      - id: compile
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        matrix:
          include:
            - name: linux
              goos: linux
              goarch: amd64
        steps:
          - run: echo build
`)

	pipelineID, _ := firstPipelineAndChainIDs(t, s, "ciwi")

	badID := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/not-a-number/run-selection", map[string]any{})
	if badID.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid pipeline id, got %d", badID.StatusCode)
	}

	notFound := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/nope", map[string]any{})
	if notFound.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown pipeline subpath, got %d", notFound.StatusCode)
	}

	methodGuard := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/run-selection", nil)
	if methodGuard.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET run-selection, got %d", methodGuard.StatusCode)
	}

	invalidJSON := mustRawJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/run-selection", `{"dry_run":`)
	if invalidJSON.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid run-selection JSON, got %d", invalidJSON.StatusCode)
	}

	runResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/run-selection", map[string]any{
		"dry_run": true,
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for pipeline run-selection, got %d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		Enqueued int `json:"enqueued"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if runPayload.Enqueued <= 0 {
		t.Fatalf("expected at least one enqueued job, got %+v", runPayload)
	}

	versionMethodGuard := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/version-resolve", map[string]any{})
	if versionMethodGuard.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST version-resolve, got %d", versionMethodGuard.StatusCode)
	}

	versionResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/version-resolve", nil)
	if versionResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for version-resolve stream, got %d body=%s", versionResp.StatusCode, readBody(t, versionResp))
	}
	versionBody := readBody(t, versionResp)
	if !strings.Contains(versionBody, `"step":"done"`) {
		t.Fatalf("expected version-resolve stream completion payload, got %q", versionBody)
	}
}

func TestPipelineChainByIDHandler(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
      ref: main
    jobs:
      - id: build
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo build
  - id: package
    trigger: manual
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
      ref: main
    jobs:
      - id: package
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo package
pipeline_chains:
  - id: build-package
    pipelines:
      - build
      - package
`)

	_, chainID := firstPipelineAndChainIDs(t, s, "ciwi")
	if chainID <= 0 {
		t.Fatalf("expected persisted pipeline chain id")
	}

	badID := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/not-a-number/run", map[string]any{})
	if badID.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid chain id, got %d", badID.StatusCode)
	}

	notFound := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/nope", map[string]any{})
	if notFound.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown chain subpath, got %d", notFound.StatusCode)
	}

	methodGuard := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/run", nil)
	if methodGuard.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET chain run, got %d", methodGuard.StatusCode)
	}

	invalidJSON := mustRawJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/run", `{"dry_run":`)
	if invalidJSON.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid chain run JSON, got %d", invalidJSON.StatusCode)
	}

	runResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/run", map[string]any{
		"dry_run": true,
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for chain run, got %d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var runPayload struct {
		Enqueued int `json:"enqueued"`
	}
	decodeJSONBody(t, runResp, &runPayload)
	if runPayload.Enqueued <= 0 {
		t.Fatalf("expected chain run to enqueue jobs, got %+v", runPayload)
	}
}

func TestPipelineChainRunIsAtomicOnValidationError(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi-atomic
pipelines:
  - id: first
    trigger: manual
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
      ref: main
    jobs:
      - id: publish
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo first
  - id: second
    trigger: manual
    vcs_source:
      repo: https://github.com/izzyreal/ciwi.git
      ref: main
    jobs:
      - id: prep
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo prep
      - id: publish
        needs:
          - prep
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo second
pipeline_chains:
  - id: first-second
    pipelines:
      - first
      - second
`)

	project, err := s.db.GetProjectByName("ciwi-atomic")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	detail, err := s.db.GetProjectDetail(project.ID)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if len(detail.PipelineChains) == 0 {
		t.Fatalf("expected pipeline chain")
	}
	chainID := detail.PipelineChains[0].ID

	runResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/run", map[string]any{
		"pipeline_job_id": "publish",
	})
	if runResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid selection across chain, got %d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	body := readBody(t, runResp)
	if !strings.Contains(body, `selection excludes required job "prep" needed by "publish"`) {
		t.Fatalf("unexpected error body: %s", body)
	}

	jobs, err := s.db.ListJobExecutions()
	if err != nil {
		t.Fatalf("ListJobExecutions: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs enqueued on failed chain validation, got %d", len(jobs))
	}
}

func TestPipelineSourceRefsHandler(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	repoURL, _, _ := createTestRemoteGitRepo(t)
	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    vcs_source:
      repo: `+repoURL+`
      ref: refs/heads/main
    jobs:
      - id: compile
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo build
`)

	pipelineID, _ := firstPipelineAndChainIDs(t, s, "ciwi")

	methodGuard := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/source-refs", map[string]any{})
	if methodGuard.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST source-refs, got %d", methodGuard.StatusCode)
	}

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/source-refs", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for pipeline source-refs, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		DefaultRef string `json:"default_ref"`
		Refs       []struct {
			Name string `json:"name"`
			Ref  string `json:"ref"`
		} `json:"refs"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.DefaultRef != "refs/heads/main" {
		t.Fatalf("expected default_ref refs/heads/main, got %q", payload.DefaultRef)
	}
	if len(payload.Refs) < 2 {
		t.Fatalf("expected at least two branch refs, got %+v", payload.Refs)
	}
}

func TestPipelineChainSourceRefsHandler(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	repoURL, _, _ := createTestRemoteGitRepo(t)
	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    vcs_source:
      repo: `+repoURL+`
      ref: refs/heads/main
    jobs:
      - id: build
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo build
  - id: package
    trigger: manual
    vcs_source:
      repo: `+repoURL+`
      ref: refs/heads/main
    jobs:
      - id: package
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo package
pipeline_chains:
  - id: build-package
    pipelines:
      - build
      - package
`)

	_, chainID := firstPipelineAndChainIDs(t, s, "ciwi")
	methodGuard := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/source-refs", map[string]any{})
	if methodGuard.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST chain source-refs, got %d", methodGuard.StatusCode)
	}

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/source-refs", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for chain source-refs, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		DefaultRef string `json:"default_ref"`
		Refs       []struct {
			Name string `json:"name"`
			Ref  string `json:"ref"`
		} `json:"refs"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.DefaultRef != "refs/heads/main" {
		t.Fatalf("expected default_ref refs/heads/main, got %q", payload.DefaultRef)
	}
	if len(payload.Refs) < 2 {
		t.Fatalf("expected at least two branch refs, got %+v", payload.Refs)
	}
}
