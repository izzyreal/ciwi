package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/izzyreal/ciwi/internal/darwinupdater"
	"github.com/izzyreal/ciwi/internal/updateutil"
)

type githubReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

const (
	updateNetworkMaxAttempts = 3
)

func selfUpdateAndRestart(ctx context.Context, targetVersion, repository, apiBase string, restartArgs []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exePath, _ = filepath.Abs(exePath)
	if looksLikeGoRunBinary(exePath) {
		return fmt.Errorf("self-update unavailable for go run binaries")
	}

	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" || !isVersionDifferent(targetVersion, currentVersion()) {
		return nil
	}
	if reason := selfUpdateServiceModeReason(); reason != "" {
		return fmt.Errorf("%s", reason)
	}

	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	repository = strings.TrimSpace(repository)
	if repository == "" {
		repository = "izzyreal/ciwi"
	}

	assetName := expectedAssetName(runtime.GOOS, runtime.GOARCH)
	if assetName == "" {
		return fmt.Errorf("no known update asset for os=%s arch=%s", runtime.GOOS, runtime.GOARCH)
	}
	checksumName := strings.TrimSpace(envOrDefault("CIWI_UPDATE_CHECKSUM_ASSET", "ciwi-checksums.txt"))
	if checksumName == "" {
		checksumName = "ciwi-checksums.txt"
	}

	asset, checksumAsset, err := fetchReleaseAssetsForTag(ctx, apiBase, repository, targetVersion, assetName, checksumName)
	if err != nil {
		return err
	}

	newBinPath, err := downloadUpdateAsset(ctx, asset.URL, asset.Name)
	if err != nil {
		return fmt.Errorf("download update asset: %w", err)
	}
	if checksumAsset.URL != "" {
		checksumText, err := downloadTextAsset(ctx, checksumAsset.URL)
		if err != nil {
			return fmt.Errorf("download checksum asset: %w", err)
		}
		if err := verifyFileSHA256(newBinPath, asset.Name, checksumText); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	if runtime.GOOS == "darwin" && hasDarwinUpdaterConfig() {
		if err := stageAndTriggerDarwinUpdater(targetVersion, asset.Name, exePath, newBinPath); err != nil {
			return err
		}
		return nil
	}

	helperPath := filepath.Join(filepath.Dir(newBinPath), "ciwi-update-helper-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
	if err := copyFile(exePath, helperPath, 0o755); err != nil {
		return fmt.Errorf("prepare update helper: %w", err)
	}
	serviceName := ""
	if runtime.GOOS == "windows" {
		if active, name := windowsServiceInfo(); active {
			serviceName = strings.TrimSpace(name)
		}
	}

	if err := startUpdateHelper(helperPath, exePath, newBinPath, os.Getpid(), restartArgs, serviceName); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}

	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func hasDarwinUpdaterConfig() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_LABEL", "")) != "" &&
		strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_PLIST", "")) != "" &&
		strings.TrimSpace(envOrDefault("CIWI_AGENT_UPDATER_LABEL", "")) != ""
}

