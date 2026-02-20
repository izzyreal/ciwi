package server

import (
	"context"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
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

func TestDetectServerUpdateCapabilityModes(t *testing.T) {
	oldVersion := version.Version
	t.Cleanup(func() { version.Version = oldVersion })

	version.Version = "dev"
	devCap := detectServerUpdateCapability()
	if devCap.Mode != "dev" || devCap.Supported {
		t.Fatalf("unexpected dev capability: %+v", devCap)
	}

	version.Version = "v0.1.0"
	if isServerRunningInDevMode() {
		t.Skip("test binary runtime matches dev-mode heuristic; service/standalone branches are not reachable here")
	}
	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "abc")
	case "darwin":
		t.Setenv("LAUNCH_JOB_LABEL", "com.example.ciwi")
	case "windows":
		t.Setenv("CIWI_SERVER_WINDOWS_SERVICE_NAME", "ciwi")
	}
	serviceCap := detectServerUpdateCapability()
	if serviceCap.Mode != "service" || !serviceCap.Supported {
		t.Fatalf("unexpected service capability: %+v", serviceCap)
	}

	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "")
	case "darwin":
		t.Setenv("LAUNCH_JOB_LABEL", "")
	case "windows":
		t.Setenv("CIWI_SERVER_WINDOWS_SERVICE_NAME", "")
	}
	standaloneCap := detectServerUpdateCapability()
	if standaloneCap.Mode != "standalone" || standaloneCap.Supported {
		t.Fatalf("unexpected standalone capability: %+v", standaloneCap)
	}
	if strings.TrimSpace(standaloneCap.Reason) == "" {
		t.Fatalf("expected standalone mode reason")
	}
}
