package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeExecutableScript(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(t.TempDir(), "script.cmd")
		if err := os.WriteFile(path, []byte(body+"\r\n"), 0o755); err != nil {
			t.Fatalf("write windows script: %v", err)
		}
		return path
	}
	path := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestUpdateApplyAndRollbackValidation(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/update/apply", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}

	bad := mustRawJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/apply", "{")
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid JSON, got %d", bad.StatusCode)
	}

	s.update.mu.Lock()
	s.update.inProgress = true
	s.update.mu.Unlock()
	busy := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/apply", map[string]any{})
	if busy.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 while update in progress, got %d", busy.StatusCode)
	}
	s.update.mu.Lock()
	s.update.inProgress = false
	s.update.mu.Unlock()

	rb := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/rollback", map[string]any{})
	if rb.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 missing rollback target, got %d", rb.StatusCode)
	}
}

func TestUpdateApplyMessageAndBoolString(t *testing.T) {
	if got := updateApplyMessage(false, false); got != "update helper started, restarting" {
		t.Fatalf("unexpected apply message: %q", got)
	}
	if got := updateApplyMessage(false, true); got != "staged update and triggered linux updater" {
		t.Fatalf("unexpected staged apply message: %q", got)
	}
	if got := updateApplyMessage(true, false); got != "rollback helper started, restarting" {
		t.Fatalf("unexpected rollback message: %q", got)
	}
	if got := updateApplyMessage(true, true); got != "staged rollback and triggered linux updater" {
		t.Fatalf("unexpected staged rollback message: %q", got)
	}
	if boolString(true) != "1" || boolString(false) != "0" {
		t.Fatalf("unexpected boolString conversion")
	}
}

func TestRestartServerViaSystemd(t *testing.T) {
	t.Setenv("CIWI_SERVER_SERVICE_NAME", "   ")
	if _, _, attempted := restartServerViaSystemd(); attempted {
		t.Fatalf("expected no attempt when service name is blank")
	}

	okScript := writeExecutableScript(t, "exit 0")
	t.Setenv("CIWI_SERVER_SERVICE_NAME", "ciwi.service")
	t.Setenv("CIWI_SYSTEMCTL_PATH", okScript)
	msg, err, attempted := restartServerViaSystemd()
	if !attempted || err != nil {
		t.Fatalf("expected successful systemd restart attempt, attempted=%v err=%v", attempted, err)
	}
	if !strings.Contains(msg, "ciwi.service") {
		t.Fatalf("unexpected systemd restart message: %q", msg)
	}

	failScript := writeExecutableScript(t, "echo boom >&2; exit 1")
	t.Setenv("CIWI_SYSTEMCTL_PATH", failScript)
	_, err, attempted = restartServerViaSystemd()
	if !attempted || err == nil {
		t.Fatalf("expected failing systemd restart attempt, attempted=%v err=%v", attempted, err)
	}
}

func TestRestartServerViaLaunchd(t *testing.T) {
	t.Setenv("CIWI_SERVER_LAUNCHD_LABEL", "   ")
	if _, _, attempted := restartServerViaLaunchd(); attempted {
		t.Fatalf("expected no launchd attempt when label is blank")
	}

	okScript := writeExecutableScript(t, "exit 0")
	t.Setenv("CIWI_SERVER_LAUNCHD_LABEL", "ciwi.server")
	t.Setenv("CIWI_SERVER_LAUNCHD_DOMAIN", "gui")
	t.Setenv("CIWI_LAUNCHCTL_PATH", okScript)
	msg, err, attempted := restartServerViaLaunchd()
	if !attempted || err != nil {
		t.Fatalf("expected successful launchd restart attempt, attempted=%v err=%v", attempted, err)
	}
	if !strings.Contains(msg, "ciwi.server") {
		t.Fatalf("unexpected launchd restart message: %q", msg)
	}

	failScript := writeExecutableScript(t, "echo boom >&2; exit 1")
	t.Setenv("CIWI_LAUNCHCTL_PATH", failScript)
	_, err, attempted = restartServerViaLaunchd()
	if !attempted || err == nil {
		t.Fatalf("expected failing launchd restart attempt, attempted=%v err=%v", attempted, err)
	}
}

func TestRequestServerRestartFallbackAndHandler(t *testing.T) {
	// Force restartServerViaService() to be unavailable on all OS variants.
	t.Setenv("CIWI_SERVER_SERVICE_NAME", "   ")
	t.Setenv("CIWI_SERVER_LAUNCHD_LABEL", "   ")
	t.Setenv("CIWI_SERVER_WINDOWS_SERVICE_NAME", "   ")

	done := make(chan struct{}, 1)
	s := &stateStore{restartServerFn: func() { done <- struct{}{} }}
	msg := s.requestServerRestart()
	if !strings.Contains(msg, "fallback exit requested") {
		t.Fatalf("unexpected fallback restart message: %q", msg)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected fallback restart callback to run")
	}

	ts, state := newTestHTTPServerWithState(t)
	defer ts.Close()
	state.restartServerFn = func() {}
	t.Setenv("CIWI_SERVER_SERVICE_NAME", "   ")
	t.Setenv("CIWI_SERVER_LAUNCHD_LABEL", "   ")
	t.Setenv("CIWI_SERVER_WINDOWS_SERVICE_NAME", "   ")

	method := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/server/restart", nil)
	if method.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", method.StatusCode)
	}

	ok := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/server/restart", map[string]any{})
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", ok.StatusCode, readBody(t, ok))
	}
	var payload struct {
		Restarting bool   `json:"restarting"`
		Message    string `json:"message"`
	}
	decodeJSONBody(t, ok, &payload)
	if !payload.Restarting || strings.TrimSpace(payload.Message) == "" {
		t.Fatalf("unexpected restart payload: %+v", payload)
	}
}
