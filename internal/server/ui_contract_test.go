package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/store"
)

func newTestHTTPServerWithUI(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/", s.uiHandler)
	mux.HandleFunc("/api/v1/config/load", s.loadConfigHandler)
	mux.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	mux.HandleFunc("/api/v1/projects/", s.projectByIDHandler)
	mux.HandleFunc("/api/v1/jobs", s.jobsHandler)
	mux.HandleFunc("/api/v1/jobs/", s.jobByIDHandler)
	mux.HandleFunc("/api/v1/pipelines/", s.pipelineByIDHandler)
	mux.HandleFunc("/api/v1/vault/connections", s.vaultConnectionsHandler)
	mux.HandleFunc("/api/v1/vault/connections/", s.vaultConnectionByIDHandler)
	mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", http.FileServer(http.Dir(artifactsDir))))

	return httptest.NewServer(mux)
}

func TestUIRootAndSharedJSServed(t *testing.T) {
	ts := newTestHTTPServerWithUI(t)
	defer ts.Close()

	client := ts.Client()

	resp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	rootHTML := readBody(t, resp)
	if !strings.Contains(rootHTML, "<h1>ciwi</h1>") {
		t.Fatalf("root page missing title")
	}
	if !strings.Contains(rootHTML, `<script src="/ui/shared.js"></script>`) {
		t.Fatalf("root page missing shared js include")
	}
	if !strings.Contains(rootHTML, `<script src="/ui/pages.js"></script>`) {
		t.Fatalf("root page missing pages js include")
	}
	if !strings.Contains(rootHTML, `href="/agents"`) {
		t.Fatalf("root page missing agents link")
	}
	if !strings.Contains(rootHTML, `href="/settings"`) {
		t.Fatalf("root page missing settings link")
	}
	if !strings.Contains(rootHTML, `<img src="/ciwi-logo.png"`) {
		t.Fatalf("root page missing header logo")
	}
	if !strings.Contains(rootHTML, `href="/ciwi-favicon.png"`) {
		t.Fatalf("root page missing favicon link")
	}
	if strings.Contains(rootHTML, "Output/Error") {
		t.Fatalf("root page should not show Output/Error overview column")
	}
	if !strings.Contains(rootHTML, "table-layout: fixed") || !strings.Contains(rootHTML, "overflow-wrap: anywhere") {
		t.Fatalf("root page missing log overflow containment CSS")
	}
	if strings.Contains(rootHTML, `id="importProjectBtn"`) {
		t.Fatalf("root page should not include project import controls")
	}
	if strings.Contains(rootHTML, `id="checkUpdatesBtn"`) {
		t.Fatalf("root page should not include update controls")
	}

	settingsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/settings", nil)
	if settingsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /settings status=%d body=%s", settingsResp.StatusCode, readBody(t, settingsResp))
	}
	settingsHTML := readBody(t, settingsResp)
	if !strings.Contains(settingsHTML, "<h1>ciwi settings</h1>") {
		t.Fatalf("settings page missing title")
	}
	if !strings.Contains(settingsHTML, `id="importProjectBtn"`) {
		t.Fatalf("settings page missing import button")
	}
	if !strings.Contains(settingsHTML, `id="checkUpdatesBtn"`) || !strings.Contains(settingsHTML, `id="applyUpdateBtn"`) {
		t.Fatalf("settings page missing update controls")
	}
	if !strings.Contains(settingsHTML, `href="/vault"`) {
		t.Fatalf("settings page missing vault link")
	}

	resp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/ui/shared.js", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/shared.js status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	js := readBody(t, resp)
	if !strings.Contains(js, "function formatTimestamp(") {
		t.Fatalf("shared js missing formatTimestamp helper")
	}
	if !strings.Contains(js, "function formatDuration(") {
		t.Fatalf("shared js missing formatDuration helper")
	}
	if !strings.Contains(js, "function jobDescription(") {
		t.Fatalf("shared js missing jobDescription helper")
	}
	if !strings.Contains(js, "function formatBytes(") {
		t.Fatalf("shared js missing formatBytes helper")
	}

	resp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/ui/pages.js", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/pages.js status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	pagesJS := readBody(t, resp)
	if !strings.Contains(pagesJS, "function apiJSON(") {
		t.Fatalf("pages js missing apiJSON helper")
	}
	if !strings.Contains(pagesJS, "function buildJobExecutionRow(") {
		t.Fatalf("pages js missing job row builder")
	}
	if !strings.Contains(pagesJS, "function openVersionResolveModal(") {
		t.Fatalf("pages js missing version resolve modal helper")
	}

	faviconResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/favicon.ico", nil)
	if faviconResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /favicon.ico status=%d body=%s", faviconResp.StatusCode, readBody(t, faviconResp))
	}
	_ = readBody(t, faviconResp)

	logoResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/ciwi-logo.png", nil)
	if logoResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ciwi-logo.png status=%d body=%s", logoResp.StatusCode, readBody(t, logoResp))
	}
	_ = readBody(t, logoResp)

	agentsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/agents", nil)
	if agentsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /agents status=%d body=%s", agentsResp.StatusCode, readBody(t, agentsResp))
	}
	agentsHTML := readBody(t, agentsResp)
	if !strings.Contains(agentsHTML, "<title>ciwi agents</title>") {
		t.Fatalf("agents page missing title")
	}
	if !strings.Contains(agentsHTML, "/api/v1/agents") {
		t.Fatalf("agents page missing agents API wiring")
	}

	agentDetailResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/agents/agent-test", nil)
	if agentDetailResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /agents/{id} status=%d body=%s", agentDetailResp.StatusCode, readBody(t, agentDetailResp))
	}
	agentDetailHTML := readBody(t, agentDetailResp)
	if !strings.Contains(agentDetailHTML, "<title>ciwi agent</title>") {
		t.Fatalf("agent detail page missing title")
	}
	if !strings.Contains(agentDetailHTML, "Agent Detail") {
		t.Fatalf("agent detail page missing header")
	}
	if !strings.Contains(agentDetailHTML, "/api/v1/agents/") {
		t.Fatalf("agent detail page missing agent API wiring")
	}
}

