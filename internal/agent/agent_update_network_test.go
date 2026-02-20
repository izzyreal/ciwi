package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFetchReleaseAssetsForTag(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/releases/tags/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"asset.bin","url":"https://example.invalid/asset"},{"name":"ciwi-checksums.txt","url":"https://example.invalid/checksums"}]}`))
	}))
	defer good.Close()

	asset, checksum, err := fetchReleaseAssetsForTag(context.Background(), good.URL, "izzyreal/ciwi", "v1.2.3", "asset.bin", "ciwi-checksums.txt")
	if err != nil {
		t.Fatalf("fetchReleaseAssetsForTag success: %v", err)
	}
	if asset.Name != "asset.bin" || checksum.Name != "ciwi-checksums.txt" {
		t.Fatalf("unexpected assets: asset=%+v checksum=%+v", asset, checksum)
	}

	missingAsset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"other.bin","url":"https://example.invalid/other"}]}`))
	}))
	defer missingAsset.Close()
	if _, _, err := fetchReleaseAssetsForTag(context.Background(), missingAsset.URL, "izzyreal/ciwi", "v1.2.3", "asset.bin", "ciwi-checksums.txt"); err == nil {
		t.Fatalf("expected missing asset error")
	}

	missingChecksum := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"asset.bin","url":"https://example.invalid/asset"}]}`))
	}))
	defer missingChecksum.Close()
	t.Setenv("CIWI_UPDATE_REQUIRE_CHECKSUM", "true")
	if _, _, err := fetchReleaseAssetsForTag(context.Background(), missingChecksum.URL, "izzyreal/ciwi", "v1.2.3", "asset.bin", "ciwi-checksums.txt"); err == nil {
		t.Fatalf("expected missing checksum asset error when checksum required")
	}
}

func TestAgentDownloadUpdateAssets(t *testing.T) {
	binSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/octet-stream" {
			t.Fatalf("unexpected accept header: %q", got)
		}
		_, _ = w.Write([]byte("binary-payload"))
	}))
	defer binSrv.Close()

	path, err := downloadUpdateAsset(context.Background(), binSrv.URL, "ciwi-test")
	if err != nil {
		t.Fatalf("downloadUpdateAsset: %v", err)
	}
	defer os.Remove(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded asset: %v", err)
	}
	if string(raw) != "binary-payload" {
		t.Fatalf("unexpected downloaded payload: %q", string(raw))
	}

	textSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("checksums text"))
	}))
	defer textSrv.Close()
	text, err := downloadTextAsset(context.Background(), textSrv.URL)
	if err != nil {
		t.Fatalf("downloadTextAsset: %v", err)
	}
	if text != "checksums text" {
		t.Fatalf("unexpected downloaded text payload: %q", text)
	}

	errorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer errorSrv.Close()
	if _, err := downloadUpdateAsset(context.Background(), errorSrv.URL, "ciwi-test"); err == nil {
		t.Fatalf("expected downloadUpdateAsset error on non-2xx")
	}
	if _, err := downloadTextAsset(context.Background(), errorSrv.URL); err == nil {
		t.Fatalf("expected downloadTextAsset error on non-2xx")
	}
}

func TestStartUpdateHelperBuildsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based helper invocation test is posix-only")
	}
	tmp := t.TempDir()
	argsLog := filepath.Join(tmp, "args.log")
	help := filepath.Join(tmp, "helper.sh")
	script := "#!/bin/sh\necho \"$@\" > \"" + argsLog + "\"\nexit 0\n"
	if err := os.WriteFile(help, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper script: %v", err)
	}
	if err := startUpdateHelper(help, "/tmp/target", "/tmp/new", 1234, []string{"agent", "--flag"}, ""); err != nil {
		t.Fatalf("startUpdateHelper: %v", err)
	}
	// Give process a moment to write args.
	found := false
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(argsLog); err == nil {
			found = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !found {
		t.Fatalf("helper did not write args log: %s", argsLog)
	}
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read helper args log: %v", err)
	}
	args := string(raw)
	if !strings.Contains(args, "update-helper") || !strings.Contains(args, "--target /tmp/target") || !strings.Contains(args, "--new /tmp/new") || !strings.Contains(args, "--pid 1234") {
		t.Fatalf("unexpected helper args: %q", args)
	}
	if !strings.Contains(args, "--arg agent") || !strings.Contains(args, "--arg --flag") {
		t.Fatalf("expected restart args to be forwarded, got %q", args)
	}
}
