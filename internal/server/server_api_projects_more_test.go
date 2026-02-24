package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/project"
	"github.com/izzyreal/ciwi/internal/version"
)

func TestProjectIconHandlerETagFlow(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	cfg, err := config.Parse([]byte(testConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse test config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	projectSummary, err := s.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	s.setProjectIcon(projectSummary.ID, "image/png", []byte{0x89, 0x50, 0x4e, 0x47})

	iconURL := ts.URL + "/api/v1/projects/" + int64ToString(projectSummary.ID) + "/icon"
	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, iconURL, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("icon get status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	etag := strings.TrimSpace(resp.Header.Get("ETag"))
	if etag == "" {
		t.Fatalf("expected ETag header")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("unexpected content type: %q", ct)
	}
	_ = readBody(t, resp)

	notModifiedReq, err := http.NewRequest(http.MethodGet, iconURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	notModifiedReq.Header.Set("If-None-Match", `"nope", `+etag)
	notModifiedResp, err := ts.Client().Do(notModifiedReq)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if notModifiedResp.StatusCode != http.StatusNotModified {
		t.Fatalf("expected 304 when ETag matches, got %d", notModifiedResp.StatusCode)
	}
	_ = readBody(t, notModifiedResp)

	wrongMethod := mustJSONRequest(t, ts.Client(), http.MethodPost, iconURL, map[string]any{})
	if wrongMethod.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST icon, got %d", wrongMethod.StatusCode)
	}
}

func TestProjectReloadHandlerBranches(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	cfg, err := config.Parse([]byte(testConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse test config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	projectSummary, err := s.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	reloadURL := ts.URL + "/api/v1/projects/" + int64ToString(projectSummary.ID) + "/reload"

	methodResp := mustJSONRequest(t, ts.Client(), http.MethodGet, reloadURL, nil)
	if methodResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET reload, got %d", methodResp.StatusCode)
	}

	oldFetch := fetchProjectConfigAndIcon
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		return project.RepoFetchResult{}, context.DeadlineExceeded
	}
	fetchErrResp := mustJSONRequest(t, ts.Client(), http.MethodPost, reloadURL, map[string]any{})
	if fetchErrResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when reload fetch fails, got %d body=%s", fetchErrResp.StatusCode, readBody(t, fetchErrResp))
	}

	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		return project.RepoFetchResult{
			ConfigContent:   testConfigYAML,
			SourceCommit:    "abc123",
			IconContentType: "image/svg+xml",
			IconContentBytes: []byte("<svg xmlns='http://www.w3.org/2000/svg' width='16' height='16'>" +
				"<rect width='16' height='16' fill='black'/></svg>"),
		}, nil
	}
	okResp := mustJSONRequest(t, ts.Client(), http.MethodPost, reloadURL, map[string]any{})
	if okResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for reload success, got %d body=%s", okResp.StatusCode, readBody(t, okResp))
	}
}

func TestProjectDeleteHandler(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	cfg, err := config.Parse([]byte(testConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse test config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	projectSummary, err := s.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	projectURL := ts.URL + "/api/v1/projects/" + int64ToString(projectSummary.ID)

	methodResp := mustJSONRequest(t, ts.Client(), http.MethodPut, projectURL, map[string]any{})
	if methodResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for PUT project root, got %d", methodResp.StatusCode)
	}

	delResp := mustJSONRequest(t, ts.Client(), http.MethodDelete, projectURL, nil)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for delete, got %d body=%s", delResp.StatusCode, readBody(t, delResp))
	}

	afterGet := mustJSONRequest(t, ts.Client(), http.MethodGet, projectURL, nil)
	if afterGet.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d body=%s", afterGet.StatusCode, readBody(t, afterGet))
	}

	delAgain := mustJSONRequest(t, ts.Client(), http.MethodDelete, projectURL, nil)
	if delAgain.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on delete missing project, got %d body=%s", delAgain.StatusCode, readBody(t, delAgain))
	}
}

