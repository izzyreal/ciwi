package vault

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

type TokenCache struct {
	mu     sync.Mutex
	byConn map[int64]tokenState
}

type tokenState struct {
	Token     string
	ExpiresAt time.Time
}

func NewTokenCache() *TokenCache {
	return &TokenCache{byConn: map[int64]tokenState{}}
}

func (c *TokenCache) Get(connID int64) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.byConn[connID]
	if !ok || strings.TrimSpace(st.Token) == "" || time.Now().After(st.ExpiresAt) {
		return ""
	}
	return st.Token
}

func (c *TokenCache) Set(connID int64, token string, exp time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byConn[connID] = tokenState{Token: token, ExpiresAt: exp}
}

func ReadSecretID(conn protocol.VaultConnection) (string, error) {
	if env := strings.TrimSpace(conn.SecretIDEnv); env != "" {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("vault secret id not available (configure secret_id_env and set it in process environment)")
}

func DedupeStrings(in []string) []string {
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
