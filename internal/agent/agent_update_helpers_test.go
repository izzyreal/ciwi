package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type temporaryNetErr struct{}

func (temporaryNetErr) Error() string   { return "temporary network" }
func (temporaryNetErr) Timeout() bool   { return false }
func (temporaryNetErr) Temporary() bool { return true }

func TestAgentUpdateHelpers(t *testing.T) {
	t.Setenv("CIWI_GITHUB_TOKEN", "")
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	applyGitHubAuthHeader(nil)
	applyGitHubAuthHeader(req)
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("expected no auth header without token")
	}
	t.Setenv("CIWI_GITHUB_TOKEN", " tok ")
	applyGitHubAuthHeader(req)
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("unexpected auth header: %q", got)
	}

	client := updateHTTPClient(3 * time.Second)
	if client.Timeout != 3*time.Second {
		t.Fatalf("unexpected updateHTTPClient timeout: %s", client.Timeout)
	}
	if tr, ok := client.Transport.(*http.Transport); !ok || !tr.DisableKeepAlives {
		t.Fatalf("expected transport with keep-alives disabled")
	}

	if !shouldRetryUpdateHTTPStatus(http.StatusTooManyRequests) || !shouldRetryUpdateHTTPStatus(http.StatusInternalServerError) || shouldRetryUpdateHTTPStatus(http.StatusBadRequest) {
		t.Fatalf("unexpected shouldRetryUpdateHTTPStatus behavior")
	}
	if shouldRetryUpdateError(nil) || shouldRetryUpdateError(context.Canceled) || shouldRetryUpdateError(context.DeadlineExceeded) {
		t.Fatalf("unexpected shouldRetryUpdateError baseline behavior")
	}
	if !shouldRetryUpdateError(temporaryNetErr{}) {
		t.Fatalf("expected temporary net error retry")
	}
	if !shouldRetryUpdateError(errors.New("connection reset by peer")) {
		t.Fatalf("expected retry marker recognition")
	}
	if shouldRetryUpdateError(errors.New("permanent failure")) {
		t.Fatalf("did not expect retry for generic permanent error")
	}

	if updateRetryDelay(1) != time.Second || updateRetryDelay(2) != 2*time.Second || updateRetryDelay(3) != 4*time.Second {
		t.Fatalf("unexpected retry delays")
	}

	if !sleepWithContext(context.Background(), 0) {
		t.Fatalf("expected immediate sleepWithContext success")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepWithContext(cancelCtx, 200*time.Millisecond) {
		t.Fatalf("expected sleepWithContext to abort on cancelled context")
	}

	if ext := exeExt(); strings.TrimSpace(ext) != ext {
		t.Fatalf("expected trimmed exeExt, got %q", ext)
	}
}

func TestMoveOrCopyFileAndHashHelpers(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.bin")
	dst := filepath.Join(root, "dst.bin")
	content := []byte("payload")
	if err := os.WriteFile(src, content, 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := moveOrCopyFile(src, dst, 0o700); err != nil {
		t.Fatalf("moveOrCopyFile: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected src to be moved away, stat err=%v", err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(raw) != "payload" {
		t.Fatalf("unexpected dst content: %q", string(raw))
	}

	if err := moveOrCopyFile(dst, dst, 0o700); err != nil {
		t.Fatalf("moveOrCopyFile same path should no-op: %v", err)
	}

	hash, err := fileSHA256(dst)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if hash != want {
		t.Fatalf("unexpected file hash: got=%q want=%q", hash, want)
	}
	checksums := hash + "  ciwi-agent-asset\n"
	if err := verifyFileSHA256(dst, "ciwi-agent-asset", checksums); err != nil {
		t.Fatalf("verifyFileSHA256 expected success: %v", err)
	}
	if err := verifyFileSHA256(dst, "other-asset", checksums); err == nil {
		t.Fatalf("verifyFileSHA256 expected failure for missing asset")
	}
}

func TestProcessRunning(t *testing.T) {
	running, err := processRunning(os.Getpid())
	if err != nil {
		t.Fatalf("processRunning current pid: %v", err)
	}
	if !running {
		t.Fatalf("expected current process to be running")
	}

	if runtime.GOOS != "windows" {
		// Large PID likely does not exist; helper should return false without crashing.
		running, err = processRunning(999999)
		if err != nil {
			t.Fatalf("processRunning missing pid: %v", err)
		}
		if running {
			t.Fatalf("did not expect nonexistent pid to be running")
		}
	}

	var netErr net.Error = temporaryNetErr{}
	if !netErr.Temporary() {
		t.Fatalf("temporaryNetErr should satisfy net.Error")
	}
}
