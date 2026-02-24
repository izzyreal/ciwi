package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
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

func TestPipelineEligibleAgentsHandler(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
    jobs:
      - id: compile
        runs_on:
          os: linux
          arch: amd64
        requires:
          tools:
            git: ">=2.40.0"
        timeout_seconds: 30
        steps:
          - run: echo build
`)
	pipelineID, _ := firstPipelineAndChainIDs(t, s, "ciwi")
	s.mu.Lock()
	s.agents["agent-linux"] = agentState{
		OS:   "linux",
		Arch: "amd64",
		Capabilities: map[string]string{
			"shells":   "posix",
			"tool.git": "2.42.0",
		},
	}
	s.agents["agent-windows"] = agentState{
		OS:   "windows",
		Arch: "amd64",
		Capabilities: map[string]string{
			"shells":   "cmd,powershell",
			"tool.git": "2.42.0",
		},
	}
	s.mu.Unlock()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/eligible-agents", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for pipeline eligible-agents, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		EligibleAgentIDs []string `json:"eligible_agent_ids"`
		PendingJobs      int      `json:"pending_jobs"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.PendingJobs != 1 {
		t.Fatalf("expected pending_jobs=1, got %d", payload.PendingJobs)
	}
	if len(payload.EligibleAgentIDs) != 1 || payload.EligibleAgentIDs[0] != "agent-linux" {
		t.Fatalf("unexpected eligible agents: %+v", payload.EligibleAgentIDs)
	}

	filteredResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/eligible-agents", map[string]any{
		"agent_id": "agent-windows",
	})
	if filteredResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for pipeline eligible-agents filtered request, got %d body=%s", filteredResp.StatusCode, readBody(t, filteredResp))
	}
	var filtered struct {
		EligibleAgentIDs []string `json:"eligible_agent_ids"`
	}
	decodeJSONBody(t, filteredResp, &filtered)
	if len(filtered.EligibleAgentIDs) != 0 {
		t.Fatalf("expected no eligible agents after non-matching agent_id filter, got %+v", filtered.EligibleAgentIDs)
	}
}

func TestPipelineChainEligibleAgentsHandler(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
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
	s.mu.Lock()
	s.agents["agent-linux"] = agentState{OS: "linux", Arch: "amd64", Capabilities: map[string]string{"shells": "posix"}}
	s.agents["agent-darwin"] = agentState{OS: "darwin", Arch: "arm64", Capabilities: map[string]string{"shells": "posix"}}
	s.mu.Unlock()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/eligible-agents", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for chain eligible-agents, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		EligibleAgentIDs []string `json:"eligible_agent_ids"`
		PendingJobs      int      `json:"pending_jobs"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.PendingJobs != 2 {
		t.Fatalf("expected pending_jobs=2, got %d", payload.PendingJobs)
	}
	if len(payload.EligibleAgentIDs) != 1 || payload.EligibleAgentIDs[0] != "agent-linux" {
		t.Fatalf("unexpected eligible agents: %+v", payload.EligibleAgentIDs)
	}
}

func TestPipelineDryRunPreviewOfflineCachedOnlyUsesCachedContext(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    trigger: manual
    vcs_source:
      repo: https://github.com/acme/repo.git
      ref: refs/heads/main
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: publish
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo publish
`)
	pipelineID, _ := firstPipelineAndChainIDs(t, s, "ciwi")
	buildExec, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo cached",
		TimeoutSeconds: 30,
		Metadata: map[string]string{
			"project":                      "ciwi",
			"pipeline_id":                  "release",
			"pipeline_run_id":              "run-release-1",
			"pipeline_version_raw":         "1.2.3",
			"pipeline_version":             "v1.2.3",
			"pipeline_source_repo":         "https://github.com/acme/repo.git",
			"pipeline_source_ref_raw":      "refs/heads/main",
			"pipeline_source_ref_resolved": "0123456789abcdef0123456789abcdef01234567",
		},
	})
	if err != nil {
		t.Fatalf("create cached run job: %v", err)
	}
	if _, err := s.db.UpdateJobExecutionStatus(buildExec.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("mark cached run succeeded: %v", err)
	}

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/dry-run-preview", map[string]any{
		"offline_cached_only": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for dry-run-preview, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		CacheUsed   bool `json:"cache_used"`
		PendingJobs []struct {
			SourceRef string `json:"source_ref"`
			StepCount int    `json:"step_count"`
		} `json:"pending_jobs"`
	}
	decodeJSONBody(t, resp, &payload)
	if !payload.CacheUsed {
		t.Fatalf("expected cache_used=true")
	}
	if len(payload.PendingJobs) != 1 {
		t.Fatalf("expected one pending job, got %+v", payload.PendingJobs)
	}
	if payload.PendingJobs[0].SourceRef != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("unexpected preview source ref: %q", payload.PendingJobs[0].SourceRef)
	}
	if payload.PendingJobs[0].StepCount != 1 {
		t.Fatalf("unexpected step_count: %+v", payload.PendingJobs[0].StepCount)
	}
}

