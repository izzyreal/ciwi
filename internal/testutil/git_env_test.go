package testutil

import (
	"os"
	"testing"
)

func TestSetGitEnvHardening(t *testing.T) {
	_ = os.Unsetenv("GIT_CONFIG_NOSYSTEM")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	restore := SetGitEnvHardening()
	if got := os.Getenv("GIT_CONFIG_NOSYSTEM"); got != "1" {
		t.Fatalf("expected GIT_CONFIG_NOSYSTEM=1, got %q", got)
	}
	if got := os.Getenv("GIT_CONFIG_GLOBAL"); got != "/dev/null" {
		t.Fatalf("expected GIT_CONFIG_GLOBAL=/dev/null, got %q", got)
	}
	if got := os.Getenv("GIT_TERMINAL_PROMPT"); got != "0" {
		t.Fatalf("expected GIT_TERMINAL_PROMPT=0, got %q", got)
	}
	if got := os.Getenv("GIT_ASKPASS"); got != "/usr/bin/false" {
		t.Fatalf("expected GIT_ASKPASS=/usr/bin/false, got %q", got)
	}

	restore()
	if _, ok := os.LookupEnv("GIT_CONFIG_NOSYSTEM"); ok {
		t.Fatalf("expected unset GIT_CONFIG_NOSYSTEM after restore")
	}
	if got := os.Getenv("GIT_TERMINAL_PROMPT"); got != "1" {
		t.Fatalf("expected restored GIT_TERMINAL_PROMPT=1, got %q", got)
	}
}

func TestSetEnvHelper(t *testing.T) {
	t.Setenv("CIWI_TEST_ENV_HELPER", "before")
	restore := setEnv("CIWI_TEST_ENV_HELPER", "after")
	if got := os.Getenv("CIWI_TEST_ENV_HELPER"); got != "after" {
		t.Fatalf("expected env to be set to after, got %q", got)
	}
	restore()
	if got := os.Getenv("CIWI_TEST_ENV_HELPER"); got != "before" {
		t.Fatalf("expected env to be restored to before, got %q", got)
	}
}
