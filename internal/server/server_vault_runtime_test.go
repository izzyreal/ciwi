package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
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

func TestSealedVaultKeepsSecretJobQueuedWithSchedulingReason(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	var sealed atomic.Bool
	sealed.Store(true)
	vaultAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sealed.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"errors":["Vault is sealed"]}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/auth/approle/login"):
			_, _ = w.Write([]byte(`{"auth":{"client_token":"token-123","lease_duration":3600}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v1/kv/data/ciwi"):
			_, _ = w.Write([]byte(`{"data":{"data":{"identity":"Developer ID Application"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer vaultAPI.Close()

	t.Setenv("CIWI_VAULT_SECRET_ID", "sid-1")
	conn, err := s.db.UpsertVaultConnection(protocol.UpsertVaultConnectionRequest{
		Name: "home-vault", URL: vaultAPI.URL, AuthMethod: "approle", AppRoleMount: "approle",
		RoleID: "role-1", SecretIDEnv: "CIWI_VAULT_SECRET_ID", KVDefaultMount: "kv", KVDefaultVer: 2,
	})
	if err != nil {
		t.Fatalf("upsert vault connection: %v", err)
	}
	job, err := s.db.CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               "test -n \"$DEV_IDENTITY_APP\"",
		RequiredCapabilities: map[string]string{"os": "darwin", "arch": "arm64", "shell": "posix"},
		StepPlan: []protocol.JobStepPlanItem{{
			Name: "Check credentials", Script: "test -n \"$DEV_IDENTITY_APP\"", VaultConnection: conn.Name,
			VaultSecrets: []protocol.ProjectSecretSpec{{Name: "dev-identity-app", Path: "ciwi", Key: "identity"}},
			Env:          map[string]string{"DEV_IDENTITY_APP": "{{ secret.dev-identity-app }}"},
		}},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	s.mu.Lock()
	s.agents["agent-mac"] = agentState{
		Hostname: "mac", OS: "darwin", Arch: "arm64", Version: currentVersion(), Authorized: true,
		Capabilities: map[string]string{"executor": "script", "shells": "posix", "os": "darwin", "arch": "arm64"},
		LastSeenUTC:  time.Now().UTC(),
	}
	s.mu.Unlock()

	lease := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-mac", "capabilities": map[string]string{"executor": "script", "shells": "posix", "os": "darwin", "arch": "arm64"},
	})
	if lease.StatusCode != http.StatusOK {
		t.Fatalf("lease status=%d body=%s", lease.StatusCode, readBody(t, lease))
	}
	var leasePayload struct {
		Assigned bool   `json:"assigned"`
		Message  string `json:"message"`
	}
	decodeJSONBody(t, lease, &leasePayload)
	if leasePayload.Assigned || !strings.Contains(leasePayload.Message, "Vault is sealed") {
		t.Fatalf("sealed Vault lease response = %+v", leasePayload)
	}

	jobResponse := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/jobs/"+job.ID, nil)
	if jobResponse.StatusCode != http.StatusOK {
		t.Fatalf("job status=%d body=%s", jobResponse.StatusCode, readBody(t, jobResponse))
	}
	var jobPayload struct {
		Job struct {
			Status              string                      `json:"status"`
			LeasedByAgentID     string                      `json:"leased_by_agent_id"`
			SchedulingDiagnosis *domain.SchedulingDiagnosis `json:"scheduling_diagnosis"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, jobResponse, &jobPayload)
	if jobPayload.Job.Status != protocol.JobExecutionStatusQueued || jobPayload.Job.LeasedByAgentID != "" {
		t.Fatalf("blocked job lifecycle = %+v", jobPayload.Job)
	}
	if jobPayload.Job.SchedulingDiagnosis == nil || !strings.Contains(jobPayload.Job.SchedulingDiagnosis.Summary, "Vault is sealed") {
		t.Fatalf("scheduling diagnosis = %+v", jobPayload.Job.SchedulingDiagnosis)
	}

	sealed.Store(false)
	if _, err := s.db.MergeJobExecutionMetadata(job.ID, map[string]string{
		protocol.JobSchedulingRetryUTCMetadataKey: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("expire retry blocker: %v", err)
	}
	retry := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/agent/lease", map[string]any{
		"agent_id": "agent-mac", "capabilities": map[string]string{"executor": "script", "shells": "posix", "os": "darwin", "arch": "arm64"},
	})
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("retry lease status=%d body=%s", retry.StatusCode, readBody(t, retry))
	}
	var retryPayload struct {
		Assigned bool `json:"assigned"`
		Job      struct {
			ID       string            `json:"id"`
			Metadata map[string]string `json:"metadata"`
		} `json:"job_execution"`
	}
	decodeJSONBody(t, retry, &retryPayload)
	if !retryPayload.Assigned || retryPayload.Job.ID != job.ID {
		t.Fatalf("unsealed Vault retry response = %+v", retryPayload)
	}
	if retryPayload.Job.Metadata[protocol.JobSchedulingBlockedMetadataKey] != "" {
		t.Fatalf("scheduling blocker was not cleared: %+v", retryPayload.Job.Metadata)
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

func TestResolveJobSecretsDryRunSkipsVaultResolutionForSkippedSteps(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	job := protocol.JobExecution{
		Metadata: map[string]string{
			"dry_run": "1",
		},
		StepPlan: []protocol.JobStepPlanItem{{
			Kind:            "dryrun_skip",
			Script:          "echo dry-run",
			VaultConnection: "missing-vault",
			VaultSecrets: []protocol.ProjectSecretSpec{{
				Name: "github_token",
				Path: "ciwi",
				Key:  "token",
			}},
			Env: map[string]string{
				"GITHUB_TOKEN": "{{secret.github_token}}",
			},
		}},
	}
	if err := s.resolveJobSecrets(context.Background(), &job); err != nil {
		t.Fatalf("resolveJobSecrets dry-run: %v", err)
	}
	if got := job.StepPlan[0].Env["GITHUB_TOKEN"]; got != "{{secret.github_token}}" {
		t.Fatalf("expected dry-run secret placeholder to remain unchanged, got %q", got)
	}
	if job.Metadata["has_secrets"] != "" {
		t.Fatalf("expected dry-run to skip has_secrets marking, got %q", job.Metadata["has_secrets"])
	}
	if len(job.SensitiveValues) != 0 {
		t.Fatalf("expected no sensitive values for dry-run, got %+v", job.SensitiveValues)
	}
}

func TestResolveJobSecretsDryRunStillResolvesNonSkippedSteps(t *testing.T) {
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
		Metadata: map[string]string{
			"dry_run": "1",
		},
		StepPlan: []protocol.JobStepPlanItem{{
			Kind:            "run",
			Script:          "echo dry-run",
			VaultConnection: conn.Name,
			VaultSecrets: []protocol.ProjectSecretSpec{{
				Name: "github_token",
				Path: "ciwi",
				Key:  "token",
			}},
			Env: map[string]string{
				"GITHUB_TOKEN": "{{secret.github_token}}",
			},
		}},
	}
	if err := s.resolveJobSecrets(context.Background(), &job); err != nil {
		t.Fatalf("resolveJobSecrets dry-run active step: %v", err)
	}
	if got := job.StepPlan[0].Env["GITHUB_TOKEN"]; got != "secret-value" {
		t.Fatalf("expected dry-run active step secret to resolve, got %q", got)
	}
	if job.Metadata["has_secrets"] != "1" {
		t.Fatalf("expected has_secrets metadata flag for active dry-run step")
	}
	if len(job.SensitiveValues) != 1 || job.SensitiveValues[0] != "secret-value" {
		t.Fatalf("unexpected sensitive values: %+v", job.SensitiveValues)
	}
}