func TestImportProjectHandlerValidationBranches(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	methodResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/import", nil)
	if methodResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET import, got %d", methodResp.StatusCode)
	}

	badJSON := mustRawJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", `{"repo_url":`)
	if badJSON.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", badJSON.StatusCode)
	}

	missingRepo := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{})
	if missingRepo.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing repo_url, got %d", missingRepo.StatusCode)
	}

	nonRootConfig := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{
		"repo_url":    "https://github.com/izzyreal/ciwi.git",
		"config_file": filepath.Join("configs", "ciwi-project.yaml"),
	})
	if nonRootConfig.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for nested config_file, got %d", nonRootConfig.StatusCode)
	}
}

func TestImportProjectHandlerFetchFailureAndSuccess(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	oldFetch := fetchProjectConfigAndIcon
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })

	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		return project.RepoFetchResult{}, context.DeadlineExceeded
	}
	failResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{
		"repo_url": "https://github.com/izzyreal/ciwi.git",
	})
	if failResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when import fetch fails, got %d body=%s", failResp.StatusCode, readBody(t, failResp))
	}

	var gotConfigFile string
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		gotConfigFile = configFile
		return project.RepoFetchResult{
			ConfigContent:   testConfigYAML,
			SourceCommit:    "deadbeef",
			IconContentType: "image/svg+xml",
			IconContentBytes: []byte("<svg xmlns='http://www.w3.org/2000/svg' width='8' height='8'>" +
				"<rect width='8' height='8' fill='black'/></svg>"),
		}, nil
	}
	okResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{
		"repo_url": "https://github.com/izzyreal/ciwi.git",
		"repo_ref": "main",
	})
	if okResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for import success, got %d body=%s", okResp.StatusCode, readBody(t, okResp))
	}
	if gotConfigFile != "ciwi-project.yaml" {
		t.Fatalf("expected default config_file to be ciwi-project.yaml, got %q", gotConfigFile)
	}
	var payload protocol.ImportProjectResponse
	if err := json.NewDecoder(okResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if payload.ProjectName != "ciwi" || payload.Pipelines != 1 {
		t.Fatalf("unexpected import response payload: %+v", payload)
	}
	projectSummary, err := s.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("GetProjectByName after import: %v", err)
	}
	if projectSummary.LoadedCommit != "deadbeef" {
		t.Fatalf("expected loaded commit persisted, got %q", projectSummary.LoadedCommit)
	}
}

func TestImportProjectSameRepoDifferentRefDoesNotReplaceExistingProject(t *testing.T) {
	ts, _ := newTestHTTPServerWithState(t)
	defer ts.Close()

	oldFetch := fetchProjectConfigAndIcon
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		return project.RepoFetchResult{
			ConfigContent: testConfigYAML,
			SourceCommit:  "deadbeef",
		}, nil
	}
	mainResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{
		"repo_url": "https://github.com/izzyreal/ciwi.git",
		"repo_ref": "main",
	})
	if mainResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for main import, got %d body=%s", mainResp.StatusCode, readBody(t, mainResp))
	}
	featureResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{
		"repo_url": "https://github.com/izzyreal/ciwi.git",
		"repo_ref": "feature/test",
	})
	if featureResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for feature import, got %d body=%s", featureResp.StatusCode, readBody(t, featureResp))
	}
	var featurePayload protocol.ImportProjectResponse
	decodeJSONBody(t, featureResp, &featurePayload)
	if strings.TrimSpace(featurePayload.ProjectName) != "ciwi" {
		t.Fatalf("expected display project name ciwi, got %q", featurePayload.ProjectName)
	}

	listResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", listResp.StatusCode, readBody(t, listResp))
	}
	var listPayload struct {
		Projects []protocol.ProjectSummary `json:"projects"`
	}
	decodeJSONBody(t, listResp, &listPayload)
	refs := map[string]bool{}
	for _, p := range listPayload.Projects {
		if strings.TrimSpace(p.RepoURL) != "https://github.com/izzyreal/ciwi.git" {
			continue
		}
		refs[strings.TrimSpace(p.RepoRef)] = true
	}
	if !refs["main"] || !refs["feature/test"] {
		t.Fatalf("expected both refs to exist without replacement, got refs=%v", refs)
	}
}

