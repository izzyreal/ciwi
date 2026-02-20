package server

import (
	"context"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	serverproject "github.com/izzyreal/ciwi/internal/server/project"
)

func TestMaybeRunPostUpdateProjectReloadGuards(t *testing.T) {
	// nil state should be a no-op
	var nilState *stateStore
	nilState.maybeRunPostUpdateProjectReload(context.Background())

	// state without db should be a no-op
	s := &stateStore{}
	s.maybeRunPostUpdateProjectReload(context.Background())
}

func TestMaybeRunPostUpdateProjectReloadSchedulesWorker(t *testing.T) {
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
		t.Fatalf("set pending reload flag: %v", err)
	}

	oldFetch := fetchProjectConfigAndIcon
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (serverproject.RepoFetchResult, error) {
		return serverproject.RepoFetchResult{ConfigContent: testConfigYAML}, nil
	}

	s.maybeRunPostUpdateProjectReload(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		v, ok, _ := db.GetAppState(updateReloadProjectsPendingKey)
		if ok && v == "0" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected pending reload flag to be cleared asynchronously")
}

func TestRunPostUpdateProjectReloadFailureKeepsPendingAndMarksMessage(t *testing.T) {
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
		t.Fatalf("set pending reload flag: %v", err)
	}

	oldFetch := fetchProjectConfigAndIcon
	t.Cleanup(func() { fetchProjectConfigAndIcon = oldFetch })
	fetchProjectConfigAndIcon = func(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (serverproject.RepoFetchResult, error) {
		return serverproject.RepoFetchResult{}, context.DeadlineExceeded
	}

	s.runPostUpdateProjectReload(context.Background())

	v, ok, err := db.GetAppState(updateReloadProjectsPendingKey)
	if err != nil {
		t.Fatalf("get pending flag: %v", err)
	}
	if !ok || v != "1" {
		t.Fatalf("expected pending flag to stay set after failures, got ok=%v value=%q", ok, v)
	}
	msg, ok, err := db.GetAppState("update_message")
	if err != nil {
		t.Fatalf("get update message: %v", err)
	}
	if !ok || msg != "post-update project reload incomplete; retry on next restart" {
		t.Fatalf("unexpected update message: ok=%v msg=%q", ok, msg)
	}
}
