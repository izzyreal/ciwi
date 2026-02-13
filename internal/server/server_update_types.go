package server

import "sync"

type updateState struct {
	mu          sync.Mutex
	inProgress  bool
	lastMessage string
	agentTarget string
}

type updateCheckResponse struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	AssetName       string `json:"asset_name,omitempty"`
	Message         string `json:"message,omitempty"`
}

type updateTagsResponse struct {
	Tags           []string `json:"tags"`
	CurrentVersion string   `json:"current_version"`
}

type updateApplyResponse struct {
	Updated        bool   `json:"updated"`
	Message        string `json:"message,omitempty"`
	Target         string `json:"target,omitempty"`
	TargetVersion  string `json:"target_version,omitempty"`
	CurrentVersion string `json:"current_version,omitempty"`
	Staged         bool   `json:"staged,omitempty"`
}

type updateStatusResponse struct {
	Status map[string]string `json:"status"`
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

type githubRepoTag struct {
	Name string `json:"name"`
}

type latestUpdateInfo struct {
	TagName       string
	HTMLURL       string
	Asset         githubReleaseAsset
	ChecksumAsset githubReleaseAsset
}
