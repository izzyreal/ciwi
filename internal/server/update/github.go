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
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
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

	url := apiBase + "/repos/" + repo + "/releases/latest"
	requestLabel := "latest release"
	if targetTag != "" {
		url = apiBase + "/repos/" + repo + "/releases/tags/" + targetTag
		requestLabel = "release for tag " + targetTag
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestInfo{}, fmt.Errorf("create request: %w", err)
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
		return LatestInfo{}, fmt.Errorf("request %s: %w", requestLabel, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return LatestInfo{}, fmt.Errorf("%s query failed: status=%d body=%s", requestLabel, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel githubLatestRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return LatestInfo{}, fmt.Errorf("decode %s: %w", requestLabel, err)
	}

	assetName := updateutil.ExpectedAssetName(runtime.GOOS, runtime.GOARCH)
	if assetName == "" {
		return LatestInfo{}, fmt.Errorf("no known release asset naming for os=%s arch=%s", runtime.GOOS, runtime.GOARCH)
	}

	checksumName := strings.TrimSpace(opts.ChecksumAssetName)
	if checksumName == "" {
		checksumName = "ciwi-checksums.txt"
	}

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
		return LatestInfo{}, fmt.Errorf("latest release has no compatible asset %q", assetName)
	}
	if opts.RequireChecksum && checksum.URL == "" {
		return LatestInfo{}, fmt.Errorf("latest release has no checksum asset (expected %q)", checksumName)
	}

	return LatestInfo{
		TagName:       rel.TagName,
		HTMLURL:       rel.HTMLURL,
		Asset:         asset,
		ChecksumAsset: checksum,
	}, nil
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
