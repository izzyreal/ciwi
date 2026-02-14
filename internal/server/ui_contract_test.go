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
	mux.HandleFunc("/api/v1/projects/import", s.importProjectHandler)
	mux.HandleFunc("/api/v1/projects/", s.projectByIDHandler)
	mux.HandleFunc("/api/v1/jobs", s.jobExecutionsHandler)
	mux.HandleFunc("/api/v1/jobs/", s.jobExecutionByIDHandler)
	mux.HandleFunc("/api/v1/pipelines/", s.pipelineByIDHandler)
	mux.HandleFunc("/api/v1/pipeline-chains/", s.pipelineChainByIDHandler)
	mux.HandleFunc("/api/v1/vault/connections", s.vaultConnectionsHandler)
	mux.HandleFunc("/api/v1/vault/connections/", s.vaultConnectionByIDHandler)
	mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", http.FileServer(http.Dir(artifactsDir))))

	return httptest.NewServer(mux)
}

func requireContainsAll(t *testing.T, content, subject string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if strings.Contains(content, needle) {
			continue
		}
		t.Fatalf("%s missing %q", subject, needle)
	}
}

func requireNotContainsAll(t *testing.T, content, subject string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(content, needle) {
			continue
		}
		t.Fatalf("%s should not contain %q", subject, needle)
	}
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
	requireContainsAll(t, rootHTML, "root page",
		"<h1>ciwi</h1>",
		`<script src="/ui/shared.js"></script>`,
		`<script src="/ui/pages.js"></script>`,
		`href="/agents"`,
		`href="/settings"`,
		`<img src="/ciwi-logo.png"`,
		`href="/ciwi-favicon.png"`,
		`id="projects"`,
		`id="clearQueueBtn"`,
		`id="queuedJobsBody"`,
		`id="flushHistoryBtn"`,
		`id="historyJobsBody"`,
		"/api/v1/projects",
		"/api/v1/jobs?view=summary",
		"/api/v1/jobs/clear-queue",
		"/api/v1/jobs/flush-history",
		"/api/v1/pipelines/",
	)
	requireNotContainsAll(t, rootHTML, "root page",
		"Output/Error",
		`id="importProjectBtn"`,
		`id="checkUpdatesBtn"`,
	)

	settingsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/settings", nil)
	if settingsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /settings status=%d body=%s", settingsResp.StatusCode, readBody(t, settingsResp))
	}
	settingsHTML := readBody(t, settingsResp)
	requireContainsAll(t, settingsHTML, "settings page",
		"<h1>ciwi settings</h1>",
		`id="importProjectBtn"`,
		`id="checkUpdatesBtn"`,
		`id="applyUpdateBtn"`,
		`id="rollbackTagSelect"`,
		`id="rollbackUpdateBtn"`,
		`id="openVaultConnectionsBtn"`,
		`window.location.href = '/vault';`,
		"/api/v1/projects/import",
		"/api/v1/projects/",
		"/reload",
		"/api/v1/update/check",
		"/api/v1/update/apply",
		"/api/v1/update/rollback",
		"/api/v1/update/tags",
		"/api/v1/update/status",
	)

	resp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/ui/shared.js", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/shared.js status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	js := readBody(t, resp)
	requireContainsAll(t, js, "shared js",
		"function formatTimestamp(",
		"function formatDurationMs(",
		"function jobDescription(",
		"function formatBytes(",
		"function createRefreshGuard(",
		"Adhoc script",
	)
	requireNotContainsAll(t, js, "shared js",
		"function formatDuration(",
		"0001-01-01T00:00:00",
	)

	resp = mustJSONRequest(t, client, http.MethodGet, ts.URL+"/ui/pages.js", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ui/pages.js status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	pagesJS := readBody(t, resp)
	requireContainsAll(t, pagesJS, "pages js",
		"function apiJSON(",
		"cache: 'no-store'",
		"function buildJobExecutionRow(",
		"function openVersionResolveModal(",
		"versionResolveModal",
		"/api/v1/pipelines/",
		"/version-resolve",
	)

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
	requireContainsAll(t, agentsHTML, "agents page",
		"<title>ciwi agents</title>",
		`id="refreshBtn"`,
		`id="rows"`,
		"/api/v1/agents",
	)

	agentDetailResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/agents/agent-test", nil)
	if agentDetailResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /agents/{id} status=%d body=%s", agentDetailResp.StatusCode, readBody(t, agentDetailResp))
	}
	agentDetailHTML := readBody(t, agentDetailResp)
	requireContainsAll(t, agentDetailHTML, "agent detail page",
		"<title>ciwi agent</title>",
		"Agent Detail",
		"Run Adhoc Script",
		"/api/v1/agents/",
		"/run-script",
	)
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
	requireContainsAll(t, projectHTML, "project page",
		"Structure",
		"Vault Access",
		"Execution History",
		`<img src="/ciwi-logo.png"`,
		`href="/ciwi-favicon.png"`,
		`<script src="/ui/pages.js"></script>`,
		`id="structure"`,
		`id="vaultConnectionSelect"`,
		`id="saveVaultBtn"`,
		`id="testVaultBtn"`,
		`id="vaultSecretsText"`,
		`id="historyBody"`,
		"/api/v1/projects/",
		"/api/v1/vault/connections",
		"/api/v1/jobs",
		"/api/v1/pipelines/",
		"/run-selection",
		"/vault-test",
		"openVersionResolveModal(",
	)
	requireNotContainsAll(t, projectHTML, "project page", "Output/Error")

	var jobsPayload struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"job_executions"`
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
	jobExecutionHTML := readBody(t, jobResp)
	requireContainsAll(t, jobExecutionHTML, "job page",
		`id="jobTitle"`,
		`id="subtitle"`,
		`id="metaGrid"`,
		`id="logBox"`,
		`id="artifactsBox"`,
		`id="testReportBox"`,
		"Job Execution ID",
		"Project",
		"Job ID",
		"Output / Error",
		"Artifacts",
		"Test Report",
		`<img src="/ciwi-logo.png"`,
		`href="/ciwi-favicon.png"`,
		"/api/v1/jobs/",
		"/artifacts",
		"/tests",
		"/force-fail",
		"cache: 'no-store'",
		"formatBytes(",
	)
	requireNotContainsAll(t, jobExecutionHTML, "job page", "0001-01-01T00:00:00")
}
