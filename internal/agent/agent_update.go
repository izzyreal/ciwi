package agent

import (
	"archive/zip"
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

var (
	agentUpdateRuntimeGOOS          = runtime.GOOS
	agentExecutablePathFn           = os.Executable
	agentAbsPathFn                  = filepath.Abs
	agentSelfUpdateServiceReasonFn  = selfUpdateServiceModeReason
	agentFetchReleaseAssetsForTagFn = fetchReleaseAssetsForTag
	agentDownloadUpdateAssetFn      = downloadUpdateAsset
	agentDownloadTextAssetFn        = downloadTextAsset
	agentVerifyFileSHA256Fn         = verifyFileSHA256
	agentHasDarwinUpdaterConfigFn   = hasDarwinUpdaterConfig
	agentStageDarwinUpdaterFn       = stageAndTriggerDarwinUpdater
	agentCopyFileFn                 = copyFile
	agentCopyDirFn                  = copyDir
	agentWindowsServiceInfoFn       = windowsServiceInfo
	agentStartUpdateHelperFn        = startUpdateHelper
	agentStartDarwinUpdaterFn       = startDarwinUpdater
	agentStopOwnDarwinLaunchAgentFn = stopOwnDarwinLaunchAgentForUpdate
	agentPIDFn                      = os.Getpid
	agentExitAfterDarwinUpdateFn    = func() {
		go os.Exit(0)
	}
	agentScheduleExitAfterUpdateFn = func() {
		go func() {
			time.Sleep(250 * time.Millisecond)
			os.Exit(0)
		}()
	}
)

func selfUpdateAndRestart(ctx context.Context, targetVersion, repository, apiBase string, restartArgs []string) error {
	exePath, err := agentExecutablePathFn()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exePath, _ = agentAbsPathFn(exePath)
	if looksLikeGoRunBinary(exePath) {
		return fmt.Errorf("self-update unavailable for go run binaries")
	}

	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" || !isVersionDifferent(targetVersion, currentVersion()) {
		return nil
	}
	if reason := agentSelfUpdateServiceReasonFn(); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	updateStarted := time.Now()
	slog.Info("agent self-update started", "target_version", targetVersion, "current_version", currentVersion(), "os", agentUpdateRuntimeGOOS, "arch", runtime.GOARCH)

	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	repository = strings.TrimSpace(repository)
	if repository == "" {
		repository = "izzyreal/ciwi"
	}

	assetName := expectedAssetName(agentUpdateRuntimeGOOS, runtime.GOARCH)
	if assetName == "" {
		return fmt.Errorf("no known update asset for os=%s arch=%s", agentUpdateRuntimeGOOS, runtime.GOARCH)
	}
	checksumName := strings.TrimSpace(envOrDefault("CIWI_UPDATE_CHECKSUM_ASSET", "ciwi-checksums.txt"))
	if checksumName == "" {
		checksumName = "ciwi-checksums.txt"
	}

	phaseStarted := time.Now()
	asset, checksumAsset, err := agentFetchReleaseAssetsForTagFn(ctx, apiBase, repository, targetVersion, assetName, checksumName)
	if err != nil {
		slog.Warn("agent self-update phase failed", "phase", "fetch_release_assets", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
		return err
	}
	slog.Info("agent self-update phase complete", "phase", "fetch_release_assets", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "asset", strings.TrimSpace(asset.Name), "has_checksum_asset", strings.TrimSpace(checksumAsset.URL) != "")

	phaseStarted = time.Now()
	assetPath, err := agentDownloadUpdateAssetFn(ctx, asset.URL, asset.Name)
	if err != nil {
		slog.Warn("agent self-update phase failed", "phase", "download_asset", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
		return fmt.Errorf("download update asset: %w", err)
	}
	slog.Info("agent self-update phase complete", "phase", "download_asset", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "asset", strings.TrimSpace(asset.Name))
	if checksumAsset.URL != "" {
		phaseStarted = time.Now()
		checksumText, err := agentDownloadTextAssetFn(ctx, checksumAsset.URL)
		if err != nil {
			slog.Warn("agent self-update phase failed", "phase", "download_checksum", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
			return fmt.Errorf("download checksum asset: %w", err)
		}
		slog.Info("agent self-update phase complete", "phase", "download_checksum", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "checksum_asset", strings.TrimSpace(checksumAsset.Name))
		phaseStarted = time.Now()
		if err := agentVerifyFileSHA256Fn(assetPath, asset.Name, checksumText); err != nil {
			slog.Warn("agent self-update phase failed", "phase", "verify_checksum", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		slog.Info("agent self-update phase complete", "phase", "verify_checksum", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "asset", strings.TrimSpace(asset.Name))
	}

	newBinPath := assetPath
	if agentUpdateRuntimeGOOS == "darwin" && strings.HasSuffix(strings.ToLower(strings.TrimSpace(asset.Name)), ".zip") {
		phaseStarted = time.Now()
		extractedBinPath, err := extractDarwinAppExecutable(assetPath)
		if err != nil {
			slog.Warn("agent self-update phase failed", "phase", "extract_darwin_app_bundle", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
			return fmt.Errorf("extract darwin app bundle: %w", err)
		}
		newBinPath = extractedBinPath
		slog.Info("agent self-update phase complete", "phase", "extract_darwin_app_bundle", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "binary_path", extractedBinPath)
	}

	if agentUpdateRuntimeGOOS == "darwin" && agentHasDarwinUpdaterConfigFn() {
		phaseStarted = time.Now()
		if err := agentStageDarwinUpdaterFn(targetVersion, asset.Name, exePath, newBinPath); err != nil {
			slog.Warn("agent self-update phase failed", "phase", "stage_and_trigger_darwin_updater", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
			return err
		}
		slog.Info("agent self-update phase complete", "phase", "stage_and_trigger_darwin_updater", "elapsed", time.Since(phaseStarted).Round(time.Millisecond))
		slog.Info("agent self-update handed off to darwin updater", "target_version", targetVersion, "elapsed_total", time.Since(updateStarted).Round(time.Millisecond))
		if err := agentStopOwnDarwinLaunchAgentFn(); err != nil {
			slog.Warn("agent self-update failed to stop own launchagent before exit", "error", err)
		}
		agentExitAfterDarwinUpdateFn()
		return nil
	}

	phaseStarted = time.Now()
	helperPath := filepath.Join(filepath.Dir(newBinPath), "ciwi-update-helper-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
	if err := agentCopyFileFn(exePath, helperPath, 0o755); err != nil {
		slog.Warn("agent self-update phase failed", "phase", "prepare_update_helper", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
		return fmt.Errorf("prepare update helper: %w", err)
	}
	slog.Info("agent self-update phase complete", "phase", "prepare_update_helper", "elapsed", time.Since(phaseStarted).Round(time.Millisecond))
	serviceName := ""
	if agentUpdateRuntimeGOOS == "windows" {
		if active, name := agentWindowsServiceInfoFn(); active {
			serviceName = strings.TrimSpace(name)
		}
	}

	phaseStarted = time.Now()
	if err := agentStartUpdateHelperFn(helperPath, exePath, newBinPath, agentPIDFn(), restartArgs, serviceName); err != nil {
		slog.Warn("agent self-update phase failed", "phase", "start_update_helper", "elapsed", time.Since(phaseStarted).Round(time.Millisecond), "error", err)
		return fmt.Errorf("start update helper: %w", err)
	}
	slog.Info("agent self-update phase complete", "phase", "start_update_helper", "elapsed", time.Since(phaseStarted).Round(time.Millisecond))
	slog.Info("agent self-update handed off to update helper", "target_version", targetVersion, "elapsed_total", time.Since(updateStarted).Round(time.Millisecond))
	agentScheduleExitAfterUpdateFn()
	return nil
}

func hasDarwinUpdaterConfig() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	return strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_LABEL", "")) != "" &&
		strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_PLIST", "")) != ""
}

func stageAndTriggerDarwinUpdater(targetVersion, assetName, targetBinary, stagedBinary string) error {
	agentLabel := strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_LABEL", ""))
	agentPlist := strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_PLIST", ""))
	if agentLabel == "" || agentPlist == "" {
		return fmt.Errorf("missing launchd agent configuration")
	}

	workDir := agentWorkDir()
	manifestPath := strings.TrimSpace(envOrDefault("CIWI_AGENT_UPDATE_MANIFEST", filepath.Join(workDir, "updates", "pending.json")))
	if manifestPath == "" {
		return fmt.Errorf("unable to resolve CIWI_AGENT_UPDATE_MANIFEST path")
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return fmt.Errorf("create update manifest directory: %w", err)
	}
	targetBundle := strings.TrimSpace(envOrDefault("CIWI_AGENT_APP_BUNDLE", ""))
	stagePath := filepath.Join(filepath.Dir(manifestPath), filepath.Base(stagedBinary))
	stagedBundle := ""
	if targetBundle != "" {
		bundleRoot := findAppBundleRoot(stagedBinary)
		if bundleRoot == "" {
			return fmt.Errorf("staged darwin app bundle not found for %s", stagedBinary)
		}
		stagedBundle = filepath.Join(filepath.Dir(manifestPath), filepath.Base(bundleRoot))
		if err := moveOrCopyDir(bundleRoot, stagedBundle); err != nil {
			return fmt.Errorf("stage update app bundle: %w", err)
		}
		stagePath = filepath.Join(stagedBundle, "Contents", "MacOS", "ciwi")
	} else {
		if err := moveOrCopyFile(stagedBinary, stagePath, 0o755); err != nil {
			return fmt.Errorf("stage update binary: %w", err)
		}
	}
	hash, err := fileSHA256(stagePath)
	if err != nil {
		return fmt.Errorf("hash staged update binary: %w", err)
	}
	manifest, err := darwinupdater.BuildManifest(targetVersion, assetName, targetBinary, stagePath, hash, agentLabel, agentPlist, targetBundle, stagedBundle, defaultAgentID(), os.Getpid())
	if err != nil {
		return fmt.Errorf("build update manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		return fmt.Errorf("write update manifest: %w", err)
	}
	helperPath := ""
	if targetBundle != "" {
		helperBundle := filepath.Join(filepath.Dir(manifestPath), "ciwi-darwin-updater-helper.app")
		_ = os.RemoveAll(helperBundle)
		if err := agentCopyDirFn(targetBundle, helperBundle); err != nil {
			_ = os.Remove(manifestPath)
			return fmt.Errorf("prepare darwin updater helper app bundle: %w", err)
		}
		helperPath = filepath.Join(helperBundle, "Contents", "MacOS", "ciwi")
	} else {
		helperPath = filepath.Join(filepath.Dir(manifestPath), "ciwi-darwin-updater-helper-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
		if err := agentCopyFileFn(targetBinary, helperPath, 0o755); err != nil {
			_ = os.Remove(manifestPath)
			return fmt.Errorf("prepare darwin updater helper: %w", err)
		}
	}
	if err := agentStartDarwinUpdaterFn(helperPath, manifestPath); err != nil {
		_ = os.Remove(manifestPath)
		if targetBundle != "" {
			_ = os.RemoveAll(findAppBundleRoot(helperPath))
		} else {
			_ = os.Remove(helperPath)
		}
		return fmt.Errorf("start darwin updater helper: %w", err)
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

func startDarwinUpdater(helperPath, manifestPath string) error {
	cmd := exec.Command(helperPath, "apply-staged-agent-update", "--manifest", manifestPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func stopOwnDarwinLaunchAgentForUpdate() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	label := strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_LABEL", ""))
	plist := strings.TrimSpace(envOrDefault("CIWI_AGENT_LAUNCHD_PLIST", ""))
	if label == "" || plist == "" {
		return nil
	}
	uid := strconv.Itoa(os.Getuid())
	service := "gui/" + uid + "/" + label
	domain := "gui/" + uid
	if err := runLaunchctl("bootout", service); err != nil {
		_ = runLaunchctl("bootout", domain, plist)
	}
	if err := runLaunchctl("disable", service); err != nil {
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

func extractDarwinAppExecutable(assetPath string) (string, error) {
	assetPath = strings.TrimSpace(assetPath)
	if assetPath == "" {
		return "", fmt.Errorf("empty darwin asset path")
	}
	extractRoot := filepath.Join(os.TempDir(), "ciwi-agent-update-app-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		return "", err
	}
	if err := unzipArchive(assetPath, extractRoot); err != nil {
		return "", err
	}
	var bundlePath string
	_ = filepath.WalkDir(extractRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || bundlePath != "" || !d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".app") {
			bundlePath = path
			return filepath.SkipDir
		}
		return nil
	})
	if bundlePath == "" {
		return "", fmt.Errorf("no .app bundle found in %s", assetPath)
	}
	binPath := filepath.Join(bundlePath, "Contents", "MacOS", "ciwi")
	if _, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("app bundle executable missing: %w", err)
	}
	return binPath, nil
}

func unzipArchive(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		targetPath := filepath.Join(dst, f.Name)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, filepath.Clean(dst)+string(filepath.Separator)) && cleanTarget != filepath.Clean(dst) {
			return fmt.Errorf("zip entry escapes destination: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = out.Close()
			_ = rc.Close()
			return err
		}
		_ = out.Close()
		_ = rc.Close()
	}
	return nil
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

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		return copyFile(path, targetPath, info.Mode())
	})
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

func moveOrCopyDir(src, dst string) error {
	if strings.TrimSpace(src) == strings.TrimSpace(dst) {
		return nil
	}
	_ = os.RemoveAll(dst)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := agentCopyDirFn(src, dst); err != nil {
		return err
	}
	_ = os.RemoveAll(src)
	return nil
}

func findAppBundleRoot(path string) string {
	cur := filepath.Clean(strings.TrimSpace(path))
	for cur != "" && cur != "." && cur != string(filepath.Separator) {
		if strings.EqualFold(filepath.Ext(cur), ".app") {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
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
