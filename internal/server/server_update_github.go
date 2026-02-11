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
	return updateutil.ExpectedAssetName(goos, goarch)
}

func isVersionNewer(latest, current string) bool {
	return updateutil.IsVersionNewer(latest, current)
}

func currentVersion() string {
	return updateutil.CurrentVersion()
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
