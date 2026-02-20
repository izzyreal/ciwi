package vault

import (
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestTokenCacheSetGetAndExpiry(t *testing.T) {
	c := NewTokenCache()
	if got := c.Get(42); got != "" {
		t.Fatalf("expected empty token for unknown conn, got %q", got)
	}
	c.Set(42, "token-abc", time.Now().Add(2*time.Second))
	if got := c.Get(42); got != "token-abc" {
		t.Fatalf("expected token-abc, got %q", got)
	}
	c.Set(42, "expired", time.Now().Add(-1*time.Second))
	if got := c.Get(42); got != "" {
		t.Fatalf("expected expired token to be empty, got %q", got)
	}
}

func TestReadSecretIDFromEnv(t *testing.T) {
	const envName = "CIWI_TEST_VAULT_SECRET_ID"
	t.Setenv(envName, "  secret-id  ")
	got, err := ReadSecretID(protocol.VaultConnection{SecretIDEnv: envName})
	if err != nil {
		t.Fatalf("ReadSecretID error: %v", err)
	}
	if got != "secret-id" {
		t.Fatalf("unexpected secret id: %q", got)
	}
}

func TestReadSecretIDMissing(t *testing.T) {
	_, err := ReadSecretID(protocol.VaultConnection{SecretIDEnv: "CIWI_TEST_VAULT_SECRET_ID_MISSING"})
	if err == nil {
		t.Fatalf("expected error when secret env missing")
	}
}

func TestDedupeStrings(t *testing.T) {
	got := DedupeStrings([]string{"a", "b", "a", "", "  ", "b", "c"})
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected dedupe output: %#v", got)
	}
}
