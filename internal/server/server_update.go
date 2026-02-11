package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type updateState struct {
	mu          sync.Mutex
	inProgress  bool
	lastMessage string
}

type updateCheckResponse struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	AssetName       string `json:"asset_name,omitempty"`
	Message         string `json:"message,omitempty"`
}

type githubLatestRelease struct {
	TagName string               `json:"tag_name"`
	HTMLURL string               `json:"html_url"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type latestUpdateInfo struct {
	TagName       string
	HTMLURL       string
	Asset         githubReleaseAsset
	ChecksumAsset githubReleaseAsset
}

func (s *stateStore) updateCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info, err := s.fetchLatestUpdateInfo(r.Context())
	if err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_checked_utc": time.Now().UTC().Format(time.RFC3339Nano),
			"update_current_version":  currentVersion(),
			"update_message":          err.Error(),
			"update_available":        "0",
		})
		writeJSON(w, http.StatusOK, updateCheckResponse{
			CurrentVersion: currentVersion(),
			Message:        err.Error(),
		})
		return
	}

	resp := updateCheckResponse{
		CurrentVersion:  currentVersion(),
		LatestVersion:   info.TagName,
		UpdateAvailable: isVersionNewer(info.TagName, currentVersion()),
		ReleaseURL:      info.HTMLURL,
		AssetName:       info.Asset.Name,
	}
	if !resp.UpdateAvailable {
		resp.Message = "already up to date"
	}
	_ = s.persistUpdateStatus(map[string]string{
		"update_last_checked_utc": time.Now().UTC().Format(time.RFC3339Nano),
		"update_current_version":  currentVersion(),
		"update_latest_version":   info.TagName,
		"update_release_url":      info.HTMLURL,
		"update_asset_name":       info.Asset.Name,
		"update_available":        boolString(resp.UpdateAvailable),
		"update_message":          resp.Message,
	})
	writeJSON(w, http.StatusOK, resp)
}

func (s *stateStore) updateApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.update.mu.Lock()
	if s.update.inProgress {
		s.update.mu.Unlock()
		http.Error(w, "update already in progress", http.StatusConflict)
		return
	}
	s.update.inProgress = true
	s.update.lastMessage = "update started"
	s.update.mu.Unlock()
	defer func() {
		s.update.mu.Lock()
		s.update.inProgress = false
		s.update.mu.Unlock()
	}()
	_ = s.persistUpdateStatus(map[string]string{
		"update_last_apply_utc":    time.Now().UTC().Format(time.RFC3339Nano),
		"update_last_apply_status": "running",
		"update_message":           "update started",
	})

	exePath, err := os.Executable()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve executable path: %v", err), http.StatusInternalServerError)
		return
	}
	exePath, _ = filepath.Abs(exePath)
	if looksLikeGoRunBinary(exePath) {
		msg := "self-update is unavailable for go run binaries; run built ciwi binary instead"
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           msg,
		})
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	info, err := s.fetchLatestUpdateInfo(r.Context())
	if err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           err.Error(),
		})
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !isVersionNewer(info.TagName, currentVersion()) {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "noop",
			"update_message":           "already up to date",
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"updated": false,
			"message": "already up to date",
		})
		return
	}

	newBinPath, err := downloadUpdateAsset(r.Context(), info.Asset.URL, info.Asset.Name)
	if err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           "download update asset: " + err.Error(),
		})
		http.Error(w, fmt.Sprintf("download update asset: %v", err), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(info.ChecksumAsset.URL) != "" {
		checksumText, err := downloadTextAsset(r.Context(), info.ChecksumAsset.URL)
		if err != nil {
			_ = s.persistUpdateStatus(map[string]string{
				"update_last_apply_status": "failed",
				"update_message":           "download checksum asset: " + err.Error(),
			})
			http.Error(w, fmt.Sprintf("download checksum asset: %v", err), http.StatusBadRequest)
			return
		}
		if err := verifyFileSHA256(newBinPath, info.Asset.Name, checksumText); err != nil {
			_ = s.persistUpdateStatus(map[string]string{
				"update_last_apply_status": "failed",
				"update_message":           "checksum verification failed: " + err.Error(),
			})
			http.Error(w, fmt.Sprintf("checksum verification failed: %v", err), http.StatusBadRequest)
			return
		}
	}

	helperPath := filepath.Join(filepath.Dir(newBinPath), "ciwi-update-helper-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
	if err := copyFile(exePath, helperPath, 0o755); err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           "prepare update helper: " + err.Error(),
		})
		http.Error(w, fmt.Sprintf("prepare update helper: %v", err), http.StatusInternalServerError)
		return
	}

	if err := startUpdateHelper(helperPath, exePath, newBinPath, os.Getpid(), os.Args[1:]); err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           "start update helper: " + err.Error(),
		})
		http.Error(w, fmt.Sprintf("start update helper: %v", err), http.StatusInternalServerError)
		return
	}
	_ = s.persistUpdateStatus(map[string]string{
		"update_last_apply_status": "success",
		"update_message":           "update helper started, restarting",
		"update_latest_version":    info.TagName,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"updated":         true,
		"message":         "update helper started, restarting",
		"target_version":  info.TagName,
		"current_version": currentVersion(),
	})

	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
}

func (s *stateStore) updateStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, err := s.db.ListAppState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": state})
}

func (s *stateStore) fetchLatestUpdateInfo(ctx context.Context) (latestUpdateInfo, error) {
	apiBase := strings.TrimRight(envOrDefault("CIWI_UPDATE_API_BASE", "https://api.github.com"), "/")
	repo := strings.TrimSpace(envOrDefault("CIWI_UPDATE_REPO", "izzyreal/ciwi"))
	url := apiBase + "/repos/" + repo + "/releases/latest"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return latestUpdateInfo{}, fmt.Errorf("create request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ciwi-updater")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return latestUpdateInfo{}, fmt.Errorf("request latest release: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return latestUpdateInfo{}, fmt.Errorf("latest release query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel githubLatestRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return latestUpdateInfo{}, fmt.Errorf("decode latest release: %w", err)
	}
	assetName := expectedAssetName(runtime.GOOS, runtime.GOARCH)
	if assetName == "" {
		return latestUpdateInfo{}, fmt.Errorf("no known release asset naming for os=%s arch=%s", runtime.GOOS, runtime.GOARCH)
	}
	checksumName := strings.TrimSpace(envOrDefault("CIWI_UPDATE_CHECKSUM_ASSET", "ciwi-checksums.txt"))
	if checksumName == "" {
		checksumName = "ciwi-checksums.txt"
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
		return latestUpdateInfo{}, fmt.Errorf("latest release has no compatible asset %q", assetName)
	}
	requireChecksum := strings.TrimSpace(envOrDefault("CIWI_UPDATE_REQUIRE_CHECKSUM", "true")) != "false"
	if requireChecksum && checksum.URL == "" {
		return latestUpdateInfo{}, fmt.Errorf("latest release has no checksum asset (expected %q)", checksumName)
	}

	return latestUpdateInfo{TagName: rel.TagName, HTMLURL: rel.HTMLURL, Asset: asset, ChecksumAsset: checksum}, nil
}

func expectedAssetName(goos, goarch string) string {
	switch {
	case goos == "linux" && goarch == "amd64":
		return "ciwi-linux-amd64"
	case goos == "linux" && goarch == "arm64":
		return "ciwi-linux-arm64"
	case goos == "darwin" && goarch == "amd64":
		return "ciwi-darwin-amd64"
	case goos == "darwin" && goarch == "arm64":
		return "ciwi-darwin-arm64"
	case goos == "windows" && goarch == "amd64":
		return "ciwi-windows-amd64.exe"
	case goos == "windows" && goarch == "arm64":
		return "ciwi-windows-arm64.exe"
	}
	return ""
}

func isVersionNewer(latest, current string) bool {
	l := parseSemver(strings.TrimSpace(latest))
	c := parseSemver(strings.TrimSpace(current))
	if l.valid && c.valid {
		if l.major != c.major {
			return l.major > c.major
		}
		if l.minor != c.minor {
			return l.minor > c.minor
		}
		return l.patch > c.patch
	}
	return strings.TrimPrefix(latest, "v") != strings.TrimPrefix(current, "v")
}

type semver struct {
	major int
	minor int
	patch int
	valid bool
}

func parseSemver(v string) semver {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semver{}
	}
	maj, e1 := strconv.Atoi(parts[0])
	min, e2 := strconv.Atoi(parts[1])
	pat, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return semver{}
	}
	return semver{major: maj, minor: min, patch: pat, valid: true}
}

func currentVersion() string {
	v := strings.TrimSpace(envOrDefault("CIWI_VERSION", "dev"))
	if v == "" {
		return "dev"
	}
	return v
}

func shouldRequestAgentUpdate(agentVersion, targetVersion string) bool {
	agentVersion = strings.TrimSpace(agentVersion)
	targetVersion = strings.TrimSpace(targetVersion)
	if agentVersion == "" || targetVersion == "" {
		return false
	}
	if !isVersionNewer(targetVersion, agentVersion) {
		return false
	}
	return strings.TrimSpace(envOrDefault("CIWI_AGENT_AUTO_UPDATE", "true")) != "false"
}

func looksLikeGoRunBinary(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	return strings.Contains(p, "/go-build") || strings.Contains(p, "/temp/")
}

func downloadUpdateAsset(ctx context.Context, assetURL, assetName string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ciwi-updater")
	req = req.WithContext(ctx)
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	tmp := filepath.Join(os.TempDir(), "ciwi-update-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
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
	req, err := http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ciwi-updater")
	req = req.WithContext(ctx)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
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

func verifyFileSHA256(path, assetName, checksumContent string) error {
	want := ""
	for _, line := range strings.Split(checksumContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s", assetName)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := fmt.Sprintf("%x", h.Sum(nil))
	if got != want {
		return fmt.Errorf("sha256 mismatch: got=%s want=%s", got, want)
	}
	return nil
}

func exeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func (s *stateStore) persistUpdateStatus(values map[string]string) error {
	for k, v := range values {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if err := s.db.SetAppState(k, v); err != nil {
			return err
		}
	}
	return nil
}

func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
