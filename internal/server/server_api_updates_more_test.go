package server

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

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