func stageAndTriggerDarwinUpdater(targetVersion, assetName, targetBinary, stagedBinary string) error {
	agentLabel := strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_LABEL", ""))
	agentPlist := strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_PLIST", ""))
	updaterLabel := strings.TrimSpace(envOrDefault("CIWI_AGENT_UPDATER_LABEL", ""))
	updaterPlist := strings.TrimSpace(envOrDefault("CIWI_AGENT_UPDATER_PLIST", ""))
	if agentLabel == "" || agentPlist == "" || updaterLabel == "" {
		return fmt.Errorf("missing launchd updater configuration")
	}

	workDir := strings.TrimSpace(envOrDefault("CIWI_AGENT_WORKDIR", ".ciwi-agent/work"))
	manifestPath := strings.TrimSpace(envOrDefault("CIWI_AGENT_UPDATE_MANIFEST", filepath.Join(workDir, "updates", "pending.json")))
	if manifestPath == "" {
		return fmt.Errorf("unable to resolve CIWI_AGENT_UPDATE_MANIFEST path")
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return fmt.Errorf("create update manifest directory: %w", err)
	}
	stagePath := filepath.Join(filepath.Dir(manifestPath), filepath.Base(stagedBinary))
	if err := moveOrCopyFile(stagedBinary, stagePath, 0o755); err != nil {
		return fmt.Errorf("stage update binary: %w", err)
	}
	hash, err := fileSHA256(stagePath)
	if err != nil {
		return fmt.Errorf("hash staged update binary: %w", err)
	}
	manifest, err := darwinupdater.BuildManifest(targetVersion, assetName, targetBinary, stagePath, hash, agentLabel, agentPlist, updaterLabel, updaterPlist, defaultAgentID(), os.Getpid())
	if err != nil {
		return fmt.Errorf("build update manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		return fmt.Errorf("write update manifest: %w", err)
	}
	if strings.TrimSpace(envOrDefault("CIWI_DARWIN_ADHOC_SIGN", "true")) != "false" {
		if err := adHocSignBinary(targetBinary); err != nil {
			_ = os.Remove(manifestPath)
			return fmt.Errorf("ad-hoc sign current binary: %w", err)
		}
	}
	if err := runLaunchctl("kickstart", "-k", "gui/"+strconv.Itoa(os.Getuid())+"/"+updaterLabel); err != nil {
		_ = os.Remove(manifestPath)
		return fmt.Errorf("trigger updater launchagent: %w", err)
	}
	return nil
}

func fetchReleaseAssetsForTag(ctx context.Context, apiBase, repository, targetVersion, assetName, checksumName string) (githubReleaseAsset, githubReleaseAsset, error) {
	url := apiBase + "/repos/" + repository + "/releases/tags/" + strings.TrimSpace(targetVersion)
	client := updateHTTPClient(20 * time.Second)
	var lastErr error
	for attempt := 1; attempt <= updateNetworkMaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "ciwi-agent-updater")
		applyGitHubAuthHeader(req)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request release for tag: %w", err)
			if attempt < updateNetworkMaxAttempts && shouldRetryUpdateError(err) {
				wait := updateRetryDelay(attempt)
				slog.Warn("release tag request failed; retrying", "target_version", targetVersion, "attempt", attempt, "next_wait", wait, "error", err)
				if !sleepWithContext(ctx, wait) {
					return githubReleaseAsset{}, githubReleaseAsset{}, context.Cause(ctx)
				}
				continue
			}
			return githubReleaseAsset{}, githubReleaseAsset{}, lastErr
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("release tag query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
			if attempt < updateNetworkMaxAttempts && shouldRetryUpdateHTTPStatus(resp.StatusCode) {
				wait := updateRetryDelay(attempt)
				slog.Warn("release tag query returned retryable status; retrying", "target_version", targetVersion, "status", resp.StatusCode, "attempt", attempt, "next_wait", wait)
				if !sleepWithContext(ctx, wait) {
					return githubReleaseAsset{}, githubReleaseAsset{}, context.Cause(ctx)
				}
				continue
			}
			return githubReleaseAsset{}, githubReleaseAsset{}, lastErr
		}

		var rel githubRelease
		if err := json.Unmarshal(body, &rel); err != nil {
			return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("decode release tag response: %w", err)
		}

		var asset githubReleaseAsset
		var checksum githubReleaseAsset
		for _, a := range rel.Assets {
			if a.Name == assetName {
				asset = a
			}
			if a.Name == checksumName || a.Name == "checksums.txt" {
				checksum = a
			}
		}
		if asset.URL == "" {
			return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("release tag %q has no asset %q", targetVersion, assetName)
		}

		requireChecksum := strings.TrimSpace(envOrDefault("CIWI_UPDATE_REQUIRE_CHECKSUM", "true")) != "false"
		if requireChecksum && checksum.URL == "" {
			return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("release tag %q has no checksum asset (expected %q)", targetVersion, checksumName)
		}
		return asset, checksum, nil
	}
	return githubReleaseAsset{}, githubReleaseAsset{}, lastErr
}

