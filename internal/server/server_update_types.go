package server

import "sync"

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
