package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/updateutil"
)

type DownloadOptions struct {
	AuthToken  string
	HTTPClient *http.Client
}

func DownloadAsset(ctx context.Context, assetURL, assetName string, opts DownloadOptions) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ciwi-updater")
	applyGitHubAuthHeader(req, opts.AuthToken)

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmp := filepath.Join(os.TempDir(), "ciwi-update-"+strconv.FormatInt(time.Now().UnixNano(), 10)+updateutil.ExeExt())
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	if assetName != "" && strings.HasSuffix(assetName, ".exe") && runtime.GOOS == "windows" && !strings.HasSuffix(tmp, ".exe") {
		newTmp := tmp + ".exe"
		if err := os.Rename(tmp, newTmp); err == nil {
			tmp = newTmp
		}
	}
	return tmp, nil
}

func DownloadTextAsset(ctx context.Context, assetURL string, opts DownloadOptions) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ciwi-updater")
	applyGitHubAuthHeader(req, opts.AuthToken)

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func LooksLikeGoRunBinary(path string) bool {
	return updateutil.LooksLikeGoRunBinary(path)
}

func VerifyFileSHA256(path, assetName, checksumContent string) error {
	return updateutil.VerifyFileSHA256(path, assetName, checksumContent)
}

func ExeExt() string {
	return updateutil.ExeExt()
}

func CopyFile(src, dst string, mode os.FileMode) error {
	return updateutil.CopyFile(src, dst, mode)
}
