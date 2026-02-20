package update

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/updateutil"
)

type mockRoundTripper func(*http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func mockClient(fn mockRoundTripper) *http.Client {
	return &http.Client{Transport: fn}
}

func jsonResponse(status int, v any) *http.Response {
	raw, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(string(raw))),
		Header:     make(http.Header),
	}
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestIsLinuxSystemUpdaterEnabled(t *testing.T) {
	if IsLinuxSystemUpdaterEnabled("darwin", "") {
		t.Fatalf("non-linux must be disabled")
	}
	if IsLinuxSystemUpdaterEnabled("linux", "false") {
		t.Fatalf("explicit false must disable updater")
	}
	if !IsLinuxSystemUpdaterEnabled("linux", "true") {
		t.Fatalf("linux + true must enable updater")
	}
}

func TestSanitizeVersionToken(t *testing.T) {
	if got := SanitizeVersionToken(" v1.2.3-rc_1 "); got != "v1.2.3-rc_1" {
		t.Fatalf("unexpected token: %q", got)
	}
	if got := SanitizeVersionToken("..@@bad$$.."); got != "bad" {
		t.Fatalf("unexpected sanitized token: %q", got)
	}
}

func TestFileSHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file.bin")
	content := []byte("hello-ciwi")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got, err := FileSHA256(p)
	if err != nil {
		t.Fatalf("hash file: %v", err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("unexpected hash: got=%q want=%q", got, want)
	}
}

func TestStageLinuxUpdateBinaryWritesManifestAndStagedBinary(t *testing.T) {
	tmp := t.TempDir()
	newBin := filepath.Join(tmp, "ciwi-new"+ExeExt())
	if err := os.WriteFile(newBin, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write new bin: %v", err)
	}
	stagingDir := filepath.Join(tmp, "stage")
	manifestPath := filepath.Join(tmp, "pending.json")
	if err := StageLinuxUpdateBinary("v1.2.3", "ciwi_asset", newBin, StageLinuxOptions{
		StagingDir:   stagingDir,
		ManifestPath: manifestPath,
	}); err != nil {
		t.Fatalf("stage linux update: %v", err)
	}
	stagedPath := filepath.Join(stagingDir, "ciwi-v1.2.3"+ExeExt())
	if _, err := os.Stat(stagedPath); err != nil {
		t.Fatalf("expected staged binary at %q: %v", stagedPath, err)
	}
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest := string(rawManifest)
	if !strings.Contains(manifest, "v1.2.3") || !strings.Contains(manifest, stagedPath) {
		t.Fatalf("manifest missing expected fields: %s", manifest)
	}
}

func TestStageLinuxUpdateBinaryUsesLatestAndDefaultManifestPath(t *testing.T) {
	tmp := t.TempDir()
	newBin := filepath.Join(tmp, "ciwi-new"+ExeExt())
	if err := os.WriteFile(newBin, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write new bin: %v", err)
	}
	stagingDir := filepath.Join(tmp, "stage")
	if err := StageLinuxUpdateBinary("###", "ciwi_asset", newBin, StageLinuxOptions{
		StagingDir: stagingDir,
	}); err != nil {
		t.Fatalf("stage linux update with defaults: %v", err)
	}
	stagedPath := filepath.Join(stagingDir, "ciwi-latest"+ExeExt())
	if _, err := os.Stat(stagedPath); err != nil {
		t.Fatalf("expected staged binary at %q: %v", stagedPath, err)
	}
	manifestPath := filepath.Join(stagingDir, "pending.json")
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read default manifest path: %v", err)
	}
	manifest := string(rawManifest)
	if !strings.Contains(manifest, "\"target_version\": \"###\"") || !strings.Contains(manifest, stagedPath) {
		t.Fatalf("manifest missing expected fields: %s", manifest)
	}
}

func TestTriggerLinuxSystemUpdaterReportsCommandError(t *testing.T) {
	err := TriggerLinuxSystemUpdater(filepath.Join(t.TempDir(), "no-systemctl"), "ciwi-updater.service")
	if err == nil {
		t.Fatalf("expected error for missing systemctl path")
	}
}

