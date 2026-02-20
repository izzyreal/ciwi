package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIsValidUpdateStatus(t *testing.T) {
	if !isValidUpdateStatus("running") {
		t.Fatalf("expected running to be valid")
	}
	if isValidUpdateStatus("queued") {
		t.Fatalf("expected queued to be invalid for update")
	}
}

func TestResolveConfigPath(t *testing.T) {
	absPath := filepath.Join(string(os.PathSeparator), "abs", "path.yaml")
	if runtime.GOOS == "windows" {
		absPath = `C:\abs\path.yaml`
	}
	if _, err := resolveConfigPath(absPath); err == nil {
		t.Fatalf("expected absolute path rejection")
	}
	if _, err := resolveConfigPath(".."); err == nil {
		t.Fatalf("expected parent path rejection")
	}
	if _, err := resolveConfigPath(""); err == nil {
		t.Fatalf("expected empty path rejection")
	}

	got, err := resolveConfigPath("configs/ciwi.yaml")
	if err != nil {
		t.Fatalf("resolveConfigPath valid path failed: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(got), "configs/ciwi.yaml") {
		t.Fatalf("unexpected resolved path: %q", got)
	}
}

func TestRenderTemplateCloneMapMergeCapabilities(t *testing.T) {
	rendered := renderTemplate("hello {{name}}", map[string]string{"name": "ciwi"})
	if rendered != "hello ciwi" {
		t.Fatalf("unexpected renderTemplate result: %q", rendered)
	}

	src := map[string]string{"a": "b"}
	cloned := cloneMap(src)
	cloned["a"] = "c"
	if src["a"] != "b" {
		t.Fatalf("cloneMap should not mutate source map")
	}
	if cloneMap(nil) != nil {
		t.Fatalf("expected nil clone for nil input")
	}

	agent := agentState{OS: "linux", Arch: "amd64", Capabilities: map[string]string{"tool.git": "2"}}
	merged := mergeCapabilities(agent, map[string]string{"tool.git": "3", "custom": "x"})
	if merged["os"] != "linux" || merged["arch"] != "amd64" || merged["tool.git"] != "3" || merged["custom"] != "x" {
		t.Fatalf("unexpected mergeCapabilities result: %v", merged)
	}
}

func TestHealthzHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthzHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected content type %q", ct)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/healthz", nil)
	healthzHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestServerInfoHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server-info", nil)
	serverInfoHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body serverInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode server info response: %v", err)
	}
	if body.Name != "ciwi" || body.APIVersion != 1 {
		t.Fatalf("unexpected server info: %+v", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/server-info", nil)
	serverInfoHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET method, got %d", rec.Code)
	}
}

func TestSummarizeAgentStatusFields(t *testing.T) {
	if got := summarizeUpdateFailure(" \n\t "); got != "" {
		t.Fatalf("expected empty summarized update failure, got %q", got)
	}
	if got := summarizeRestartStatus(" \n\t "); got != "" {
		t.Fatalf("expected empty summarized restart status, got %q", got)
	}

	normalized := summarizeUpdateFailure("  update    failed \n due\tto network ")
	if normalized != "update failed due to network" {
		t.Fatalf("unexpected normalized update failure: %q", normalized)
	}

	long := strings.Repeat("x", maxReportedUpdateFailureLength+50)
	got := summarizeRestartStatus(long)
	if len(got) > maxReportedUpdateFailureLength {
		t.Fatalf("expected truncation to <= %d chars, got len=%d", maxReportedUpdateFailureLength, len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated restart status to end with ellipsis, got %q", got)
	}
}

func TestOptionalTime(t *testing.T) {
	if got := optionalTime(time.Time{}); got != nil {
		t.Fatalf("expected nil pointer for zero time, got %v", got)
	}
	in := time.Now().UTC().Round(0)
	got := optionalTime(in)
	if got == nil || !got.Equal(in) {
		t.Fatalf("expected non-nil pointer with same value, got %v", got)
	}
}

func TestListProjectsHandlerMethodGuard(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/projects", map[string]any{})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET projects list, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
}

func TestServerEnvOrDefault(t *testing.T) {
	_ = os.Unsetenv("CIWI_TEST_ENV")
	if got := envOrDefault("CIWI_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	t.Setenv("CIWI_TEST_ENV", "value")
	if got := envOrDefault("CIWI_TEST_ENV", "fallback"); got != "value" {
		t.Fatalf("expected env value, got %q", got)
	}
}

func TestRunCmd(t *testing.T) {
	ctx := context.Background()
	if runtime.GOOS == "windows" {
		out, err := runCmd(ctx, "", "cmd", "/c", "echo", "hello")
		if err != nil {
			t.Fatalf("runCmd echo failed: %v", err)
		}
		if !strings.Contains(strings.ToLower(out), "hello") {
			t.Fatalf("unexpected output: %q", out)
		}
	} else {
		out, err := runCmd(ctx, "", "sh", "-c", "echo hello")
		if err != nil {
			t.Fatalf("runCmd echo failed: %v", err)
		}
		if !strings.Contains(out, "hello") {
			t.Fatalf("unexpected output: %q", out)
		}
	}

	if _, err := runCmd(ctx, "", "definitely-not-a-real-command-ciwi", "arg"); err == nil {
		t.Fatalf("expected runCmd to fail on unknown command")
	}
}