func TestImportProjectWithoutRepoRefPersistsResolvedDefaultBranch(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	oldFetch := fetchProjectConfigAndIcon
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		return project.RepoFetchResult{
			ConfigContent: testConfigYAML,
			SourceCommit:  "deadbeef",
			ResolvedRef:   "main",
		}, nil
	}

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/import", map[string]any{
		"repo_url": "https://github.com/izzyreal/ciwi.git",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for import, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload protocol.ImportProjectResponse
	decodeJSONBody(t, resp, &payload)
	if strings.TrimSpace(payload.RepoRef) != "main" {
		t.Fatalf("expected resolved repo_ref main in response, got %q", payload.RepoRef)
	}

	projectSummary, err := s.db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("GetProjectByName after import: %v", err)
	}
	if strings.TrimSpace(projectSummary.RepoRef) != "main" {
		t.Fatalf("expected stored project repo_ref main, got %q", projectSummary.RepoRef)
	}
}

func TestDetectServerUpdateCapabilityModes(t *testing.T) {
	oldVersion := version.Version
	t.Cleanup(func() { version.Version = oldVersion })
	version.Version = "v0.1.0"

	devCap := detectServerUpdateCapabilityWith(func() bool { return true }, func() bool { return false })
	if devCap.Mode != "dev" || devCap.Supported {
		t.Fatalf("unexpected dev capability: %+v", devCap)
	}

	serviceCap := detectServerUpdateCapabilityWith(func() bool { return false }, func() bool { return true })
	if serviceCap.Mode != "service" || !serviceCap.Supported {
		t.Fatalf("unexpected service capability: %+v", serviceCap)
	}

	standaloneCap := detectServerUpdateCapabilityWith(func() bool { return false }, func() bool { return false })
	if standaloneCap.Mode != "standalone" || standaloneCap.Supported {
		t.Fatalf("unexpected standalone capability: %+v", standaloneCap)
	}
	if strings.TrimSpace(standaloneCap.Reason) == "" {
		t.Fatalf("expected standalone mode reason")
	}
}

func TestReloadProjectFromRepoBranches(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	err := s.reloadProjectFromRepo(context.Background(), protocol.ProjectSummary{
		Name:       "ciwi",
		ConfigFile: "ciwi-project.yaml",
	})
	if err == nil || !strings.Contains(err.Error(), "project has no repo_url configured") {
		t.Fatalf("expected missing repo_url error, got %v", err)
	}

	oldFetch := fetchProjectConfigAndIcon
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		return project.RepoFetchResult{}, context.DeadlineExceeded
	}
	err = s.reloadProjectFromRepo(context.Background(), protocol.ProjectSummary{
		Name:       "ciwi",
		RepoURL:    "https://github.com/izzyreal/ciwi.git",
		RepoRef:    "main",
		ConfigFile: "ciwi-project.yaml",
	})
	if err == nil {
		t.Fatalf("expected reload fetch failure")
	}

	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (project.RepoFetchResult, error) {
		return project.RepoFetchResult{
			ConfigContent: testConfigYAML,
			SourceCommit:  "feedcafe",
		}, nil
	}
	err = s.reloadProjectFromRepo(context.Background(), protocol.ProjectSummary{
		Name:       "ciwi",
		RepoURL:    "https://github.com/izzyreal/ciwi.git",
		RepoRef:    "main",
		ConfigFile: "ciwi-project.yaml",
	})
	if err != nil {
		t.Fatalf("reloadProjectFromRepo success: %v", err)
	}
}

