package agent

import (
	"os"
	"testing"
)

func TestExecEnvWithGoVerbose(t *testing.T) {
	base := []string{"A=B"}
	if got := withGoVerbose(base, false); len(got) != 1 {
		t.Fatalf("expected env unchanged when disabled: %v", got)
	}
	got := withGoVerbose(base, true)
	if len(got) != 2 || got[1] != "GOFLAGS=-v" {
		t.Fatalf("expected GOFLAGS appended, got %v", got)
	}
	got = withGoVerbose([]string{"GOFLAGS=-mod=readonly"}, true)
	if len(got) != 1 {
		t.Fatalf("existing GOFLAGS should not be duplicated: %v", got)
	}
}

func TestMergeEnvAndRedactSensitive(t *testing.T) {
	base := []string{"A=1", "B=2"}
	extra := map[string]string{"B": "3", "C": "4"}
	got := mergeEnv(base, extra)
	if len(got) != 3 {
		t.Fatalf("unexpected merge result len: %v", got)
	}

	redacted := redactSensitive("token=abc123", []string{"abc123", ""})
	if redacted != "token=***" {
		t.Fatalf("unexpected redaction result: %q", redacted)
	}
}

func TestBoolTrimExitEnvHelpers(t *testing.T) {
	t.Setenv("CIWI_BOOL", "yes")
	if !boolEnv("CIWI_BOOL", false) {
		t.Fatalf("expected boolEnv to parse yes")
	}
	t.Setenv("CIWI_BOOL", "off")
	if boolEnv("CIWI_BOOL", true) {
		t.Fatalf("expected boolEnv to parse off")
	}
	t.Setenv("CIWI_BOOL", "weird")
	if !boolEnv("CIWI_BOOL", true) {
		t.Fatalf("unexpected default handling for weird bool env")
	}

	trimmed := trimOutput("0123456789")
	if trimmed == "" {
		t.Fatalf("trimOutput should not return empty for short inputs")
	}

	if code := exitCodeFromErr(os.ErrNotExist); code != nil {
		t.Fatalf("expected non-exit error to produce nil code")
	}

	t.Setenv("CIWI_AGENT_TEST_ENV", "value")
	if got := envOrDefault("CIWI_AGENT_TEST_ENV", "fallback"); got != "value" {
		t.Fatalf("expected env value, got %q", got)
	}
	if got := defaultAgentID(); got == "" {
		t.Fatalf("defaultAgentID should not be empty")
	}
}
