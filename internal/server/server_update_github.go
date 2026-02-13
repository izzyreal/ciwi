package server

import (
	"context"
	"strings"

	serverupdate "github.com/izzyreal/ciwi/internal/server/update"
	"github.com/izzyreal/ciwi/internal/updateutil"
)

func (s *stateStore) fetchLatestUpdateInfo(ctx context.Context) (latestUpdateInfo, error) {
	return s.fetchUpdateInfoForTag(ctx, "")
}

func (s *stateStore) fetchUpdateInfoForTag(ctx context.Context, targetTag string) (latestUpdateInfo, error) {
	info, err := serverupdate.FetchLatestInfo(ctx, serverupdate.FetchInfoOptions{
		APIBase:           envOrDefault("CIWI_UPDATE_API_BASE", "https://api.github.com"),
		Repo:              envOrDefault("CIWI_UPDATE_REPO", "izzyreal/ciwi"),
		TargetTag:         targetTag,
		ChecksumAssetName: envOrDefault("CIWI_UPDATE_CHECKSUM_ASSET", "ciwi-checksums.txt"),
		RequireChecksum:   strings.TrimSpace(envOrDefault("CIWI_UPDATE_REQUIRE_CHECKSUM", "true")) != "false",
		AuthToken:         envOrDefault("CIWI_GITHUB_TOKEN", ""),
	})
	if err != nil {
		return latestUpdateInfo{}, err
	}
	return latestUpdateInfo{
		TagName: info.TagName,
		HTMLURL: info.HTMLURL,
		Asset: githubReleaseAsset{
			Name: info.Asset.Name,
			URL:  info.Asset.URL,
		},
		ChecksumAsset: githubReleaseAsset{
			Name: info.ChecksumAsset.Name,
			URL:  info.ChecksumAsset.URL,
		},
	}, nil
}

func (s *stateStore) fetchUpdateTags(ctx context.Context) ([]string, error) {
	return serverupdate.FetchTags(ctx, serverupdate.FetchTagsOptions{
		APIBase:   envOrDefault("CIWI_UPDATE_API_BASE", "https://api.github.com"),
		Repo:      envOrDefault("CIWI_UPDATE_REPO", "izzyreal/ciwi"),
		AuthToken: envOrDefault("CIWI_GITHUB_TOKEN", ""),
	})
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
