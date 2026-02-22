package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	servervault "github.com/izzyreal/ciwi/internal/server/vault"
)

var secretPlaceholderRE = regexp.MustCompile(`\{\{\s*secret\.([a-zA-Z0-9_\-]+)\s*\}\}`)

func (s *stateStore) resolveJobSecrets(ctx context.Context, job *protocol.JobExecution) error {
	if job == nil {
		return nil
	}

	for _, value := range job.Env {
		if len(secretPlaceholderRE.FindAllStringSubmatch(value, -1)) > 0 {
			return fmt.Errorf("secret placeholders are only supported in step env")
		}
	}

	connectionByName := map[string]protocol.VaultConnection{}
	sensitive := []string{}
	hasSecrets := false
	for i := range job.StepPlan {
		step := &job.StepPlan[i]
		if len(step.Env) == 0 {
			continue
		}
		needsStepSecrets := false
		for _, value := range step.Env {
			if len(secretPlaceholderRE.FindAllStringSubmatch(value, -1)) > 0 {
				needsStepSecrets = true
				break
			}
		}
		if !needsStepSecrets {
			continue
		}
		hasSecrets = true
		connName := strings.TrimSpace(step.VaultConnection)
		if connName == "" {
			return fmt.Errorf("step %d uses secret placeholders but has no vault.connection", i+1)
		}
		conn, ok := connectionByName[connName]
		if !ok {
			var err error
			conn, err = s.vaultStore().GetVaultConnectionByName(connName)
			if err != nil {
				return fmt.Errorf("resolve vault connection %q: %w", connName, err)
			}
			connectionByName[connName] = conn
		}
		secretMap := map[string]protocol.ProjectSecretSpec{}
		for _, spec := range step.VaultSecrets {
			secretMap[strings.TrimSpace(spec.Name)] = spec
		}
		resolvedStepEnv := cloneMap(step.Env)
		for key, value := range resolvedStepEnv {
			matches := secretPlaceholderRE.FindAllStringSubmatch(value, -1)
			if len(matches) == 0 {
				continue
			}
			out := value
			for _, m := range matches {
				if len(m) < 2 {
					continue
				}
				secretName := strings.TrimSpace(m[1])
				spec, ok := secretMap[secretName]
				if !ok {
					return fmt.Errorf("step %d secret %q is not configured in step vault.secrets", i+1, secretName)
				}
				secretValue, getErr := s.readVaultSecret(ctx, conn, spec)
				if getErr != nil {
					return fmt.Errorf("resolve step %d secret %q: %w", i+1, secretName, getErr)
				}
				out = strings.ReplaceAll(out, m[0], secretValue)
				sensitive = append(sensitive, secretValue)
			}
			resolvedStepEnv[key] = out
		}
		step.Env = resolvedStepEnv
	}

	if !hasSecrets {
		return nil
	}
	if job.Metadata == nil {
		job.Metadata = map[string]string{}
	}
	job.Metadata["has_secrets"] = "1"
	job.SensitiveValues = servervault.DedupeStrings(sensitive)
	return nil
}

func (s *stateStore) readVaultSecret(ctx context.Context, conn protocol.VaultConnection, spec protocol.ProjectSecretSpec) (string, error) {
	token, err := s.getVaultToken(ctx, conn, "")
	if err != nil {
		return "", err
	}
	mount := strings.TrimSpace(spec.Mount)
	if mount == "" {
		mount = strings.TrimSpace(conn.KVDefaultMount)
	}
	if mount == "" {
		mount = "kv"
	}
	kvVer := spec.KVVersion
	if kvVer <= 0 {
		kvVer = conn.KVDefaultVer
	}
	if kvVer <= 0 {
		kvVer = 2
	}

	path := strings.TrimPrefix(strings.TrimSpace(spec.Path), "/")
	readPath := ""
	if kvVer == 2 {
		readPath = fmt.Sprintf("/v1/%s/data/%s", mount, path)
	} else {
		readPath = fmt.Sprintf("/v1/%s/%s", mount, path)
	}

	body, status, err := s.vaultRequest(ctx, conn, token, http.MethodGet, readPath, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("vault read failed: status=%d body=%s", status, strings.TrimSpace(body))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return "", fmt.Errorf("decode vault read response: %w", err)
	}
	key := strings.TrimSpace(spec.Key)
	if key == "" {
		return "", fmt.Errorf("secret key is required")
	}

	var data map[string]any
	if kvVer == 2 {
		dataOuter, _ := payload["data"].(map[string]any)
		if dataOuter != nil {
			data, _ = dataOuter["data"].(map[string]any)
		}
	} else {
		data, _ = payload["data"].(map[string]any)
	}
	if data == nil {
		return "", fmt.Errorf("vault read response has no data")
	}
	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found at %s", key, spec.Path)
	}
	return fmt.Sprintf("%v", val), nil
}

func (s *stateStore) getVaultToken(ctx context.Context, conn protocol.VaultConnection, secretIDOverride string) (string, error) {
	if s.vaultTokens == nil {
		s.vaultTokens = servervault.NewTokenCache()
	}
	if strings.TrimSpace(secretIDOverride) == "" {
		if tok := s.vaultTokens.Get(conn.ID); tok != "" {
			return tok, nil
		}
	}

	secretID := strings.TrimSpace(secretIDOverride)
	if secretID == "" {
		var err error
		secretID, err = servervault.ReadSecretID(conn)
		if err != nil {
			return "", err
		}
	}
	loginPath := fmt.Sprintf("/v1/auth/%s/login", strings.Trim(strings.TrimSpace(conn.AppRoleMount), "/"))
	reqBody := map[string]string{"role_id": conn.RoleID, "secret_id": secretID}
	body, status, err := s.vaultRequest(ctx, conn, "", http.MethodPost, loginPath, reqBody)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("vault login failed: status=%d body=%s", status, strings.TrimSpace(body))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return "", fmt.Errorf("decode vault login response: %w", err)
	}
	auth, _ := payload["auth"].(map[string]any)
	if auth == nil {
		return "", fmt.Errorf("vault login response has no auth block")
	}
	clientToken, _ := auth["client_token"].(string)
	if strings.TrimSpace(clientToken) == "" {
		return "", fmt.Errorf("vault login response missing client_token")
	}
	leaseDur := 3600.0
	if d, ok := auth["lease_duration"].(float64); ok && d > 0 {
		leaseDur = d
	}
	s.vaultTokens.Set(conn.ID, clientToken, time.Now().Add(time.Duration(leaseDur*0.8)*time.Second))
	return clientToken, nil
}

func (s *stateStore) vaultRequest(ctx context.Context, conn protocol.VaultConnection, token, method, path string, payload any) (string, int, error) {
	url := strings.TrimRight(strings.TrimSpace(conn.URL), "/") + path
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", 0, fmt.Errorf("marshal vault payload: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", 0, fmt.Errorf("create vault request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	if ns := strings.TrimSpace(conn.Namespace); ns != "" {
		req.Header.Set("X-Vault-Namespace", ns)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("send vault request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	return string(respBody), resp.StatusCode, nil
}