func TestTriggerLinuxSystemUpdaterSuccessAndDefaultUnit(t *testing.T) {
	tmp := t.TempDir()
	argsPath := filepath.Join(tmp, "args.txt")
	scriptPath := filepath.Join(tmp, "fake-systemctl.sh")
	script := "#!/bin/sh\n" +
		"echo \"$@\" > \"" + argsPath + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake systemctl script: %v", err)
	}
	if err := TriggerLinuxSystemUpdater(scriptPath, ""); err != nil {
		t.Fatalf("TriggerLinuxSystemUpdater: %v", err)
	}
	rawArgs, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	got := strings.TrimSpace(string(rawArgs))
	if got != "start --no-block ciwi-updater.service" {
		t.Fatalf("unexpected systemctl args: %q", got)
	}
}

func TestFetchLatestInfoSuccessAndAuthHeader(t *testing.T) {
	assetName := updateutil.ExpectedAssetName(runtime.GOOS, runtime.GOARCH)
	if strings.TrimSpace(assetName) == "" {
		t.Skip("no expected asset naming for this GOOS/GOARCH")
	}

	var authHeader string
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/repos/acme/ciwi/releases/latest" {
			return textResponse(http.StatusNotFound, "not found"), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"tag_name": "v9.9.9",
			"html_url": "https://example/releases/v9.9.9",
			"assets": []map[string]any{
				{"name": assetName, "url": "https://example/download/ciwi"},
				{"name": "ciwi-checksums.txt", "url": "https://example/download/sums"},
			},
		}), nil
	})

	info, err := FetchLatestInfo(context.Background(), FetchInfoOptions{
		APIBase:         "https://api.example",
		Repo:            "acme/ciwi",
		AuthToken:       "secret-token",
		HTTPClient:      client,
		RequireChecksum: true,
	})
	if err != nil {
		t.Fatalf("fetch latest info: %v", err)
	}
	if info.TagName != "v9.9.9" || info.Asset.Name != assetName || info.ChecksumAsset.Name != "ciwi-checksums.txt" {
		t.Fatalf("unexpected latest info: %+v", info)
	}
	if authHeader != "Bearer secret-token" {
		t.Fatalf("missing auth header, got %q", authHeader)
	}
}

func TestFetchLatestInfoRequiresChecksumWhenEnabled(t *testing.T) {
	assetName := updateutil.ExpectedAssetName(runtime.GOOS, runtime.GOARCH)
	if strings.TrimSpace(assetName) == "" {
		t.Skip("no expected asset naming for this GOOS/GOARCH")
	}
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"tag_name": "v1.0.0",
			"assets":   []map[string]any{{"name": assetName, "url": "https://example/download/ciwi"}},
		}), nil
	})
	_, err := FetchLatestInfo(context.Background(), FetchInfoOptions{
		APIBase:           "https://api.example",
		Repo:              "acme/ciwi",
		HTTPClient:        client,
		RequireChecksum:   true,
		ChecksumAssetName: "ciwi-checksums.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "no checksum asset") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestFetchTagsDeduplicatesAndSkipsEmpty(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/repos/acme/ciwi/tags" {
			return textResponse(http.StatusNotFound, "not found"), nil
		}
		return jsonResponse(http.StatusOK, []map[string]any{
			{"name": "v1.0.0"},
			{"name": "v1.0.0"},
			{"name": "v0.9.0"},
			{"name": " "},
		}), nil
	})
	tags, err := FetchTags(context.Background(), FetchTagsOptions{
		APIBase:    "https://api.example",
		Repo:       "acme/ciwi",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("fetch tags: %v", err)
	}
	if len(tags) != 2 || tags[0] != "v1.0.0" || tags[1] != "v0.9.0" {
		t.Fatalf("unexpected tags: %#v", tags)
	}
}

