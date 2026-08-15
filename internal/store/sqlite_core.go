package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/pipelinechain"
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

type PersistedPipelineChain struct {
	DBID        int64
	ProjectID   int64
	ProjectName string
	ChainID     string
	ChainName   string
	Pipelines   []string
	Position    int
}

type PersistedPipelineJob struct {
	ID                     string
	Needs                  []string
	ArtifactSources        []config.PipelineJobArtifactSource
	RunsOn                 map[string]string
	RequiresTools          map[string]string
	RequiresContainerTools map[string]string
	TimeoutSeconds         int
	Artifacts              []string
	Caches                 []config.PipelineJobCacheSpec
	MatrixInclude          []map[string]string
	Steps                  []config.PipelineJobStep
	Position               int
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	// SQLite allows one writer at a time. Keep a single pooled connection so
	// per-connection PRAGMAs (for example busy_timeout) are applied consistently.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.migrateSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func initializeCurrentSchema(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			source_kind TEXT NOT NULL DEFAULT 'vcs',
			config_path TEXT NOT NULL,
			config_yaml TEXT NOT NULL DEFAULT '',
			config_revision TEXT NOT NULL DEFAULT '',
			repo_url TEXT,
			repo_ref TEXT,
			config_file TEXT,
			loaded_commit TEXT NOT NULL DEFAULT '',
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
		`CREATE TABLE IF NOT EXISTS pipeline_chains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			chain_id TEXT NOT NULL,
			chain_name TEXT NOT NULL DEFAULT '',
			position INTEGER NOT NULL DEFAULT 0,
			pipelines_json TEXT NOT NULL DEFAULT '[]',
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL,
			UNIQUE(project_id, chain_id),
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS pipeline_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pipeline_id INTEGER NOT NULL,
			job_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			needs_json TEXT NOT NULL DEFAULT '[]',
			artifact_sources_json TEXT NOT NULL DEFAULT '[]',
			runs_on_json TEXT NOT NULL,
			requires_tools_json TEXT NOT NULL DEFAULT '{}',
			requires_container_tools_json TEXT NOT NULL DEFAULT '{}',
			requires_capabilities_json TEXT NOT NULL DEFAULT '{}',
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
			dependency_artifact_job_ids_json TEXT NOT NULL DEFAULT '[]',
			caches_json TEXT NOT NULL DEFAULT '[]',
			source_repo TEXT,
			source_ref TEXT,
			metadata_json TEXT NOT NULL,
			step_plan_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL,
			created_utc TEXT NOT NULL,
			started_utc TEXT,
			finished_utc TEXT,
			leased_by_agent_id TEXT,
			leased_utc TEXT,
			exit_code INTEGER,
			error_text TEXT,
			cache_stats_json TEXT NOT NULL DEFAULT '[]',
			runtime_capabilities_json TEXT NOT NULL DEFAULT '{}',
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
			total_count INTEGER NOT NULL DEFAULT 0,
			passed_count INTEGER NOT NULL DEFAULT 0,
			failed_count INTEGER NOT NULL DEFAULT 0,
			skipped_count INTEGER NOT NULL DEFAULT 0,
			created_utc TEXT NOT NULL,
			FOREIGN KEY(job_execution_id) REFERENCES job_executions(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS job_execution_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_execution_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			timestamp_utc TEXT NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_utc TEXT NOT NULL,
			FOREIGN KEY(job_execution_id) REFERENCES job_executions(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_job_executions_status_created ON job_executions(status, created_utc);`,
		`CREATE INDEX IF NOT EXISTS idx_job_execution_events_job_created ON job_execution_events(job_execution_id, created_utc);`,
		`CREATE INDEX IF NOT EXISTS idx_job_execution_events_identity ON job_execution_events(job_execution_id, event_type, timestamp_utc);`,
		`CREATE TABLE IF NOT EXISTS app_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_utc TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS command_receipts (
			command_key TEXT PRIMARY KEY,
			operation TEXT NOT NULL,
			request_fingerprint TEXT NOT NULL,
			status TEXT NOT NULL,
			result_json TEXT NOT NULL DEFAULT '',
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL
		);`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("create current schema: %w", err)
		}
	}
	return nil
}