func startUpdateHelper(helperPath, targetPath, newBinaryPath string, parentPID int, restartArgs []string, serviceName string) error {
	args := []string{
		"update-helper",
		"--target", targetPath,
		"--new", newBinaryPath,
		"--pid", strconv.Itoa(parentPID),
	}
	if strings.TrimSpace(serviceName) != "" {
		args = append(args, "--service-name", strings.TrimSpace(serviceName))
	}
	for _, a := range restartArgs {
		args = append(args, "--arg", a)
	}

	cmd := exec.Command(helperPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func expectedAssetName(goos, goarch string) string {
	return updateutil.ExpectedAssetName(goos, goarch)
}

func currentVersion() string {
	return updateutil.CurrentVersion()
}

func looksLikeGoRunBinary(path string) bool {
	return updateutil.LooksLikeGoRunBinary(path)
}

func isVersionNewer(latest, current string) bool {
	return updateutil.IsVersionNewer(latest, current)
}

func isVersionDifferent(target, current string) bool {
	return updateutil.IsVersionDifferent(target, current)
}

func downloadUpdateAsset(ctx context.Context, assetURL, assetName string) (string, error) {
	client := updateHTTPClient(2 * time.Minute)
	var resp *http.Response
	var err error
	for attempt := 1; attempt <= updateNetworkMaxAttempts; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
		if reqErr != nil {
			return "", reqErr
		}
		req.Header.Set("Accept", "application/octet-stream")
		req.Header.Set("User-Agent", "ciwi-agent-updater")
		applyGitHubAuthHeader(req)
		resp, err = client.Do(req)
		if err != nil {
			if attempt < updateNetworkMaxAttempts && shouldRetryUpdateError(err) {
				wait := updateRetryDelay(attempt)
				slog.Warn("update asset download request failed; retrying", "asset_url", assetURL, "attempt", attempt, "next_wait", wait, "error", err)
				if !sleepWithContext(ctx, wait) {
					return "", context.Cause(ctx)
				}
				continue
			}
			return "", err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			_ = resp.Body.Close()
			err = fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
			if attempt < updateNetworkMaxAttempts && shouldRetryUpdateHTTPStatus(resp.StatusCode) {
				wait := updateRetryDelay(attempt)
				slog.Warn("update asset download returned retryable status; retrying", "asset_url", assetURL, "status", resp.StatusCode, "attempt", attempt, "next_wait", wait)
				if !sleepWithContext(ctx, wait) {
					return "", context.Cause(ctx)
				}
				continue
			}
			return "", err
		}
		break
	}
	defer resp.Body.Close()

	tmp := filepath.Join(os.TempDir(), "ciwi-agent-update-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
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

func downloadTextAsset(ctx context.Context, assetURL string) (string, error) {
	client := updateHTTPClient(30 * time.Second)
	var resp *http.Response
	var err error
	for attempt := 1; attempt <= updateNetworkMaxAttempts; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
		if reqErr != nil {
			return "", reqErr
		}
		req.Header.Set("Accept", "application/octet-stream")
		req.Header.Set("User-Agent", "ciwi-agent-updater")
		applyGitHubAuthHeader(req)
		resp, err = client.Do(req)
		if err != nil {
			if attempt < updateNetworkMaxAttempts && shouldRetryUpdateError(err) {
				wait := updateRetryDelay(attempt)
				slog.Warn("checksum asset download request failed; retrying", "asset_url", assetURL, "attempt", attempt, "next_wait", wait, "error", err)
				if !sleepWithContext(ctx, wait) {
					return "", context.Cause(ctx)
				}
				continue
			}
			return "", err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			_ = resp.Body.Close()
			err = fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
			if attempt < updateNetworkMaxAttempts && shouldRetryUpdateHTTPStatus(resp.StatusCode) {
				wait := updateRetryDelay(attempt)
				slog.Warn("checksum asset download returned retryable status; retrying", "asset_url", assetURL, "status", resp.StatusCode, "attempt", attempt, "next_wait", wait)
				if !sleepWithContext(ctx, wait) {
					return "", context.Cause(ctx)
				}
				continue
			}
			return "", err
		}
		break
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func verifyFileSHA256(path, assetName, checksumContent string) error {
	return updateutil.VerifyFileSHA256(path, assetName, checksumContent)
}

func exeExt() string {
	return updateutil.ExeExt()
}

func copyFile(src, dst string, mode os.FileMode) error {
	return updateutil.CopyFile(src, dst, mode)
}

func applyGitHubAuthHeader(req *http.Request) {
	if req == nil {
		return
	}
	token := strings.TrimSpace(envOrDefault("CIWI_GITHUB_TOKEN", ""))
	if token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func updateHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			ForceAttemptHTTP2:   false,
			DisableKeepAlives:   true,
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
		},
	}
}

func shouldRetryUpdateHTTPStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func shouldRetryUpdateError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	markers := []string{
		"http/2 stream",
		"stream was not closed cleanly",
		"remote end hung up unexpectedly",
		"unexpected eof",
		"eof",
		"connection reset",
		"connection refused",
		"connection timed out",
		"tls handshake timeout",
		"no such host",
		"temporary failure",
		"timeout",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func updateRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Second
	}
	if attempt == 2 {
		return 2 * time.Second
	}
	return 4 * time.Second
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func moveOrCopyFile(src, dst string, mode fs.FileMode) error {
	if strings.TrimSpace(src) == strings.TrimSpace(dst) {
		return nil
	}
	_ = os.Remove(dst)
	if err := os.Rename(src, dst); err == nil {
		return os.Chmod(dst, mode)
	}
	if err := copyFile(src, dst, mode); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func runLaunchctl(args ...string) error {
	launchctlPath := strings.TrimSpace(envOrDefault("CIWI_LAUNCHCTL_PATH", "/bin/launchctl"))
	if launchctlPath == "" {
		launchctlPath = "/bin/launchctl"
	}
	cmd := exec.Command(launchctlPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", launchctlPath, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func adHocSignBinary(path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("empty path")
	}
	codesignPath := strings.TrimSpace(envOrDefault("CIWI_CODESIGN_PATH", "/usr/bin/codesign"))
	if codesignPath == "" {
		codesignPath = "/usr/bin/codesign"
	}
	cmd := exec.Command(codesignPath, "--force", "--sign", "-", p)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s --force --sign - %s: %w (%s)", codesignPath, p, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func processRunning(pid int) (bool, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "process already finished") || strings.Contains(strings.ToLower(err.Error()), "no such process") {
		return false, nil
	}
	return false, nil
}
