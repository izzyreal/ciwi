package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/store"
)

const testConfigYAML = `
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
        timeout_seconds: 300
        matrix:
          include:
            - name: linux-amd64
              goos: linux
              goarch: amd64
            - name: windows-amd64
              goos: windows
              goarch: amd64
        steps:
          - run: mkdir -p dist
          - run: GOOS={{goos}} GOARCH={{goarch}} go build -o dist/ciwi-{{goos}}-{{goarch}} ./cmd/ciwi
`

func newTestHTTPServer(t *testing.T) *httptest.Server {
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
		agents:           make(map[string]agentState),
		agentUpdates:     make(map[string]string),
		agentToolRefresh: make(map[string]bool),
		agentRestarts:    make(map[string]bool),
		db:               db,
		artifactsDir:     artifactsDir,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/server-info", serverInfoHandler)
	mux.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	mux.HandleFunc("/api/v1/projects/import", s.importProjectHandler)
	mux.HandleFunc("/api/v1/projects/", s.projectByIDHandler)
	mux.HandleFunc("/api/v1/heartbeat", s.heartbeatHandler)
	mux.HandleFunc("/api/v1/agents", s.listAgentsHandler)
	mux.HandleFunc("/api/v1/agents/", s.agentByIDHandler)
	mux.HandleFunc("/api/v1/jobs", s.jobExecutionsHandler)
	mux.HandleFunc("/api/v1/jobs/", s.jobExecutionByIDHandler)
	mux.HandleFunc("/api/v1/jobs/clear-queue", s.clearJobExecutionQueueHandler)
	mux.HandleFunc("/api/v1/jobs/flush-history", s.flushJobExecutionHistoryHandler)
	mux.HandleFunc("/api/v1/pipelines/", s.pipelineByIDHandler)
	mux.HandleFunc("/api/v1/pipeline-chains/", s.pipelineChainByIDHandler)
	mux.HandleFunc("/api/v1/agent/lease", s.leaseJobHandler)
	mux.HandleFunc("/api/v1/vault/connections", s.vaultConnectionsHandler)
	mux.HandleFunc("/api/v1/vault/connections/", s.vaultConnectionByIDHandler)
	mux.HandleFunc("/api/v1/update/check", s.updateCheckHandler)
	mux.HandleFunc("/api/v1/update/apply", s.updateApplyHandler)
	mux.HandleFunc("/api/v1/update/rollback", s.updateRollbackHandler)
	mux.HandleFunc("/api/v1/server/restart", s.serverRestartHandler)
	mux.HandleFunc("/api/v1/update/tags", s.updateTagsHandler)
	mux.HandleFunc("/api/v1/update/status", s.updateStatusHandler)

	return httptest.NewServer(mux)
}

func mustJSONRequest(t *testing.T, client *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request JSON: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func mustRawJSONRequest(t *testing.T, client *http.Client, method, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeJSONBody(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("decode response body: %v, tail=%q", err, string(raw))
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
