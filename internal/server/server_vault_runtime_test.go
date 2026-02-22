package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestResolveJobSecretsAndVaultRuntime(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	vaultAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/auth/approle/login"):
			_, _ = w.Write([]byte(`{"auth":{"client_token":"token-123","lease_duration":3600}}`))
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v1/kv/data/ciwi"):
			_, _ = w.Write([]byte(`{"data":{"data":{"token":"secret-value"}}}`))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer vaultAPI.Close()

	t.Setenv("CIWI_VAULT_SECRET_ID", "sid-1")
	conn, err := s.db.UpsertVaultConnection(protocol.UpsertVaultConnectionRequest{
		Name:           "vault-main",
		URL:            vaultAPI.URL,
		AuthMethod:     "approle",
		AppRoleMount:   "approle",
		RoleID:         "role-1",
		SecretIDEnv:    "CIWI_VAULT_SECRET_ID",
		KVDefaultMount: "kv",
		KVDefaultVer:   2,
	})
	if err != nil {
		t.Fatalf("upsert vault connection: %v", err)
	}

	job := protocol.JobExecution{
		StepPlan: []protocol.JobStepPlanItem{{
			Name:            "release",
			Script:          "echo ok",
			VaultConnection: conn.Name,
			VaultSecrets: []protocol.ProjectSecretSpec{{
				Name: "github_token",
				Path: "ciwi",
				Key:  "token",
			}},
			Env: map[string]string{
				"GITHUB_TOKEN": "{{secret.github_token}}",
				"UNCHANGED":    "plain",
			},
		}},
	}
	if err := s.resolveJobSecrets(context.Background(), &job); err != nil {
		t.Fatalf("resolveJobSecrets: %v", err)
	}
	if got := job.StepPlan[0].Env["GITHUB_TOKEN"]; got != "secret-value" {
		t.Fatalf("unexpected resolved token: %q", got)
	}
	if got := job.StepPlan[0].Env["UNCHANGED"]; got != "plain" {
		t.Fatalf("unexpected plain env: %q", got)
	}
	if job.Metadata["has_secrets"] != "1" {
		t.Fatalf("expected has_secrets metadata flag")
	}
	if len(job.SensitiveValues) != 1 || job.SensitiveValues[0] != "secret-value" {
		t.Fatalf("unexpected sensitive values: %+v", job.SensitiveValues)
	}

	// Token should now be cached and still allow direct read.
	secret, err := s.readVaultSecret(context.Background(), conn, protocol.ProjectSecretSpec{Name: "github_token", Path: "ciwi", Key: "token"})
	if err != nil {
		t.Fatalf("readVaultSecret: %v", err)
	}
	if secret != "secret-value" {
		t.Fatalf("unexpected secret read result: %q", secret)
	}

	if _, err := s.getVaultToken(context.Background(), conn, ""); err != nil {
		t.Fatalf("getVaultToken: %v", err)
	}
}

func TestResolveJobSecretsNoopAndMissingSecret(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	// No placeholders should no-op.
	plainJob := protocol.JobExecution{Env: map[string]string{"A": "B"}}
	if err := s.resolveJobSecrets(context.Background(), &plainJob); err != nil {
		t.Fatalf("resolveJobSecrets plain: %v", err)
	}

	_, err := s.db.UpsertVaultConnection(protocol.UpsertVaultConnectionRequest{
		Name:         "vault-main",
		URL:          "http://127.0.0.1:1", // unused here due early missing-secret failure
		AuthMethod:   "approle",
		AppRoleMount: "approle",
		RoleID:       "role-1",
		SecretIDEnv:  "CIWI_VAULT_SECRET_ID",
	})
	if err != nil {
		t.Fatalf("upsert vault connection: %v", err)
	}

	job := protocol.JobExecution{
		StepPlan: []protocol.JobStepPlanItem{{
			Script:          "echo x",
			VaultConnection: "vault-main",
			VaultSecrets: []protocol.ProjectSecretSpec{{
				Name: "known",
				Path: "ciwi",
				Key:  "token",
			}},
			Env: map[string]string{"X": "{{secret.unknown}}"},
		}},
	}
	if err := s.resolveJobSecrets(context.Background(), &job); err == nil {
		t.Fatalf("expected resolveJobSecrets to fail for unknown secret")
	}
}
