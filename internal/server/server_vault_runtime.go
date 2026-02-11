package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

var secretPlaceholderRE = regexp.MustCompile(`\{\{\s*secret\.([a-zA-Z0-9_\-]+)\s*\}\}`)

type vaultTokenCache struct {
	mu     sync.Mutex
	byConn map[int64]vaultTokenState
}

type vaultTokenState struct {
	Token     string
	ExpiresAt time.Time
}

func newVaultTokenCache() *vaultTokenCache {
	return &vaultTokenCache{byConn: map[int64]vaultTokenState{}}
}

func (s *stateStore) resolveJobSecrets(ctx context.Context, job *protocol.Job) error {
	if job == nil || len(job.Env) == 0 {
		return nil
	}
	projectName := strings.TrimSpace(job.Metadata["project"])
	if projectName == "" {
		return nil
	}

	project, err := s.db.GetProjectByName(projectName)
	if err != nil {
		return nil
	}
	settings, err := s.db.GetProjectVaultSettings(project.ID)
	if err != nil || len(settings.Secrets) == 0 {
		return nil
	}
	if settings.VaultConnectionID <= 0 && strings.TrimSpace(settings.VaultConnectionName) == "" {
		return nil
	}

	var conn protocol.VaultConnection
	if settings.VaultConnectionID > 0 {
		conn, err = s.db.GetVaultConnectionByID(settings.VaultConnectionID)
		if err != nil {
			return err
		}
	} else {
		conn, err = s.db.GetVaultConnectionByName(settings.VaultConnectionName)
		if err != nil {
			return err
		}
	}
	secretMap := map[string]protocol.ProjectSecretSpec{}
	for _, sp := range settings.Secrets {
		secretMap[sp.Name] = sp
	}

	resolved := cloneMap(job.Env)
	sensitive := []string{}
	needsSecrets := false
	for key, value := range resolved {
		matches := secretPlaceholderRE.FindAllStringSubmatch(value, -1)
		if len(matches) == 0 {
			continue
		}
		needsSecrets = true
		out := value
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			secretName := m[1]
			spec, ok := secretMap[secretName]
			if !ok {
				return fmt.Errorf("secret %q is not configured for project", secretName)
			}
			secretValue, getErr := s.readVaultSecret(ctx, conn, spec)
			if getErr != nil {
				return fmt.Errorf("resolve secret %q: %w", secretName, getErr)
			}
			out = strings.ReplaceAll(out, m[0], secretValue)
			sensitive = append(sensitive, secretValue)
		}
		resolved[key] = out
	}

	if needsSecrets {
		job.Env = resolved
		if job.Metadata == nil {
			job.Metadata = map[string]string{}
		}
		job.Metadata["has_secrets"] = "1"
		job.SensitiveValues = dedupeStrings(sensitive)
	}
	return nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
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
		s.vaultTokens = newVaultTokenCache()
	}
	if strings.TrimSpace(secretIDOverride) == "" {
		if tok := s.vaultTokens.get(conn.ID); tok != "" {
			return tok, nil
		}
	}

	secretID := strings.TrimSpace(secretIDOverride)
	if secretID == "" {
		var err error
		secretID, err = readSecretID(conn)
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
	s.vaultTokens.set(conn.ID, clientToken, time.Now().Add(time.Duration(leaseDur*0.8)*time.Second))
	return clientToken, nil
}

func readSecretID(conn protocol.VaultConnection) (string, error) {
	if env := strings.TrimSpace(conn.SecretIDEnv); env != "" {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("vault secret id not available (configure secret_id_env and set it in process environment)")
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

func (c *vaultTokenCache) get(connID int64) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.byConn[connID]
	if !ok || strings.TrimSpace(st.Token) == "" || time.Now().After(st.ExpiresAt) {
		return ""
	}
	return st.Token
}

func (c *vaultTokenCache) set(connID int64, token string, exp time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byConn[connID] = vaultTokenState{Token: token, ExpiresAt: exp}
}
