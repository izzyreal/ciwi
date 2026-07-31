package store

import (
	"errors"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestManagedYAMLProjectLifecycleAndConflicts(t *testing.T) {
	s := openTestStore(t)
	rawInitial := managedYAMLTestConfig("Alpha", "build")
	cfgInitial, err := config.Parse([]byte(rawInitial), "managed-initial")
	if err != nil {
		t.Fatalf("parse initial config: %v", err)
	}
	created, err := s.CreateManagedYAMLProject(cfgInitial, rawInitial)
	if err != nil {
		t.Fatalf("create managed YAML: %v", err)
	}
	if created.ProjectID <= 0 || created.ProjectName != "Alpha" || created.Revision != ManagedYAMLRevision(rawInitial) || created.Pipelines != 1 {
		t.Fatalf("unexpected created definition: %+v", created)
	}

	stored, err := s.GetManagedYAMLProject(created.ProjectID)
	if err != nil {
		t.Fatalf("get managed YAML: %v", err)
	}
	if stored.YAML != rawInitial || stored.Revision != created.Revision {
		t.Fatalf("managed YAML was not stored verbatim: %+v", stored)
	}
	project, err := s.GetProjectByID(created.ProjectID)
	if err != nil {
		t.Fatalf("get managed project: %v", err)
	}
	if project.SourceKind != protocol.ProjectSourceManagedYAML || project.ConfigPath != "" || project.RepoURL != "" || project.ConfigFile != "" {
		t.Fatalf("unexpected managed project metadata: %+v", project)
	}

	rawUpdated := managedYAMLTestConfig("Beta", "release")
	cfgUpdated, err := config.Parse([]byte(rawUpdated), "managed-updated")
	if err != nil {
		t.Fatalf("parse updated config: %v", err)
	}
	updated, err := s.UpdateManagedYAMLProject(created.ProjectID, created.Revision, cfgUpdated, rawUpdated)
	if err != nil {
		t.Fatalf("update managed YAML: %v", err)
	}
	if updated.ProjectID != created.ProjectID || updated.ProjectName != "Beta" || updated.Revision == created.Revision {
		t.Fatalf("unexpected updated definition: %+v", updated)
	}
	detail, err := s.GetProjectDetail(created.ProjectID)
	if err != nil {
		t.Fatalf("get updated project detail: %v", err)
	}
	if detail.Name != "Beta" || detail.SourceKind != protocol.ProjectSourceManagedYAML || len(detail.Pipelines) != 1 || detail.Pipelines[0].PipelineID != "release" {
		t.Fatalf("parsed project definition was not replaced: %+v", detail)
	}

	if _, err := s.UpdateManagedYAMLProject(created.ProjectID, created.Revision, cfgInitial, rawInitial); !errors.Is(err, ErrManagedYAMLRevisionConflict) {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}
	afterConflict, err := s.GetManagedYAMLProject(created.ProjectID)
	if err != nil {
		t.Fatalf("get after stale update: %v", err)
	}
	if afterConflict.YAML != rawUpdated || afterConflict.Revision != updated.Revision {
		t.Fatalf("stale update changed stored YAML: %+v", afterConflict)
	}

	duplicateRaw := managedYAMLTestConfig("beta", "other")
	duplicateCfg, err := config.Parse([]byte(duplicateRaw), "managed-duplicate")
	if err != nil {
		t.Fatalf("parse duplicate config: %v", err)
	}
	if _, err := s.CreateManagedYAMLProject(duplicateCfg, duplicateRaw); !errors.Is(err, ErrManagedYAMLNameConflict) {
		t.Fatalf("expected case-insensitive name conflict, got %v", err)
	}
	if err := s.ValidateManagedYAMLName("BETA", created.ProjectID); err != nil {
		t.Fatalf("expected validation to ignore current project: %v", err)
	}
	if err := s.ValidateManagedYAMLName("BETA", 0); !errors.Is(err, ErrManagedYAMLNameConflict) {
		t.Fatalf("expected validation name conflict, got %v", err)
	}
}

func TestManagedYAMLOperationsRejectVCSProject(t *testing.T) {
	s := openTestStore(t)
	raw := managedYAMLTestConfig("Repository project", "build")
	cfg, err := config.Parse([]byte(raw), "repository")
	if err != nil {
		t.Fatalf("parse repository config: %v", err)
	}
	if err := s.LoadConfig(cfg, "https://example.test/repo.git@main:ciwi-project.yaml", "https://example.test/repo.git", "main", "ciwi-project.yaml"); err != nil {
		t.Fatalf("load repository config: %v", err)
	}
	project, err := s.GetProjectByName("Repository project")
	if err != nil {
		t.Fatalf("get repository project: %v", err)
	}
	if project.SourceKind != protocol.ProjectSourceVCS {
		t.Fatalf("expected migrated/default VCS source kind, got %+v", project)
	}
	if _, err := s.GetManagedYAMLProject(project.ID); !errors.Is(err, ErrProjectIsNotManagedYAML) {
		t.Fatalf("expected managed YAML read rejection, got %v", err)
	}
	if _, err := s.UpdateManagedYAMLProject(project.ID, "revision", cfg, raw); !errors.Is(err, ErrProjectIsNotManagedYAML) {
		t.Fatalf("expected managed YAML update rejection, got %v", err)
	}
}

func managedYAMLTestConfig(projectName, pipelineID string) string {
	return `version: 1
project:
  name: ` + projectName + `
pipelines:
  - id: ` + pipelineID + `
    trigger: manual
    jobs:
      - id: run
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        steps:
          - run: echo ok
`
}