func TestDownloadTextAssetSuccess(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return textResponse(http.StatusOK, "hello\nworld"), nil
	})
	out, err := DownloadTextAsset(context.Background(), "https://downloads.example/text", DownloadOptions{HTTPClient: client})
	if err != nil {
		t.Fatalf("download text asset: %v", err)
	}
	if out != "hello\nworld" {
		t.Fatalf("unexpected text: %q", out)
	}
}

func TestDownloadTextAssetHTTPErrorIncludesBody(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return textResponse(http.StatusBadGateway, "upstream error"), nil
	})
	_, err := DownloadTextAsset(context.Background(), "https://downloads.example/text", DownloadOptions{HTTPClient: client})
	if err == nil || !strings.Contains(err.Error(), "status=502") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestDownloadAssetAndVerifyChecksum(t *testing.T) {
	content := []byte("ciwi-binary")
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(content))),
			Header:     make(http.Header),
		}, nil
	})
	p, err := DownloadAsset(context.Background(), "https://downloads.example/asset", "ciwi", DownloadOptions{HTTPClient: client})
	if err != nil {
		t.Fatalf("download asset: %v", err)
	}
	defer os.Remove(p)
	sum := sha256.Sum256(content)
	checkLine := fmt.Sprintf("%s  ciwi\n", hex.EncodeToString(sum[:]))
	if err := VerifyFileSHA256(p, "ciwi", checkLine); err != nil {
		t.Fatalf("verify checksum: %v", err)
	}
}

func TestDownloadAssetHTTPErrorIncludesBody(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return textResponse(http.StatusBadGateway, "nope"), nil
	})
	_, err := DownloadAsset(context.Background(), "https://downloads.example/asset", "ciwi", DownloadOptions{HTTPClient: client})
	if err == nil || !strings.Contains(err.Error(), "status=502") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestApplyGitHubAuthHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example", nil)
	applyGitHubAuthHeader(req, " tok ")
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("unexpected auth header: %q", got)
	}
}

func TestStartUpdateHelperErrorOnMissingBinary(t *testing.T) {
	err := StartUpdateHelper("/path/does/not/exist-helper", "/tmp/target", "/tmp/new", os.Getpid(), []string{"serve"})
	if err == nil {
		t.Fatalf("expected helper start error")
	}
}

func TestLooksLikeGoRunBinaryDelegates(t *testing.T) {
	name := "/tmp/go-build1234/b001/exe/main"
	if !LooksLikeGoRunBinary(name) {
		t.Fatalf("expected go-run binary path to match")
	}
}

func TestDownloadTextAssetWithAuthHeader(t *testing.T) {
	var gotAuth string
	payload := base64.StdEncoding.EncodeToString([]byte("ok"))
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		return textResponse(http.StatusOK, payload), nil
	})
	_, err := DownloadTextAsset(context.Background(), "https://downloads.example/text", DownloadOptions{
		AuthToken:  "token-123",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("download text asset: %v", err)
	}
	if gotAuth != "Bearer token-123" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
}

func TestFetchLatestInfoTargetTagAndErrorLabel(t *testing.T) {
	assetName := updateutil.ExpectedAssetName(runtime.GOOS, runtime.GOARCH)
	if strings.TrimSpace(assetName) == "" {
		t.Skip("no expected asset naming for this GOOS/GOARCH")
	}
	var gotPath string
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return textResponse(http.StatusBadGateway, "upstream down"), nil
	})
	_, err := FetchLatestInfo(context.Background(), FetchInfoOptions{
		APIBase:         "https://api.example",
		Repo:            "acme/ciwi",
		TargetTag:       "v1.2.3",
		HTTPClient:      client,
		RequireChecksum: false,
	})
	if err == nil || !strings.Contains(err.Error(), "release for tag v1.2.3 query failed") {
		t.Fatalf("expected tagged release error label, got %v", err)
	}
	if gotPath != "/repos/acme/ciwi/releases/tags/v1.2.3" {
		t.Fatalf("unexpected tagged release path: %q", gotPath)
	}
}

