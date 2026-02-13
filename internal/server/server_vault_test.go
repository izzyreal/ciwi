package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultConnectionAndProjectVaultFlow(t *testing.T) {
	t.Setenv("CIWI_TEST_VAULT_SECRET_ID", "sid-1")

	vault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/approle/login":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"auth":{"client_token":"tok-123","lease_duration":3600}}`))
		case "/v1/kv/data/gh":
			if r.Header.Get("X-Vault-Token") != "tok-123" {
				http.Error(w, "unauthorized", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"data":{"token":"ghs_secret"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer vault.Close()

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	saveConnResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/vault/connections", map[string]any{
		"name":               "home-vault",
		"url":                vault.URL,
		"auth_method":        "approle",
		"approle_mount":      "approle",
		"role_id":            "role-1",
		"secret_id_env":      "CIWI_TEST_VAULT_SECRET_ID",
		"kv_default_mount":   "kv",
		"kv_default_version": 2,
	})
	if saveConnResp.StatusCode != http.StatusCreated {
		t.Fatalf("save vault connection status=%d body=%s", saveConnResp.StatusCode, readBody(t, saveConnResp))
	}
	var connPayload struct {
		Connection struct {
			ID int64 `json:"id"`
		} `json:"connection"`
	}
	decodeJSONBody(t, saveConnResp, &connPayload)
	if connPayload.Connection.ID <= 0 {
		t.Fatalf("expected vault connection id > 0")
	}

	testConnResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/vault/connections/"+int64ToString(connPayload.Connection.ID)+"/test", map[string]any{
		"secret_id_override": "sid-1",
		"test_secret": map[string]any{
			"name":  "github_secret",
			"path":  "gh",
			"key":   "token",
			"mount": "kv",
		},
	})
	if testConnResp.StatusCode != http.StatusOK {
		t.Fatalf("test vault connection status=%d body=%s", testConnResp.StatusCode, readBody(t, testConnResp))
	}
	var testConnPayload struct {
		OK bool `json:"ok"`
	}
	decodeJSONBody(t, testConnResp, &testConnPayload)
	if !testConnPayload.OK {
		t.Fatalf("expected test connection ok")
	}

	tmp := t.TempDir()
	cfg := `
version: 1
project:
  name: vault-proj
  vault:
    connection: home-vault
    secrets:
      - name: github_secret
        mount: kv
        path: gh
        key: token
pipelines:
  - id: build
    source:
      repo: https://example.invalid/repo.git
    jobs:
      - id: j1
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo hello
            env:
              GITHUB_SECRET: "{{ secret.github_secret }}"
`
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

	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{"config_path": "ciwi.yaml"})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load config status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	var projects struct {
		Projects []struct {
			ID int64 `json:"id"`
		} `json:"projects"`
	}
	pResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if pResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", pResp.StatusCode, readBody(t, pResp))
	}
	decodeJSONBody(t, pResp, &projects)
	projectID := projects.Projects[0].ID

	if projectID <= 0 {
		t.Fatalf("expected project id > 0")
	}

	projectTestResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/projects/"+int64ToString(projectID)+"/vault-test", map[string]any{
		"secret_id_override": "sid-1",
	})
	if projectTestResp.StatusCode != http.StatusOK {
		t.Fatalf("project vault test status=%d body=%s", projectTestResp.StatusCode, readBody(t, projectTestResp))
	}
	var projectTest struct {
		OK bool `json:"ok"`
	}
	decodeJSONBody(t, projectTestResp, &projectTest)
	if !projectTest.OK {
		t.Fatalf("expected project vault test ok")
	}

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run pipeline status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	leaseResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-test",
		"capabilities": map[string]any{
			"os":   "linux",
			"arch": "amd64",
		},
	})
	if leaseResp.StatusCode != http.StatusOK {
		t.Fatalf("lease status=%d body=%s", leaseResp.StatusCode, readBody(t, leaseResp))
	}
	var leasePayload struct {
		Assigned bool `json:"assigned"`
		Job      *struct {
			Env             map[string]string `json:"env"`
			Metadata        map[string]string `json:"metadata"`
			SensitiveValues []string          `json:"sensitive_values"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, leaseResp, &leasePayload)
	if !leasePayload.Assigned || leasePayload.Job == nil {
		t.Fatalf("expected assigned job")
	}
	if got := leasePayload.Job.Env["GITHUB_SECRET"]; got != "ghs_secret" {
		t.Fatalf("expected resolved secret in env, got %q", got)
	}
	if leasePayload.Job.Metadata["has_secrets"] != "1" {
		t.Fatalf("expected metadata has_secrets=1")
	}
	if len(leasePayload.Job.SensitiveValues) == 0 || !strings.Contains(strings.Join(leasePayload.Job.SensitiveValues, ","), "ghs_secret") {
		t.Fatalf("expected sensitive values to include resolved secret")
	}
}

func TestVaultPageServed(t *testing.T) {
	ts := newTestHTTPServerWithUI(t)
	defer ts.Close()
	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/vault", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /vault status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "Vault Connections") {
		t.Fatalf("vault page title missing")
	}
	if !strings.Contains(body, "/api/v1/vault/connections") {
		t.Fatalf("vault page api wiring missing")
	}
}

func TestParseSecretPlaceholderRegex(t *testing.T) {
	matches := secretPlaceholderRE.FindAllStringSubmatch("x {{ secret.github_secret }} y", -1)
	raw, _ := json.Marshal(matches)
	if len(matches) != 1 || matches[0][1] != "github_secret" {
		t.Fatalf("unexpected placeholder parse: %s", string(raw))
	}
}

func TestLeaseSecretResolutionFailureMarksJobFailed(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	saveConnResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/vault/connections", map[string]any{
		"name":          "dummy-vault",
		"url":           "http://127.0.0.1:8200",
		"auth_method":   "approle",
		"approle_mount": "approle",
		"role_id":       "role-1",
		"secret_id_env": "CIWI_TEST_VAULT_SECRET_ID",
	})
	if saveConnResp.StatusCode != http.StatusCreated {
		t.Fatalf("save vault connection status=%d body=%s", saveConnResp.StatusCode, readBody(t, saveConnResp))
	}
	_ = readBody(t, saveConnResp)

	tmp := t.TempDir()
	cfg := `
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    source:
      repo: https://example.invalid/repo.git
    jobs:
      - id: unit
        runs_on:
          os: linux
          arch: amd64
        timeout_seconds: 60
        steps:
          - run: echo hello
            env:
              GITHUB_SECRET: "{{ secret.github-secret }}"
`
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

	loadResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/config/load", map[string]any{"config_path": "ciwi.yaml"})
	if loadResp.StatusCode != http.StatusOK {
		t.Fatalf("load config status=%d body=%s", loadResp.StatusCode, readBody(t, loadResp))
	}
	_ = readBody(t, loadResp)

	var projects struct {
		Projects []struct {
			ID int64 `json:"id"`
		} `json:"projects"`
	}
	pResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if pResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", pResp.StatusCode, readBody(t, pResp))
	}
	decodeJSONBody(t, pResp, &projects)
	projectID := projects.Projects[0].ID

	saveProjectVaultResp := mustJSONRequest(t, client, http.MethodPut, ts.URL+"/api/v1/projects/"+int64ToString(projectID)+"/vault", map[string]any{
		"vault_connection_name": "dummy-vault",
		"secrets": []map[string]any{
			{"name": "some-other-secret", "mount": "kv", "path": "gh", "key": "token"},
		},
	})
	if saveProjectVaultResp.StatusCode != http.StatusOK {
		t.Fatalf("save project vault status=%d body=%s", saveProjectVaultResp.StatusCode, readBody(t, saveProjectVaultResp))
	}
	_ = readBody(t, saveProjectVaultResp)

	runResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/pipelines/1/run", map[string]any{})
	if runResp.StatusCode != http.StatusCreated {
		t.Fatalf("run pipeline status=%d body=%s", runResp.StatusCode, readBody(t, runResp))
	}
	_ = readBody(t, runResp)

	leaseResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-test",
		"capabilities": map[string]any{
			"os":   "linux",
			"arch": "amd64",
		},
	})
	if leaseResp.StatusCode != http.StatusOK {
		t.Fatalf("lease status=%d body=%s", leaseResp.StatusCode, readBody(t, leaseResp))
	}
	var leasePayload struct {
		Assigned bool   `json:"assigned"`
		Message  string `json:"message"`
	}
	decodeJSONBody(t, leaseResp, &leasePayload)
	if leasePayload.Assigned {
		t.Fatalf("expected no assignment on secret resolution failure")
	}
	if !strings.Contains(leasePayload.Message, "secret resolution failed") {
		t.Fatalf("expected failure message, got %q", leasePayload.Message)
	}

	jobsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("list jobs status=%d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	var jobs struct {
		Jobs []struct {
			Status string `json:"status"`
		} `json:"job_executions"`
	}
	decodeJSONBody(t, jobsResp, &jobs)
	if len(jobs.Jobs) == 0 {
		t.Fatalf("expected at least one job")
	}
	if jobs.Jobs[0].Status != "failed" {
		t.Fatalf("expected failed job status, got %q", jobs.Jobs[0].Status)
	}
}
