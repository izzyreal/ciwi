package config

import "testing"

func TestParseAcceptsJobGoCache(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
project:
  name: ciwi
pipelines:
  - id: build
    jobs:
      - id: compile
        timeout_seconds: 60
        go_cache: {}
        steps:
          - run: go build ./...
`), "test-go-cache")
	if err != nil {
		t.Fatalf("parse go_cache config: %v", err)
	}
}

func TestEffectivePipelineJobCachesGoCacheEnabledByDefault(t *testing.T) {
	job := PipelineJobSpec{
		Caches: []PipelineJobCacheSpec{
			{ID: "fetchcontent", Env: "FETCHCONTENT_BASE_DIR"},
		},
		GoCache: &PipelineJobGoCacheSpec{},
	}
	got := EffectivePipelineJobCaches(job)
	if len(got) != 3 {
		t.Fatalf("expected 3 caches, got %d", len(got))
	}
	if got[0].ID != "fetchcontent" {
		t.Fatalf("expected existing cache first, got %q", got[0].ID)
	}
	if got[1].ID != ManagedGoBuildCacheID || got[1].Env != ManagedGoBuildCacheEnv {
		t.Fatalf("unexpected go build cache: %+v", got[1])
	}
	if got[2].ID != ManagedGoModCacheID || got[2].Env != ManagedGoModCacheEnv {
		t.Fatalf("unexpected go mod cache: %+v", got[2])
	}
}

func TestEffectivePipelineJobCachesDoesNotDuplicateManagedEntries(t *testing.T) {
	job := PipelineJobSpec{
		Caches: []PipelineJobCacheSpec{
			{ID: "custom-go", Env: ManagedGoBuildCacheEnv},
			{ID: ManagedGoModCacheID, Env: "CUSTOM_GOMOD_ENV"},
		},
		GoCache: &PipelineJobGoCacheSpec{},
	}
	got := EffectivePipelineJobCaches(job)
	if len(got) != 2 {
		t.Fatalf("expected 2 caches, got %d", len(got))
	}
}

func TestEffectivePipelineJobCachesCanDisableGoCache(t *testing.T) {
	enabled := false
	job := PipelineJobSpec{
		Caches: []PipelineJobCacheSpec{
			{ID: "fetchcontent", Env: "FETCHCONTENT_BASE_DIR"},
		},
		GoCache: &PipelineJobGoCacheSpec{Enabled: &enabled},
	}
	got := EffectivePipelineJobCaches(job)
	if len(got) != 1 {
		t.Fatalf("expected 1 cache, got %d", len(got))
	}
}
