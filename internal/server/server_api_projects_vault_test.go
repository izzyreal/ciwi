package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
)

func TestProjectsAndVaultAPI(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	cfg, err := config.Parse([]byte(testConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse test config: %v", err)
	}
	if err := s.db.LoadConfig(cfg, "ciwi-project.yaml", "https://github.com/izzyreal/ciwi.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}

	projectsResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects", nil)
	if projectsResp.StatusCode != http.StatusOK {
		t.Fatalf("list projects status=%d body=%s", projectsResp.StatusCode, readBody(t, projectsResp))
	}
	var projectsPayload struct {
		Projects []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"projects"`
	}
	decodeJSONBody(t, projectsResp, &projectsPayload)
	if len(projectsPayload.Projects) != 1 || projectsPayload.Projects[0].Name != "ciwi" {
		t.Fatalf("unexpected projects payload: %+v", projectsPayload)
	}
	projectID := projectsPayload.Projects[0].ID

	detailResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/"+int64ToString(projectID), nil)
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("project detail status=%d body=%s", detailResp.StatusCode, readBody(t, detailResp))
	}

	if !matchesETag(`"abc", "def"`, `"def"`) {
		t.Fatalf("expected matchesETag true")
	}
	if matchesETag(`"abc"`, `"def"`) {
		t.Fatalf("expected matchesETag false")
	}

	badCreate := mustRawJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/vault/connections", `{}`)
	if badCreate.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected bad vault create request to fail, got %d", badCreate.StatusCode)
	}

	vaultAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/auth/approle/login"):
			_, _ = w.Write([]byte(`{"auth":{"client_token":"token-123456","lease_duration":3600}}`))
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v1/kv/data/ciwi"):
			_, _ = w.Write([]byte(`{"data":{"data":{"token":"ghp_secret"}}}`))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer vaultAPI.Close()
	t.Setenv("CIWI_VAULT_SECRET_ID", "sid-123")

	createResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/vault/connections", map[string]any{
		"name":          "main-vault",
		"url":           vaultAPI.URL,
		"auth_method":   "approle",
		"approle_mount": "approle",
		"role_id":       "role-1",
		"secret_id_env": "CIWI_VAULT_SECRET_ID",
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create vault connection status=%d body=%s", createResp.StatusCode, readBody(t, createResp))
	}
	var createPayload struct {
		Connection struct {
			ID int64 `json:"id"`
		} `json:"connection"`
	}
	decodeJSONBody(t, createResp, &createPayload)
	if createPayload.Connection.ID <= 0 {
		t.Fatalf("expected created vault connection id, got %+v", createPayload)
	}
	connID := createPayload.Connection.ID

	listResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/vault/connections", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list vault connections status=%d body=%s", listResp.StatusCode, readBody(t, listResp))
	}

	methodResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/vault/connections/"+int64ToString(connID), nil)
	if methodResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected method-not-allowed for GET by id, got %d", methodResp.StatusCode)
	}

	connTestResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/vault/connections/"+int64ToString(connID)+"/test", map[string]any{
		"test_secret": map[string]any{
			"name": "github_token",
			"path": "ciwi",
			"key":  "token",
		},
	})
	if connTestResp.StatusCode != http.StatusOK {
		t.Fatalf("vault connection test status=%d body=%s", connTestResp.StatusCode, readBody(t, connTestResp))
	}
	var connTestPayload struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	decodeJSONBody(t, connTestResp, &connTestPayload)
	if !connTestPayload.OK || !strings.Contains(connTestPayload.Message, "vault auth ok") {
		t.Fatalf("unexpected vault connection test response: %+v", connTestPayload)
	}

	projectVaultGet := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/projects/"+int64ToString(projectID)+"/vault", nil)
	if projectVaultGet.StatusCode != http.StatusNotFound {
		t.Fatalf("expected project vault endpoint to be removed, got status=%d body=%s", projectVaultGet.StatusCode, readBody(t, projectVaultGet))
	}

	projectVaultTest := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects/"+int64ToString(projectID)+"/vault-test", map[string]any{})
	if projectVaultTest.StatusCode != http.StatusNotFound {
		t.Fatalf("expected project vault-test endpoint to be removed, got status=%d body=%s", projectVaultTest.StatusCode, readBody(t, projectVaultTest))
	}

	deleteResp := mustJSONRequest(t, ts.Client(), http.MethodDelete, ts.URL+"/api/v1/vault/connections/"+int64ToString(connID), nil)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete vault connection status=%d body=%s", deleteResp.StatusCode, readBody(t, deleteResp))
	}

	notFoundDelete := mustJSONRequest(t, ts.Client(), http.MethodDelete, ts.URL+"/api/v1/vault/connections/"+int64ToString(connID), nil)
	if notFoundDelete.StatusCode != http.StatusNotFound {
		t.Fatalf("expected deleting missing vault connection to return 404, got %d", notFoundDelete.StatusCode)
	}

	invalidID := mustJSONRequest(t, ts.Client(), http.MethodDelete, ts.URL+"/api/v1/vault/connections/not-a-number", nil)
	if invalidID.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid id to return 400, got %d", invalidID.StatusCode)
	}

	notFoundPath := mustJSONRequest(t, ts.Client(), http.MethodPost, fmt.Sprintf("%s/api/v1/vault/connections/%d/nope", ts.URL, connID), map[string]any{})
	if notFoundPath.StatusCode != http.StatusNotFound {
		t.Fatalf("expected unknown subpath to return 404, got %d", notFoundPath.StatusCode)
	}
}