func (s *Store) compact() error {
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint before vacuum: %w", err)
	}
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint after vacuum: %w", err)
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

	projectID, err := upsertProject(tx, cfg.Project.Name, configPath, repoURL, repoRef, configFile, "", now)
	if err != nil {
		return err
	}
	if err := replaceProjectDefinition(tx, projectID, cfg, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func replaceProjectDefinition(tx *sql.Tx, projectID int64, cfg config.File, now string) error {
	if err := pruneStaleProjectPipelines(tx, projectID, cfg.Pipelines); err != nil {
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
			needsJSON, _ := json.Marshal(j.Needs)
			artifactSourcesJSON, _ := json.Marshal(j.ArtifactSources)
			runsOnJSON, _ := json.Marshal(j.RunsOn)
			requiresToolsJSON, _ := json.Marshal(j.Requires.Tools)
			requiresContainerToolsJSON, _ := json.Marshal(j.Requires.Container.Tools)
			requiresCapsJSON := "{}"
			artifactsJSON, _ := json.Marshal(j.Artifacts)
			cachesJSON, _ := json.Marshal(config.EffectivePipelineJobCaches(j))
			matrixJSON, _ := json.Marshal(j.Matrix.Include)
			stepsJSON, _ := json.Marshal(j.Steps)

			if _, err := tx.Exec(`
				INSERT INTO pipeline_jobs (pipeline_id, job_id, position, needs_json, artifact_sources_json, runs_on_json, requires_tools_json, requires_container_tools_json, requires_capabilities_json, timeout_seconds, artifacts_json, caches_json, matrix_json, steps_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, pipelineDBID, j.ID, i, string(needsJSON), string(artifactSourcesJSON), string(runsOnJSON), string(requiresToolsJSON), string(requiresContainerToolsJSON), string(requiresCapsJSON), j.TimeoutSeconds, string(artifactsJSON), string(cachesJSON), string(matrixJSON), string(stepsJSON)); err != nil {
				return fmt.Errorf("insert pipeline job: %w", err)
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM pipeline_chains WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("clear pipeline chains: %w", err)
	}
	for position, ch := range cfg.PipelineChains {
		pipelines := pipelinechain.NormalizePipelines(ch.Pipelines)
		pipelinesJSON, _ := json.Marshal(pipelines)
		if _, err := tx.Exec(`
			INSERT INTO pipeline_chains (project_id, chain_id, chain_name, position, pipelines_json, created_utc, updated_utc)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, projectID, pipelinechain.ID(pipelines), pipelinechain.DisplayName(ch.Name, pipelines), position, string(pipelinesJSON), now, now); err != nil {
			return fmt.Errorf("insert pipeline chain: %w", err)
		}
	}

	return nil
}

func pruneStaleProjectPipelines(tx *sql.Tx, projectID int64, pipelines []config.Pipeline) error {
	if len(pipelines) == 0 {
		if _, err := tx.Exec(`DELETE FROM pipelines WHERE project_id = ?`, projectID); err != nil {
			return fmt.Errorf("clear project pipelines: %w", err)
		}
		return nil
	}
	placeholders := make([]string, 0, len(pipelines))
	args := make([]any, 0, 1+len(pipelines))
	args = append(args, projectID)
	for _, p := range pipelines {
		placeholders = append(placeholders, "?")
		args = append(args, p.ID)
	}
	query := `DELETE FROM pipelines WHERE project_id = ? AND pipeline_id NOT IN (` + strings.Join(placeholders, ",") + `)`
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("prune stale pipelines: %w", err)
	}
	return nil
}
