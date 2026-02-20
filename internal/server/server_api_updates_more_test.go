package server

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/version"
)

func TestServerUpdateCheckEndpointErrorPersistsStatus(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	t.Setenv("CIWI_UPDATE_REQUIRE_CHECKSUM", "false")
	oldVersion := version.Version
	version.Version = "v0.1.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/check", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update check status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		CurrentVersion string `json:"current_version"`
		Message        string `json:"message"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.CurrentVersion != "v0.1.0" || strings.TrimSpace(payload.Message) == "" {
		t.Fatalf("unexpected error payload: %+v", payload)
	}

	status := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/update/status", nil)
	if status.StatusCode != http.StatusOK {
		t.Fatalf("update status status=%d body=%s", status.StatusCode, readBody(t, status))
	}
	var statusPayload struct {
		Status map[string]string `json:"status"`
	}
	decodeJSONBody(t, status, &statusPayload)
	if statusPayload.Status["update_available"] != "0" {
		t.Fatalf("expected update_available=0 after failed check, got %q", statusPayload.Status["update_available"])
	}
	if strings.TrimSpace(statusPayload.Status["update_message"]) == "" {
		t.Fatalf("expected persisted update_message on failed check")
	}
}

func TestServerUpdateTagsEndpointError(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/update/tags", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for update tags fetch failure, got %d body=%s", resp.StatusCode, readBody(t, resp))
	}
}

func TestServerUpdateStatusMethodNotAllowed(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/status", map[string]any{})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET update status, got %d", resp.StatusCode)
	}
}

func TestServerUpdateApplyGoRunGuard(t *testing.T) {
	asset := expectedAssetName(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		t.Skip("runtime has no configured release asset naming")
	}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/latest") || strings.Contains(r.URL.Path, "/releases/tags/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.invalid/release","assets":[{"name":"` + asset + `","url":"https://example.invalid/asset"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	t.Setenv("CIWI_UPDATE_REQUIRE_CHECKSUM", "false")
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/apply", map[string]any{"target_version": "v9.9.9"})
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected go-run self-update guard to reject apply in tests, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "self-update is unavailable for go run binaries") {
		t.Fatalf("expected go-run guard message in apply response")
	}
}

func TestServerUpdateHandlersMethodAndJSONValidation(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "check method guard", method: http.MethodGet, path: "/api/v1/update/check", wantStatus: http.StatusMethodNotAllowed},
		{name: "apply method guard", method: http.MethodGet, path: "/api/v1/update/apply", wantStatus: http.StatusMethodNotAllowed},
		{name: "rollback method guard", method: http.MethodGet, path: "/api/v1/update/rollback", wantStatus: http.StatusMethodNotAllowed},
		{name: "tags method guard", method: http.MethodPost, path: "/api/v1/update/tags", body: "{}", wantStatus: http.StatusMethodNotAllowed},
		{name: "apply invalid json", method: http.MethodPost, path: "/api/v1/update/apply", body: `{"target_version":`, wantStatus: http.StatusBadRequest},
		{name: "rollback invalid json", method: http.MethodPost, path: "/api/v1/update/rollback", body: `{"target_version":`, wantStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var resp *http.Response
			if strings.TrimSpace(tc.body) == "" {
				resp = mustJSONRequest(t, ts.Client(), tc.method, ts.URL+tc.path, nil)
			} else {
				resp = mustRawJSONRequest(t, ts.Client(), tc.method, ts.URL+tc.path, tc.body)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, tc.wantStatus, readBody(t, resp))
			}
		})
	}
}

func TestServerUpdateApplyAndRollbackConflictWhenInProgress(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	s.update.mu.Lock()
	s.update.inProgress = true
	s.update.lastMessage = "already running"
	s.update.mu.Unlock()

	applyResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/apply", map[string]any{
		"target_version": "v9.9.9",
	})
	if applyResp.StatusCode != http.StatusConflict {
		t.Fatalf("apply status=%d want=409 body=%s", applyResp.StatusCode, readBody(t, applyResp))
	}

	rollbackResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/rollback", map[string]any{
		"target_version": "v9.9.8",
	})
	if rollbackResp.StatusCode != http.StatusConflict {
		t.Fatalf("rollback status=%d want=409 body=%s", rollbackResp.StatusCode, readBody(t, rollbackResp))
	}
}

func TestPersistUpdateStatusSkipsEmptyKeys(t *testing.T) {
	_, s := newTestHTTPServerWithState(t)
	if err := s.persistUpdateStatus(map[string]string{
		"":                    "ignored",
		"   ":                 "ignored-too",
		"update_message":      "ok",
		"update_last_checked": time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("persistUpdateStatus: %v", err)
	}
	state, err := s.updateStateStore().ListAppState()
	if err != nil {
		t.Fatalf("ListAppState: %v", err)
	}
	if got := strings.TrimSpace(state["update_message"]); got != "ok" {
		t.Fatalf("expected update_message to persist, got %q", got)
	}
	if _, exists := state[""]; exists {
		t.Fatalf("unexpected blank app-state key present")
	}
}