func TestPipelineDryRunPreviewOfflineCachedOnlyFailsWithoutCacheForVersionedPipeline(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    trigger: manual
    vcs_source:
      repo: https://github.com/acme/repo.git
      ref: refs/heads/main
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: publish
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo publish
`)
	pipelineID, _ := firstPipelineAndChainIDs(t, s, "ciwi")
	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/dry-run-preview", map[string]any{
		"offline_cached_only": true,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for dry-run-preview without cache, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "offline_cached_only requires a prior successful") {
		t.Fatalf("unexpected error body: %s", body)
	}
}

func TestPipelineChainDryRunPreview(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    trigger: manual
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
	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipeline-chains/"+int64ToString(chainID)+"/dry-run-preview", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for chain dry-run-preview, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		PendingJobs []map[string]any `json:"pending_jobs"`
	}
	decodeJSONBody(t, resp, &payload)
	if len(payload.PendingJobs) != 2 {
		t.Fatalf("expected 2 pending jobs in chain preview, got %d", len(payload.PendingJobs))
	}
}

func TestPipelineRunSelectionOfflineCachedExecutionMode(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	loadPipelineTestConfig(t, s, `
version: 1
project:
  name: ciwi
pipelines:
  - id: release
    trigger: manual
    vcs_source:
      repo: https://github.com/acme/repo.git
      ref: refs/heads/main
    versioning:
      file: VERSION
      tag_prefix: v
    jobs:
      - id: publish
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 30
        steps:
          - run: echo publish
`)
	pipelineID, _ := firstPipelineAndChainIDs(t, s, "ciwi")
	prev, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:         "echo previous",
		TimeoutSeconds: 30,
		Metadata: map[string]string{
			"project":                      "ciwi",
			"pipeline_id":                  "release",
			"pipeline_run_id":              "run-release-prev",
			"pipeline_version_raw":         "1.2.3",
			"pipeline_version":             "v1.2.3",
			"pipeline_source_repo":         "https://github.com/acme/repo.git",
			"pipeline_source_ref_raw":      "refs/heads/main",
			"pipeline_source_ref_resolved": "0123456789abcdef0123456789abcdef01234567",
		},
	})
	if err != nil {
		t.Fatalf("create previous job execution: %v", err)
	}
	if _, err := s.db.UpdateJobExecutionStatus(prev.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID: "agent-1",
		Status:  protocol.JobExecutionStatusSucceeded,
	}); err != nil {
		t.Fatalf("mark previous job succeeded: %v", err)
	}

	runResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/pipelines/"+int64ToString(pipelineID)+"/run-selection", map[string]any{
		"execution_mode": "offline_cached",
	})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for offline_cached run-selection, got %d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	var payload struct {
		Enqueued int `json:"enqueued"`
	}
	decodeJSONBody(t, runResp, &payload)
	if payload.Enqueued != 1 {
		t.Fatalf("expected one enqueued offline_cached job, got %d", payload.Enqueued)
	}
}
