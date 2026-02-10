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
	if !strings.Contains(rootHTML, `id="importProjectBtn"`) {
		t.Fatalf("root page missing import button")
	}
	if !strings.Contains(rootHTML, `<script src="/ui/shared.js"></script>`) {
		t.Fatalf("root page missing shared js include")
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
	if !strings.Contains(projectHTML, "loadProject()") {
		t.Fatalf("project page missing loadProject call")
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
	if !strings.Contains(jobHTML, "Output / Error") {
		t.Fatalf("job page missing output section")
	}
}