func TestProjectInspectHandlerRawYAMLAndExecutorScript(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: inspect-project
pipelines:
  - id: release
    trigger: manual
    vcs_source:
      repo: https://github.com/acme/inspect-project.git
      ref: main
    jobs:
      - id: publish
        runs_on:
          executor: script
          shell: posix
          os: linux
        timeout_seconds: 60
        steps:
          - run: echo publish
            skip_dry_run: true
          - run: echo upload
            skip_dry_run: true
`), "inspect-project.yaml")
	if err != nil {
		t.Fatalf("parse test config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "inspect-project.yaml", "https://github.com/acme/inspect-project.git", "main", "inspect-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	projectSummary, err := s.db.GetProjectByName("inspect-project")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	p, err := s.db.GetPipelineByProjectAndID("inspect-project", "release")
	if err != nil {
		t.Fatalf("GetPipelineByProjectAndID: %v", err)
	}

	t.Run("method not allowed", func(t *testing.T) {
		resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/"+int64ToString(projectSummary.ID)+"/inspect", nil)
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d body=%s", resp.StatusCode, readBody(t, resp))
		}
	})

	t.Run("raw pipeline yaml", func(t *testing.T) {
		resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/"+int64ToString(projectSummary.ID)+"/inspect", map[string]any{
			"pipeline_db_id": p.DBID,
			"view":           "raw_yaml",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("raw inspect status=%d body=%s", resp.StatusCode, readBody(t, resp))
		}
		body := readBody(t, resp)
		if !strings.Contains(body, `"view":"raw_yaml"`) {
			t.Fatalf("expected raw view in response, got %s", body)
		}
		if !strings.Contains(body, "skip_dry_run: true") {
			t.Fatalf("expected raw yaml with skip_dry_run, got %s", body)
		}
	})

	t.Run("executor script dry-run all skipped uses placeholder", func(t *testing.T) {
		resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/"+int64ToString(projectSummary.ID)+"/inspect", map[string]any{
			"pipeline_db_id":  p.DBID,
			"pipeline_job_id": "publish",
			"dry_run":         true,
			"view":            "executor_script",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("script inspect status=%d body=%s", resp.StatusCode, readBody(t, resp))
		}
		body := readBody(t, resp)
		if !strings.Contains(body, `echo [dry-run] all steps skipped`) {
			t.Fatalf("expected placeholder executor script, got %s", body)
		}
	})
}

func TestProjectInspectHandlerSecretMappingsView(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	cfg, err := config.Parse([]byte(`
version: 1
project:
  name: inspect-secrets
pipelines:
  - id: release
    trigger: manual
    vcs_source:
      repo: https://github.com/acme/inspect-secrets.git
      ref: main
    jobs:
      - id: publish
        runs_on:
          executor: script
          shell: posix
          os: linux
        timeout_seconds: 60
        steps:
          - run: echo check
            vault:
              connection: home-vault
              secrets:
                - name: github-token
                  mount: kv
                  path: ciwi/gh
                  key: token
            env:
              GITHUB_TOKEN: "{{ secret.github-token }}"
`), "inspect-secrets.yaml")
	if err != nil {
		t.Fatalf("parse test config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "inspect-secrets.yaml", "https://github.com/acme/inspect-secrets.git", "main", "inspect-secrets.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	projectSummary, err := s.db.GetProjectByName("inspect-secrets")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	p, err := s.db.GetPipelineByProjectAndID("inspect-secrets", "release")
	if err != nil {
		t.Fatalf("GetPipelineByProjectAndID: %v", err)
	}

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/"+int64ToString(projectSummary.ID)+"/inspect", map[string]any{
		"pipeline_db_id":  p.DBID,
		"pipeline_job_id": "publish",
		"view":            "secret_mappings",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("secret mappings inspect status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload projectInspectResponse
	decodeJSONBody(t, resp, &payload)
	if payload.View != "secret_mappings" {
		t.Fatalf("expected secret_mappings view in response, got %+v", payload)
	}
	content := strings.TrimSpace(payload.Content)
	if !strings.Contains(content, "vault_connection: home-vault") {
		t.Fatalf("expected vault connection in mapping output, got %q", content)
	}
	if !strings.Contains(content, "github-token: mount=kv path=ciwi/gh key=token") {
		t.Fatalf("expected mapped secret details in output, got %q", content)
	}
}

func TestPersistImportedProjectParseError(t *testing.T) {
	_, s := newTestHTTPServerWithState(t)
	_, err := s.persistImportedProject(protocol.ImportProjectRequest{
		RepoURL:    "https://github.com/izzyreal/ciwi.git",
		RepoRef:    "main",
		ConfigFile: "ciwi-project.yaml",
	}, "not: [valid", "abc", "main", "", nil)
	if err == nil {
		t.Fatalf("expected parse error from persistImportedProject")
	}
}

func TestProjectByIDHandlerInvalidPaths(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	badID := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/not-a-number", nil)
	if badID.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid project id, got %d", badID.StatusCode)
	}

	tooManyParts := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/1/icon/extra", nil)
	if tooManyParts.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for over-nested project path, got %d", tooManyParts.StatusCode)
	}

	unknownSubpath := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/1/nope", nil)
	if unknownSubpath.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown project subpath, got %d", unknownSubpath.StatusCode)
	}
}
