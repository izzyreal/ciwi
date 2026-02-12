package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/updateutil"
)

func (s *stateStore) fetchLatestUpdateInfo(ctx context.Context) (latestUpdateInfo, error) {
	return s.fetchUpdateInfoForTag(ctx, "")
}

func (s *stateStore) fetchUpdateInfoForTag(ctx context.Context, targetTag string) (latestUpdateInfo, error) {
	apiBase := strings.TrimRight(envOrDefault("CIWI_UPDATE_API_BASE", "https://api.github.com"), "/")
	repo := strings.TrimSpace(envOrDefault("CIWI_UPDATE_REPO", "izzyreal/ciwi"))
	targetTag = strings.TrimSpace(targetTag)
	url := apiBase + "/repos/" + repo + "/releases/latest"
	requestLabel := "latest release"
	if targetTag != "" {
		url = apiBase + "/repos/" + repo + "/releases/tags/" + targetTag
		requestLabel = "release for tag " + targetTag
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return latestUpdateInfo{}, fmt.Errorf("create request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ciwi-updater")
	applyGitHubAuthHeader(req)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return latestUpdateInfo{}, fmt.Errorf("request %s: %w", requestLabel, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return latestUpdateInfo{}, fmt.Errorf("%s query failed: status=%d body=%s", requestLabel, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel githubLatestRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return latestUpdateInfo{}, fmt.Errorf("decode %s: %w", requestLabel, err)
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

func (s *stateStore) fetchUpdateTags(ctx context.Context) ([]string, error) {
	apiBase := strings.TrimRight(envOrDefault("CIWI_UPDATE_API_BASE", "https://api.github.com"), "/")
	repo := strings.TrimSpace(envOrDefault("CIWI_UPDATE_REPO", "izzyreal/ciwi"))
	url := apiBase + "/repos/" + repo + "/tags?per_page=100"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ciwi-updater")
	applyGitHubAuthHeader(req)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request tags: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tags query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tags []githubRepoTag
	if err := json.Unmarshal(body, &tags); err != nil {
		return nil, fmt.Errorf("decode tags response: %w", err)
	}
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, t := range tags {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out, nil
}

func expectedAssetName(goos, goarch string) string {
	return updateutil.ExpectedAssetName(goos, goarch)
}

func isVersionNewer(latest, current string) bool {
	return updateutil.IsVersionNewer(latest, current)
}

func isVersionDifferent(target, current string) bool {
	return updateutil.IsVersionDifferent(target, current)
}

func currentVersion() string {
	return updateutil.CurrentVersion()
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
