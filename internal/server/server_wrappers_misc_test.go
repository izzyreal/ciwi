package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestListenPortFromAddr(t *testing.T) {
	if got := listenPortFromAddr(""); got != "8112" {
		t.Fatalf("expected default port 8112, got %q", got)
	}
	if got := listenPortFromAddr(":9000"); got != "9000" {
		t.Fatalf("expected :9000 to parse to 9000, got %q", got)
	}
	if got := listenPortFromAddr("127.0.0.1:7777"); got != "7777" {
		t.Fatalf("expected host:port to parse port 7777, got %q", got)
	}
	if got := listenPortFromAddr("not-a-port:"); got != "" {
		t.Fatalf("expected invalid addr parse to empty, got %q", got)
	}
}

func TestServerJobExecutionHandlersForwarding(t *testing.T) {
	ts, _ := newTestHTTPServerWithState(t)
	defer ts.Close()

	jobsResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/jobs", nil)
	if jobsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected jobs handler 200, got %d body=%s", jobsResp.StatusCode, readBody(t, jobsResp))
	}
	_ = readBody(t, jobsResp)

	clearResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/jobs/clear-queue", nil)
	if clearResp.StatusCode != http.StatusOK {
		t.Fatalf("expected clear queue 200, got %d body=%s", clearResp.StatusCode, readBody(t, clearResp))
	}
	_ = readBody(t, clearResp)

	flushResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/jobs/flush-history", nil)
	if flushResp.StatusCode != http.StatusOK {
		t.Fatalf("expected flush history 200, got %d body=%s", flushResp.StatusCode, readBody(t, flushResp))
	}
	_ = readBody(t, flushResp)
}

func TestBuildRouterSmoke(t *testing.T) {
	_, s := newTestHTTPServerWithState(t)
	handler := buildRouter(s, s.artifactsDir)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	healthResp := mustJSONRequest(t, srv.Client(), http.MethodGet, srv.URL+"/healthz", nil)
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("expected healthz 200, got %d body=%s", healthResp.StatusCode, readBody(t, healthResp))
	}
	_ = readBody(t, healthResp)

	infoResp := mustJSONRequest(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/server-info", nil)
	if infoResp.StatusCode != http.StatusOK {
		t.Fatalf("expected server-info 200, got %d body=%s", infoResp.StatusCode, readBody(t, infoResp))
	}
	_ = readBody(t, infoResp)
}

func TestStartUpdateHelperWrapperError(t *testing.T) {
	err := startUpdateHelper("/path/does/not/exist-helper", "/tmp/target", "/tmp/new", os.Getpid(), []string{"serve"})
	if err == nil {
		t.Fatalf("expected startUpdateHelper wrapper to return an error for missing helper binary")
	}
}

func TestTriggerLinuxSystemUpdaterWrapper(t *testing.T) {
	t.Setenv("CIWI_SYSTEMCTL_PATH", "/usr/bin/true")
	t.Setenv("CIWI_UPDATER_UNIT", "ciwi-updater.service")
	if err := triggerLinuxSystemUpdater(); err != nil {
		t.Fatalf("triggerLinuxSystemUpdater with /usr/bin/true: %v", err)
	}

	t.Setenv("CIWI_SYSTEMCTL_PATH", "/usr/bin/false")
	if err := triggerLinuxSystemUpdater(); err == nil {
		t.Fatalf("expected triggerLinuxSystemUpdater error with /usr/bin/false")
	}
}

func TestIsServerRunningAsServiceCurrentRuntime(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		t.Setenv("INVOCATION_ID", "")
		if isServerRunningAsService() {
			t.Fatalf("expected linux service detection false without INVOCATION_ID")
		}
		t.Setenv("INVOCATION_ID", "abc")
		if !isServerRunningAsService() {
			t.Fatalf("expected linux service detection true with INVOCATION_ID")
		}
	case "darwin":
		t.Setenv("LAUNCH_JOB_LABEL", "")
		if isServerRunningAsService() {
			t.Fatalf("expected darwin service detection false without LAUNCH_JOB_LABEL")
		}
		t.Setenv("LAUNCH_JOB_LABEL", "nl.izmar.ciwi")
		if !isServerRunningAsService() {
			t.Fatalf("expected darwin service detection true with LAUNCH_JOB_LABEL")
		}
	case "windows":
		t.Setenv("CIWI_SERVER_WINDOWS_SERVICE_NAME", "")
		if isServerRunningAsService() {
			t.Fatalf("expected windows service detection false without service name")
		}
		t.Setenv("CIWI_SERVER_WINDOWS_SERVICE_NAME", "ciwi")
		if !isServerRunningAsService() {
			t.Fatalf("expected windows service detection true with service name")
		}
	}
}

func TestStartMDNSAdvertiserDisabled(t *testing.T) {
	t.Setenv("CIWI_MDNS_ENABLE", "false")
	shutdown := startMDNSAdvertiser("127.0.0.1:8112")
	// Should always be callable even when mDNS is disabled.
	shutdown()

	t.Setenv("CIWI_MDNS_ENABLE", "true")
	// Invalid listen addr should no-op and still return callable shutdown.
	shutdown = startMDNSAdvertiser("invalid:")
	shutdown()
}

func TestFileSHA256Wrapper(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hash-me")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	h, err := fileSHA256(p)
	if err != nil {
		t.Fatalf("fileSHA256 wrapper: %v", err)
	}
	if h == "" {
		t.Fatalf("expected non-empty sha256 hash")
	}
}
