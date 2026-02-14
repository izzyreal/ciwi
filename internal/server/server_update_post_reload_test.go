package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	serverproject "github.com/izzyreal/ciwi/internal/server/project"
	"github.com/izzyreal/ciwi/internal/store"
)

func TestIsRootRepoProject(t *testing.T) {
	if !isRootRepoProject(protocol.ProjectSummary{RepoURL: "https://example/repo.git", ConfigFile: "ciwi-project.yaml"}) {
		t.Fatalf("expected root repo project")
	}
	if isRootRepoProject(protocol.ProjectSummary{RepoURL: "", ConfigFile: "ciwi-project.yaml"}) {
		t.Fatalf("expected empty repo_url to be non-root")
	}
	if isRootRepoProject(protocol.ProjectSummary{RepoURL: "https://example/repo.git", ConfigFile: "configs/ciwi-project.yaml"}) {
		t.Fatalf("expected nested config path to be non-root")
	}
}

func TestRunPostUpdateProjectReloadReloadsRootProjectsAndClearsPending(t *testing.T) {
	db := openServerReloadTestStore(t)
	s := &stateStore{
		db:           db,
		projectIcons: map[int64]projectIconState{},
	}

	cfg, err := config.Parse([]byte(testConfigYAML), "ciwi-project.yaml")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if err := db.LoadConfig(cfg, "https://example/repo.git@main:ciwi-project.yaml", "https://example/repo.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := db.SetAppState(updateReloadProjectsPendingKey, "1"); err != nil {
		t.Fatalf("set pending reload: %v", err)
	}

	oldFetch := fetchProjectConfigAndIcon
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (serverproject.RepoFetchResult, error) {
		return serverproject.RepoFetchResult{
			ConfigContent:    testConfigYAML,
			IconContentType:  "image/png",
			IconContentBytes: []byte("png-bytes"),
		}, nil
	}
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })

	s.runPostUpdateProjectReload(context.Background())

	pending, ok, err := db.GetAppState(updateReloadProjectsPendingKey)
	if err != nil {
		t.Fatalf("get pending state: %v", err)
	}
	if !ok || pending != "0" {
		t.Fatalf("expected pending reload to clear to 0, got ok=%v value=%q", ok, pending)
	}

	project, err := db.GetProjectByName("ciwi")
	if err != nil {
		t.Fatalf("get project by name: %v", err)
	}
	icon, ok := s.getProjectIcon(project.ID)
	if !ok {
		t.Fatalf("expected project icon to be repopulated")
	}
	if icon.ContentType != "image/png" {
		t.Fatalf("unexpected icon type: %q", icon.ContentType)
	}
	if string(icon.Data) != "png-bytes" {
		t.Fatalf("unexpected icon bytes: %q", string(icon.Data))
	}
}

func openServerReloadTestStore(t *testing.T) *store.Store {
	t.Helper()
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
