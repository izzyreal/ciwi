package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadTextAssetWrapper(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		_, _ = w.Write([]byte("checksum content"))
	}))
	defer ts.Close()

	t.Setenv("CIWI_GITHUB_TOKEN", "secret-token")
	got, err := downloadTextAsset(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("downloadTextAsset: %v", err)
	}
	if got != "checksum content" {
		t.Fatalf("unexpected text asset content: %q", got)
	}
}

func TestDownloadUpdateAssetWrapper(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-2" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		_, _ = w.Write([]byte("binary"))
	}))
	defer ts.Close()

	t.Setenv("CIWI_GITHUB_TOKEN", "token-2")
	path, err := downloadUpdateAsset(context.Background(), ts.URL, "ciwi-linux-amd64")
	if err != nil {
		t.Fatalf("downloadUpdateAsset: %v", err)
	}
	defer os.Remove(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(raw) != "binary" {
		t.Fatalf("unexpected binary payload: %q", string(raw))
	}
}

func TestServerUpdateFileHelpers(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.bin")
	if err := os.WriteFile(src, []byte("payload"), 0o755); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "dst.bin")
	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(raw) != "payload" {
		t.Fatalf("unexpected copied contents: %q", string(raw))
	}

	hash, err := fileSHA256(src)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}
	checksums := hash + "  ciwi-linux-amd64\n"
	if err := verifyFileSHA256(src, "ciwi-linux-amd64", checksums); err != nil {
		t.Fatalf("verifyFileSHA256 should match: %v", err)
	}
	if err := verifyFileSHA256(src, "other-asset", checksums); err == nil {
		t.Fatalf("verifyFileSHA256 should fail for unknown asset")
	}

	if strings.TrimSpace(exeExt()) != exeExt() {
		t.Fatalf("exeExt should already be trimmed: %q", exeExt())
	}
}

func TestLooksLikeGoRunBinaryWrapper(t *testing.T) {
	if !looksLikeGoRunBinary("/tmp/go-build123/b001/exe/main") {
		t.Fatalf("expected go-run style path to match")
	}
}
