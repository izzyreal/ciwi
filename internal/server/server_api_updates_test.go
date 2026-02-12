package server

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/izzyreal/ciwi/internal/version"
)

func TestServerUpdateCheckEndpoint(t *testing.T) {
	asset := expectedAssetName(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		t.Skip("runtime has no configured release asset naming")
	}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/izzyreal/ciwi/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://github.com/izzyreal/ciwi/releases/tag/v0.2.0","assets":[{"name":"` + asset + `","url":"https://example.invalid/asset"}]}`))
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	oldVersion := version.Version
	version.Version = "v0.1.0"
	t.Cleanup(func() { version.Version = oldVersion })
	t.Setenv("CIWI_UPDATE_REQUIRE_CHECKSUM", "false")

	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/check", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update check status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		CurrentVersion  string `json:"current_version"`
		LatestVersion   string `json:"latest_version"`
		UpdateAvailable bool   `json:"update_available"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.CurrentVersion != "v0.1.0" {
		t.Fatalf("unexpected current_version: %q", payload.CurrentVersion)
	}
	if payload.LatestVersion != "v0.2.0" {
		t.Fatalf("unexpected latest_version: %q", payload.LatestVersion)
	}
	if !payload.UpdateAvailable {
		t.Fatalf("expected update_available=true")
	}
}

func TestServerInfoEndpoint(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v0.9.1"
	t.Cleanup(func() { version.Version = oldVersion })
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/server-info", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("server info status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		Name       string `json:"name"`
		APIVersion int    `json:"api_version"`
		Version    string `json:"version"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.Name != "ciwi" {
		t.Fatalf("unexpected name: %q", payload.Name)
	}
	if payload.APIVersion != 1 {
		t.Fatalf("unexpected api_version: %d", payload.APIVersion)
	}
	if payload.Version != "v0.9.1" {
		t.Fatalf("unexpected version: %q", payload.Version)
	}
}

func TestServerUpdateStatusEndpoint(t *testing.T) {
	asset := expectedAssetName(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		t.Skip("runtime has no configured release asset naming")
	}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/izzyreal/ciwi/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://github.com/izzyreal/ciwi/releases/tag/v0.2.0","assets":[{"name":"` + asset + `","url":"https://example.invalid/asset"},{"name":"ciwi-checksums.txt","url":"https://example.invalid/checksums"}]}`))
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	oldVersion := version.Version
	version.Version = "v0.1.0"
	t.Cleanup(func() { version.Version = oldVersion })
	t.Setenv("CIWI_UPDATE_REQUIRE_CHECKSUM", "false")

	ts := newTestHTTPServer(t)
	defer ts.Close()

	checkResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/check", map[string]any{})
	if checkResp.StatusCode != http.StatusOK {
		t.Fatalf("update check status=%d body=%s", checkResp.StatusCode, readBody(t, checkResp))
	}
	_ = readBody(t, checkResp)

	statusResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/update/status", nil)
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("update status status=%d body=%s", statusResp.StatusCode, readBody(t, statusResp))
	}
	var payload struct {
		Status map[string]string `json:"status"`
	}
	decodeJSONBody(t, statusResp, &payload)
	if payload.Status["update_current_version"] != "v0.1.0" {
		t.Fatalf("unexpected update_current_version: %q", payload.Status["update_current_version"])
	}
	if payload.Status["update_latest_version"] != "v0.2.0" {
		t.Fatalf("unexpected update_latest_version: %q", payload.Status["update_latest_version"])
	}
	if payload.Status["update_available"] != "1" {
		t.Fatalf("unexpected update_available: %q", payload.Status["update_available"])
	}
}

func TestServerUpdateTagsEndpoint(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/izzyreal/ciwi/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"v2.0.0"},{"name":"v1.9.0"}]`))
	}))
	defer gh.Close()

	t.Setenv("CIWI_UPDATE_API_BASE", gh.URL)
	t.Setenv("CIWI_UPDATE_REPO", "izzyreal/ciwi")
	oldVersion := version.Version
	version.Version = "v1.8.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/update/tags", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update tags status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		Tags           []string `json:"tags"`
		CurrentVersion string   `json:"current_version"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.CurrentVersion != "v1.8.0" {
		t.Fatalf("unexpected current_version: %q", payload.CurrentVersion)
	}
	if len(payload.Tags) < 3 {
		t.Fatalf("expected current version to be prepended to tags, got %+v", payload.Tags)
	}
	if payload.Tags[0] != "v1.8.0" {
		t.Fatalf("expected current version first, got %+v", payload.Tags)
	}
}

func TestServerRollbackRequiresTargetVersion(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()

	resp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/update/rollback", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("rollback status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
}
