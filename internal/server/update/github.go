package update

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

type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type LatestInfo struct {
	TagName       string
	HTMLURL       string
	Asset         ReleaseAsset
	ChecksumAsset ReleaseAsset
}

type FetchInfoOptions struct {
	APIBase           string
	Repo              string
	TargetTag         string
	ChecksumAssetName string
	RequireChecksum   bool
	AuthToken         string
	HTTPClient        *http.Client
}

type FetchTagsOptions struct {
	APIBase    string
	Repo       string
	AuthToken  string
	HTTPClient *http.Client
}

type githubLatestRelease struct {
	TagName    string         `json:"tag_name"`
	HTMLURL    string         `json:"html_url"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []ReleaseAsset `json:"assets"`
}

type githubRepoTag struct {
	Name string `json:"name"`
}

func FetchLatestInfo(ctx context.Context, opts FetchInfoOptions) (LatestInfo, error) {
	apiBase := strings.TrimRight(strings.TrimSpace(opts.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		repo = "izzyreal/ciwi"
	}
	targetTag := strings.TrimSpace(opts.TargetTag)

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	assetName := updateutil.ExpectedAssetName(runtime.GOOS, runtime.GOARCH)
	if assetName == "" {
		return LatestInfo{}, fmt.Errorf("no known release asset naming for os=%s arch=%s", runtime.GOOS, runtime.GOARCH)
	}

	checksumName := strings.TrimSpace(opts.ChecksumAssetName)
	if checksumName == "" {
		checksumName = "ciwi-checksums.txt"
	}

	if targetTag != "" {
		var rel githubLatestRelease
		requestLabel := "release for tag " + targetTag
		url := apiBase + "/repos/" + repo + "/releases/tags/" + targetTag
		if err := fetchGitHubJSON(ctx, client, url, requestLabel, opts.AuthToken, &rel); err != nil {
			return LatestInfo{}, err
		}
		return compatibleReleaseInfo(rel, requestLabel, assetName, checksumName, opts.RequireChecksum)
	}

	var latest githubLatestRelease
	latestURL := apiBase + "/repos/" + repo + "/releases/latest"
	if err := fetchGitHubJSON(ctx, client, latestURL, "latest release", opts.AuthToken, &latest); err != nil {
		return LatestInfo{}, err
	}
	info, latestErr := compatibleReleaseInfo(latest, "latest release", assetName, checksumName, opts.RequireChecksum)
	if latestErr == nil {
		return info, nil
	}

	var releases []githubLatestRelease
	releasesURL := apiBase + "/repos/" + repo + "/releases?per_page=100"
	if err := fetchGitHubJSON(ctx, client, releasesURL, "recent releases", opts.AuthToken, &releases); err != nil {
		return LatestInfo{}, fmt.Errorf("%v; find previous complete release: %w", latestErr, err)
	}
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		info, err := compatibleReleaseInfo(rel, fmt.Sprintf("release %q", strings.TrimSpace(rel.TagName)), assetName, checksumName, opts.RequireChecksum)
		if err == nil {
			return info, nil
		}
	}
	return LatestInfo{}, fmt.Errorf("%v; no complete stable release found among %d recent releases", latestErr, len(releases))
}

// FetchAvailableInfos returns complete stable releases in GitHub's newest-first
// order. If the releases listing is unavailable, it falls back to the single
// release returned by FetchLatestInfo so update checks remain useful.
func FetchAvailableInfos(ctx context.Context, opts FetchInfoOptions) ([]LatestInfo, error) {
	apiBase := strings.TrimRight(strings.TrimSpace(opts.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		repo = "izzyreal/ciwi"
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	assetName := updateutil.ExpectedAssetName(runtime.GOOS, runtime.GOARCH)
	if assetName == "" {
		return nil, fmt.Errorf("no known release asset naming for os=%s arch=%s", runtime.GOOS, runtime.GOARCH)
	}
	checksumName := strings.TrimSpace(opts.ChecksumAssetName)
	if checksumName == "" {
		checksumName = "ciwi-checksums.txt"
	}

	var releases []githubLatestRelease
	releasesURL := apiBase + "/repos/" + repo + "/releases?per_page=100"
	if err := fetchGitHubJSON(ctx, client, releasesURL, "recent releases", opts.AuthToken, &releases); err != nil {
		info, latestErr := FetchLatestInfo(ctx, opts)
		if latestErr != nil {
			return nil, fmt.Errorf("list recent releases: %v; fetch latest release: %w", err, latestErr)
		}
		return []LatestInfo{info}, nil
	}

	infos := make([]LatestInfo, 0, len(releases))
	seen := make(map[string]struct{}, len(releases))
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		tag := strings.TrimSpace(rel.TagName)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		info, err := compatibleReleaseInfo(rel, fmt.Sprintf("release %q", tag), assetName, checksumName, opts.RequireChecksum)
		if err != nil {
			continue
		}
		seen[tag] = struct{}{}
		infos = append(infos, info)
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("no complete stable release found among %d recent releases", len(releases))
	}
	return infos, nil
}

func compatibleReleaseInfo(rel githubLatestRelease, label, assetName, checksumName string, requireChecksum bool) (LatestInfo, error) {
	var asset ReleaseAsset
	var checksum ReleaseAsset
	for _, a := range rel.Assets {
		if a.Name == assetName {
			asset = a
		}
		if a.Name == checksumName || a.Name == "checksums.txt" {
			checksum = a
		}
	}
	if asset.URL == "" {
		return LatestInfo{}, fmt.Errorf("%s has no compatible asset %q", label, assetName)
	}
	if requireChecksum && checksum.URL == "" {
		return LatestInfo{}, fmt.Errorf("%s has no checksum asset (expected %q)", label, checksumName)
	}

	return LatestInfo{
		TagName:       rel.TagName,
		HTMLURL:       rel.HTMLURL,
		Asset:         asset,
		ChecksumAsset: checksum,
	}, nil
}

func fetchGitHubJSON(ctx context.Context, client *http.Client, url, requestLabel, authToken string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ciwi-updater")
	applyGitHubAuthHeader(req, authToken)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", requestLabel, err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if readErr != nil {
		return fmt.Errorf("read %s response: %w", requestLabel, readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s query failed: status=%d body=%s", requestLabel, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode %s: %w", requestLabel, err)
	}
	return nil
}

func FetchTags(ctx context.Context, opts FetchTagsOptions) ([]string, error) {
	apiBase := strings.TrimRight(strings.TrimSpace(opts.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		repo = "izzyreal/ciwi"
	}
	url := apiBase + "/repos/" + repo + "/tags?per_page=100"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ciwi-updater")
	applyGitHubAuthHeader(req, opts.AuthToken)

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
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

func applyGitHubAuthHeader(req *http.Request, token string) {
	if req == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}