func TestUIProjectAndJobPagesServed(t *testing.T) {
	ts := newTestHTTPServerWithUI(t)
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
		t.Fatalf("load config status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run pipeline status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	projectResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/projects/1", nil)
	if projectResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /projects/1 status=%d body=%s", projectResp.StatusCode, readBody(t, projectResp))
	}
	projectHTML := readBody(t, projectResp)
	if !strings.Contains(projectHTML, "Execution History") {
		t.Fatalf("project page missing execution history section")
	}
	if !strings.Contains(projectHTML, `<img src="/ciwi-logo.png"`) {
		t.Fatalf("project page missing header logo")
	}
	if !strings.Contains(projectHTML, `href="/ciwi-favicon.png"`) {
		t.Fatalf("project page missing favicon link")
	}
	if strings.Contains(projectHTML, "Output/Error") {
		t.Fatalf("project page should not show Output/Error column")
	}
	if !strings.Contains(projectHTML, "loadProject()") {
		t.Fatalf("project page missing loadProject call")
	}
	if !strings.Contains(projectHTML, `<script src="/ui/pages.js"></script>`) {
		t.Fatalf("project page missing pages js include")
	}
	if !strings.Contains(projectHTML, "table-layout: fixed") || !strings.Contains(projectHTML, "overflow-wrap: anywhere") {
		t.Fatalf("project page missing log overflow containment CSS")
	}

	var jobsPayload struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	decodeJSONBody(t, jobsResp, &jobsPayload)
	if len(jobsPayload.Jobs) == 0 {
		t.Fatalf("expected at least one job after run")
	}

	jobResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/jobs/"+jobsPayload.Jobs[0].ID, nil)
	if jobResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /jobs/{id} status=%d body=%s", jobResp.StatusCode, readBody(t, jobResp))
	}
	jobHTML := readBody(t, jobResp)
	if !strings.Contains(jobHTML, `id="logBox"`) {
		t.Fatalf("job page missing log box")
	}
	if !strings.Contains(jobHTML, `<img src="/ciwi-logo.png"`) {
		t.Fatalf("job page missing header logo")
	}
	if !strings.Contains(jobHTML, `href="/ciwi-favicon.png"`) {
		t.Fatalf("job page missing favicon link")
	}
	if !strings.Contains(jobHTML, "Output / Error") {
		t.Fatalf("job page missing output section")
	}
	if !strings.Contains(jobHTML, "formatBytes(a.size_bytes)") {
		t.Fatalf("job page should render human-friendly artifact sizes")
	}
	if !strings.Contains(jobHTML, "artifact-path") || !strings.Contains(jobHTML, "copy-btn") {
		t.Fatalf("job page should support artifact text selection/copy")
	}
	if strings.Contains(jobHTML, "['Status',") {
		t.Fatalf("job page should not duplicate status in meta rows")
	}
}
