package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type Store struct {
	db *sql.DB
}

type PersistedPipeline struct {
	DBID        int64
	ProjectID   int64
	ProjectName string
	PipelineID  string
	Trigger     string
	DependsOn   []string
	SourceRepo  string
	SourceRef   string
	Versioning  config.PipelineVersioning
	Jobs        []PersistedPipelineJob
}

type PersistedPipelineJob struct {
	ID             string
	RunsOn         map[string]string
	RequiresTools  map[string]string
	TimeoutSeconds int
	Artifacts      []string
	Caches         []config.PipelineJobCacheSpec
	MatrixInclude  []map[string]string
	Steps          []config.PipelineJobStep
	Position       int
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA busy_timeout=5000;`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			config_path TEXT NOT NULL,
			repo_url TEXT,
			repo_ref TEXT,
			config_file TEXT,
			vault_connection_id INTEGER,
			vault_connection_name TEXT,
			project_secrets_json TEXT NOT NULL DEFAULT '[]',
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS vault_connections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			url TEXT NOT NULL,
			auth_method TEXT NOT NULL,
			approle_mount TEXT NOT NULL,
			role_id TEXT NOT NULL,
			secret_id_file TEXT,
			secret_id_env TEXT,
			namespace TEXT,
			kv_default_mount TEXT,
			kv_default_version INTEGER NOT NULL DEFAULT 2,
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS pipelines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			pipeline_id TEXT NOT NULL,
			trigger_mode TEXT,
			depends_on_json TEXT NOT NULL DEFAULT '[]',
			source_repo TEXT,
			source_ref TEXT,
			versioning_json TEXT NOT NULL DEFAULT '{}',
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL,
			UNIQUE(project_id, pipeline_id),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS pipeline_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pipeline_id INTEGER NOT NULL,
			job_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			runs_on_json TEXT NOT NULL,
			requires_tools_json TEXT NOT NULL DEFAULT '{}',
			timeout_seconds INTEGER NOT NULL,
			artifacts_json TEXT NOT NULL DEFAULT '[]',
			caches_json TEXT NOT NULL DEFAULT '[]',
			matrix_json TEXT NOT NULL,
			steps_json TEXT NOT NULL,
			FOREIGN KEY(pipeline_id) REFERENCES pipelines(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS job_executions (
			id TEXT PRIMARY KEY,
			script TEXT NOT NULL,
			env_json TEXT NOT NULL DEFAULT '{}',
			required_capabilities_json TEXT NOT NULL,
			timeout_seconds INTEGER NOT NULL,
			artifact_globs_json TEXT NOT NULL DEFAULT '[]',
			caches_json TEXT NOT NULL DEFAULT '[]',
			source_repo TEXT,
			source_ref TEXT,
			metadata_json TEXT NOT NULL,
			status TEXT NOT NULL,
			created_utc TEXT NOT NULL,
			started_utc TEXT,
			finished_utc TEXT,
			leased_by_agent_id TEXT,
			leased_utc TEXT,
			exit_code INTEGER,
			error_text TEXT,
			output_text TEXT,
			current_step_text TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS job_execution_artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_execution_id TEXT NOT NULL,
			path TEXT NOT NULL,
			stored_rel TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			created_utc TEXT NOT NULL,
			FOREIGN KEY(job_execution_id) REFERENCES job_executions(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS job_execution_test_reports (
			job_execution_id TEXT PRIMARY KEY,
			report_json TEXT NOT NULL,
			created_utc TEXT NOT NULL,
			FOREIGN KEY(job_execution_id) REFERENCES job_executions(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_job_executions_status_created ON job_executions(status, created_utc);`,
		`CREATE TABLE IF NOT EXISTS app_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_utc TEXT NOT NULL
		);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}
	if err := s.addColumnIfMissing("projects", "repo_url", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("projects", "repo_ref", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("projects", "config_file", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("projects", "vault_connection_id", "INTEGER"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("projects", "vault_connection_name", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("projects", "project_secrets_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("pipeline_jobs", "artifacts_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("pipeline_jobs", "caches_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("pipeline_jobs", "requires_tools_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("pipelines", "depends_on_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("pipelines", "versioning_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("job_executions", "artifact_globs_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("job_executions", "caches_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("job_executions", "env_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("job_executions", "current_step_text", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func (s *Store) addColumnIfMissing(table, col, typ string) error {
	_, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, typ))
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return fmt.Errorf("add column %s.%s: %w", table, col, err)
	}
	return nil
}

func (s *Store) LoadConfig(cfg config.File, configPath, repoURL, repoRef, configFile string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	projectID, err := upsertProject(tx, cfg.Project.Name, configPath, repoURL, repoRef, configFile, now)
	if err != nil {
		return err
	}
	if err := applyProjectVaultConfig(tx, projectID, cfg.Project.Vault, now); err != nil {
		return err
	}

	for _, p := range cfg.Pipelines {
		pipelineDBID, err := upsertPipeline(tx, projectID, p, now)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(`DELETE FROM pipeline_jobs WHERE pipeline_id = ?`, pipelineDBID); err != nil {
			return fmt.Errorf("clear pipeline jobs: %w", err)
		}

		for i, j := range p.Jobs {
			runsOnJSON, _ := json.Marshal(j.RunsOn)
			requiresToolsJSON, _ := json.Marshal(j.Requires.Tools)
			artifactsJSON, _ := json.Marshal(j.Artifacts)
			cachesJSON, _ := json.Marshal(j.Caches)
			matrixJSON, _ := json.Marshal(j.Matrix.Include)
			stepsJSON, _ := json.Marshal(j.Steps)

			if _, err := tx.Exec(`
				INSERT INTO pipeline_jobs (pipeline_id, job_id, position, runs_on_json, requires_tools_json, timeout_seconds, artifacts_json, caches_json, matrix_json, steps_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, pipelineDBID, j.ID, i, string(runsOnJSON), string(requiresToolsJSON), j.TimeoutSeconds, string(artifactsJSON), string(cachesJSON), string(matrixJSON), string(stepsJSON)); err != nil {
				return fmt.Errorf("insert pipeline job: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func applyProjectVaultConfig(tx *sql.Tx, projectID int64, vault *config.ProjectVault, now string) error {
	if vault == nil {
		return nil
	}

	secrets := make([]protocol.ProjectSecretSpec, 0, len(vault.Secrets))
	for _, sec := range vault.Secrets {
		secrets = append(secrets, protocol.ProjectSecretSpec{
			Name:      sec.Name,
			Mount:     sec.Mount,
			Path:      sec.Path,
			Key:       sec.Key,
			KVVersion: sec.KVVersion,
		})
	}
	secretsJSON, _ := json.Marshal(secrets)

	connName := strings.TrimSpace(vault.Connection)
	var connID any
	if connName != "" {
		var id int64
		if err := tx.QueryRow(`SELECT id FROM vault_connections WHERE name = ?`, connName).Scan(&id); err == nil {
			connID = id
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("resolve vault connection %q: %w", connName, err)
		}
	}

	if _, err := tx.Exec(`
		UPDATE projects
		SET vault_connection_id = ?, vault_connection_name = ?, project_secrets_json = ?, updated_utc = ?
		WHERE id = ?
	`, connID, connName, string(secretsJSON), now, projectID); err != nil {
		return fmt.Errorf("apply project vault config: %w", err)
	}
	return nil
}
