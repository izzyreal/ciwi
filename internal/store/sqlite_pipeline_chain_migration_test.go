package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/pipelinechain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestPipelineChainIdentityMigrationDerivesKeysNamesAndOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			config_path TEXT NOT NULL,
			repo_url TEXT,
			repo_ref TEXT,
			config_file TEXT,
			loaded_commit TEXT NOT NULL DEFAULT '',
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL
		)`,
		`CREATE TABLE pipeline_chains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			chain_id TEXT NOT NULL,
			pipelines_json TEXT NOT NULL DEFAULT '[]',
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL,
			UNIQUE(project_id, chain_id),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		)`,
		`INSERT INTO projects (id, name, config_path, created_utc, updated_utc) VALUES (1, 'legacy', 'legacy.yaml', 'now', 'now')`,
		`INSERT INTO pipeline_chains (project_id, chain_id, pipelines_json, created_utc, updated_utc) VALUES (1, 'build-release', '["build","release"]', 'now', 'now')`,
		`INSERT INTO pipeline_chains (project_id, chain_id, pipelines_json, created_utc, updated_utc) VALUES (1, 'same-workflow', '["build","release"]', 'now', 'now')`,
		`INSERT INTO pipeline_chains (project_id, chain_id, pipelines_json, created_utc, updated_utc) VALUES (1, 'package', '["package"]', 'now', 'now')`,
	} {
		if _, err := legacy.Exec(stmt); err != nil {
			_ = legacy.Close()
			t.Fatalf("prepare legacy database: %v", err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	defer s.Close()
	project, err := s.GetProjectByID(1)
	if err != nil {
		t.Fatalf("get migrated project: %v", err)
	}
	if project.SourceKind != protocol.ProjectSourceVCS {
		t.Fatalf("expected existing project to migrate as VCS, got %+v", project)
	}

	chains, err := s.listPipelineChainsByProjectID(1)
	if err != nil {
		t.Fatalf("list migrated chains: %v", err)
	}
	if len(chains) != 2 {
		t.Fatalf("expected duplicate sequence to collapse, got %+v", chains)
	}
	if chains[0].ChainID != pipelinechain.ID([]string{"build", "release"}) || chains[0].ChainName != "build → release" || chains[0].Position != 0 {
		t.Fatalf("unexpected first migrated chain: %+v", chains[0])
	}
	if chains[1].ChainID != pipelinechain.ID([]string{"package"}) || chains[1].ChainName != "package" || chains[1].Position != 1 {
		t.Fatalf("unexpected second migrated chain: %+v", chains[1])
	}
}

func TestDerivedPipelineChainIdentityIsProjectScoped(t *testing.T) {
	s := openTestStore(t)
	for i, projectName := range []string{"alpha", "beta"} {
		cfg, err := config.Parse([]byte(fmt.Sprintf(`
version: 1
project:
  name: %s
pipelines:
  - id: build
    jobs:
      - id: compile
        runs_on:
          executor: script
          shell: posix
        timeout_seconds: 60
        steps:
          - run: echo build
pipeline_chains:
  - name: %s release
    pipelines: [build]
`, projectName, projectName)), projectName+".yaml")
		if err != nil {
			t.Fatalf("parse %s config: %v", projectName, err)
		}
		if err := s.LoadConfig(cfg, fmt.Sprintf("project-%d.yaml", i), "", "", "ciwi-project.yaml"); err != nil {
			t.Fatalf("load %s config: %v", projectName, err)
		}
	}

	alpha, err := s.GetProjectByName("alpha")
	if err != nil {
		t.Fatalf("get alpha project: %v", err)
	}
	beta, err := s.GetProjectByName("beta")
	if err != nil {
		t.Fatalf("get beta project: %v", err)
	}
	alphaDetail, err := s.GetProjectDetail(alpha.ID)
	if err != nil {
		t.Fatalf("get alpha detail: %v", err)
	}
	betaDetail, err := s.GetProjectDetail(beta.ID)
	if err != nil {
		t.Fatalf("get beta detail: %v", err)
	}
	if len(alphaDetail.PipelineChains) != 1 || len(betaDetail.PipelineChains) != 1 {
		t.Fatalf("expected one chain per project: alpha=%+v beta=%+v", alphaDetail.PipelineChains, betaDetail.PipelineChains)
	}
	if alphaDetail.PipelineChains[0].ID != betaDetail.PipelineChains[0].ID {
		t.Fatalf("identical sequences should have the same derived id")
	}
	alphaChain, err := s.GetPipelineChain(alpha.ID, alphaDetail.PipelineChains[0].ID)
	if err != nil {
		t.Fatalf("get alpha chain: %v", err)
	}
	betaChain, err := s.GetPipelineChain(beta.ID, betaDetail.PipelineChains[0].ID)
	if err != nil {
		t.Fatalf("get beta chain: %v", err)
	}
	if alphaChain.ProjectName != "alpha" || betaChain.ProjectName != "beta" {
		t.Fatalf("project-scoped lookup crossed projects: alpha=%+v beta=%+v", alphaChain, betaChain)
	}
}