func TestFetchLatestInfoMissingCompatibleAsset(t *testing.T) {
	client := mockClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"tag_name": "v1.0.0",
			"assets": []map[string]any{
				{"name": "ciwi-other-platform", "url": "https://example/download/other"},
			},
		}), nil
	})
	_, err := FetchLatestInfo(context.Background(), FetchInfoOptions{
		APIBase:         "https://api.example",
		Repo:            "acme/ciwi",
		HTTPClient:      client,
		RequireChecksum: false,
	})
	if err == nil || !strings.Contains(err.Error(), "no compatible asset") {
		t.Fatalf("expected missing compatible asset error, got %v", err)
	}
}

func TestFetchTagsErrors(t *testing.T) {
	t.Run("status error", func(t *testing.T) {
		client := mockClient(func(r *http.Request) (*http.Response, error) {
			return textResponse(http.StatusServiceUnavailable, "busy"), nil
		})
		_, err := FetchTags(context.Background(), FetchTagsOptions{
			APIBase:    "https://api.example",
			Repo:       "acme/ciwi",
			HTTPClient: client,
		})
		if err == nil || !strings.Contains(err.Error(), "tags query failed: status=503") {
			t.Fatalf("expected tags status error, got %v", err)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		client := mockClient(func(r *http.Request) (*http.Response, error) {
			return textResponse(http.StatusOK, "{"), nil
		})
		_, err := FetchTags(context.Background(), FetchTagsOptions{
			APIBase:    "https://api.example",
			Repo:       "acme/ciwi",
			HTTPClient: client,
		})
		if err == nil || !strings.Contains(err.Error(), "decode tags response") {
			t.Fatalf("expected tags decode error, got %v", err)
		}
	})
}

func TestStageLinuxUpdateBinaryWriteManifestError(t *testing.T) {
	tmp := t.TempDir()
	newBin := filepath.Join(tmp, "ciwi-new"+ExeExt())
	if err := os.WriteFile(newBin, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write new bin: %v", err)
	}
	// Manifest path directory intentionally missing to force write error.
	manifestPath := filepath.Join(tmp, "missing-dir", "pending.json")
	err := StageLinuxUpdateBinary("v1.0.0", "ciwi-linux-amd64", newBin, StageLinuxOptions{
		StagingDir:   filepath.Join(tmp, "stage"),
		ManifestPath: manifestPath,
	})
	if err == nil || !strings.Contains(err.Error(), "write staged manifest") {
		t.Fatalf("expected write staged manifest error, got %v", err)
	}
}

func TestStartUpdateHelperArgsAndCopyFile(t *testing.T) {
	tmp := t.TempDir()
	helper := filepath.Join(tmp, "helper.sh")
	argsLog := filepath.Join(tmp, "args.log")
	script := "#!/bin/sh\necho \"$@\" > \"" + argsLog + "\"\nexit 0\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := StartUpdateHelper(helper, "/tmp/target", "/tmp/new", 1234, []string{"serve", "--flag"}); err != nil {
		t.Fatalf("StartUpdateHelper: %v", err)
	}
	var (
		raw []byte
		err error
	)
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, err = os.ReadFile(argsLog)
		if err == nil && len(raw) > 0 {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	args := string(raw)
	if !strings.Contains(args, "update-helper") || !strings.Contains(args, "--target /tmp/target") || !strings.Contains(args, "--new /tmp/new") || !strings.Contains(args, "--pid 1234") {
		t.Fatalf("unexpected helper args: %q", args)
	}
	if !strings.Contains(args, "--arg serve") || !strings.Contains(args, "--arg --flag") {
		t.Fatalf("expected restart args in helper invocation, got %q", args)
	}

	src := filepath.Join(tmp, "src.bin")
	dst := filepath.Join(tmp, "dst.bin")
	if err := os.WriteFile(src, []byte("copy-me"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := CopyFile(src, dst, 0o700); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "copy-me" {
		t.Fatalf("unexpected copied content: %q", string(got))
	}
}
