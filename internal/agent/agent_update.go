package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type githubReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

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
	if targetVersion == "" || !isVersionNewer(targetVersion, currentVersion()) {
		return nil
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

	helperPath := filepath.Join(filepath.Dir(newBinPath), "ciwi-update-helper-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
	if err := copyFile(exePath, helperPath, 0o755); err != nil {
		return fmt.Errorf("prepare update helper: %w", err)
	}
	if err := startUpdateHelper(helperPath, exePath, newBinPath, os.Getpid(), restartArgs); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}

	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func fetchReleaseAssetsForTag(ctx context.Context, apiBase, repository, targetVersion, assetName, checksumName string) (githubReleaseAsset, githubReleaseAsset, error) {
	url := apiBase + "/repos/" + repository + "/releases/tags/" + strings.TrimSpace(targetVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ciwi-agent-updater")

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("request release for tag: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubReleaseAsset{}, githubReleaseAsset{}, fmt.Errorf("release tag query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
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

func startUpdateHelper(helperPath, targetPath, newBinaryPath string, parentPID int, restartArgs []string) error {
	args := []string{
		"update-helper",
		"--target", targetPath,
		"--new", newBinaryPath,
		"--pid", strconv.Itoa(parentPID),
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

func currentVersion() string {
	v := strings.TrimSpace(envOrDefault("CIWI_VERSION", "dev"))
	if v == "" {
		return "dev"
	}
	return v
}

func looksLikeGoRunBinary(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	return strings.Contains(p, "/go-build") || strings.Contains(p, "/temp/")
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

func downloadUpdateAsset(ctx context.Context, assetURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ciwi-agent-updater")
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ciwi-agent-updater")
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
