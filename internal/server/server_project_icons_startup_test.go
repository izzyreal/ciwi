package server

import (
	"context"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	serverproject "github.com/izzyreal/ciwi/internal/server/project"
)

func TestWarmProjectIconsOnStartupLoadsRepoProjectIcons(t *testing.T) {
	db := openServerReloadTestStore(t)
	s := &stateStore{
		db:           db,
		projectIcons: map[int64]projectIconState{},
	}

	repoCfg, err := config.Parse([]byte(testConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse repo config: %v", err)
	}
	if err := db.LoadConfig(repoCfg, "https://example/repo.git@main:ciwi-project.yaml", "https://example/repo.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load repo config: %v", err)
	}

	localCfg, err := config.Parse([]byte(localProjectConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse local config: %v", err)
	}
	if err := db.LoadConfig(localCfg, "/tmp/local/ciwi-project.yaml", "", "", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load local config: %v", err)
	}

	fetchCalls := 0
	oldFetch := fetchProjectConfigAndIcon
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (serverproject.RepoFetchResult, error) {
		fetchCalls++
		return serverproject.RepoFetchResult{
			ConfigContent:    testConfigYAML,
			IconContentType:  "image/png",
			IconContentBytes: []byte("repo-icon"),
		}, nil
	}
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })

	s.warmProjectIconsOnStartup(context.Background())

	if fetchCalls != 1 {
		t.Fatalf("expected one repo fetch call, got %d", fetchCalls)
	}

	repoProject, err := db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("get repo project: %v", err)
	}
	icon, ok := s.getProjectIcon(repoProject.ID)
	if !ok {
		t.Fatalf("expected warmed icon for repo project")
	}
	if icon.ContentType != "image/png" {
		t.Fatalf("unexpected icon type: %q", icon.ContentType)
	}
	if string(icon.Data) != "repo-icon" {
		t.Fatalf("unexpected icon bytes: %q", string(icon.Data))
	}

	localProject, err := db.GetProjectByName("local-only")
	if err != nil {
		t.Fatalf("get local project: %v", err)
	}
	if _, ok := s.getProjectIcon(localProject.ID); ok {
		t.Fatalf("expected no icon for local non-repo project")
	}
}

const localProjectConfigYAML = `version: 1
project:
  name: local-only
pipelines:
  - id: local
    jobs:
      - id: build
        steps:
          - run: echo hello
`
