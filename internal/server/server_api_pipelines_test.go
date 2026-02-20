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
    source:
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
    source:
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
    source:
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
