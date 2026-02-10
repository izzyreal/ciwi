package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/izzyreal/ciwi/internal/config"
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
	SourceRepo  string
	SourceRef   string
	Jobs        []PersistedPipelineJob
}

type PersistedPipelineJob struct {
	ID             string
	RunsOn         map[string]string
	TimeoutSeconds int
	Artifacts      []string
	MatrixInclude  []map[string]string
	Steps          []string
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
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			config_path TEXT NOT NULL,
			repo_url TEXT,
			repo_ref TEXT,
			config_file TEXT,
			created_utc TEXT NOT NULL,
			updated_utc TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS pipelines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			pipeline_id TEXT NOT NULL,
			trigger_mode TEXT,
			source_repo TEXT,
			source_ref TEXT,
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
			timeout_seconds INTEGER NOT NULL,
			artifacts_json TEXT NOT NULL DEFAULT '[]',
			matrix_json TEXT NOT NULL,
			steps_json TEXT NOT NULL,
			FOREIGN KEY(pipeline_id) REFERENCES pipelines(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			script TEXT NOT NULL,
			required_capabilities_json TEXT NOT NULL,
			timeout_seconds INTEGER NOT NULL,
			artifact_globs_json TEXT NOT NULL DEFAULT '[]',
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
			output_text TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS job_artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL,
			path TEXT NOT NULL,
			stored_rel TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			created_utc TEXT NOT NULL,
			FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_status_created ON jobs(status, created_utc);`,
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
	if err := s.addColumnIfMissing("pipeline_jobs", "artifacts_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("jobs", "artifact_globs_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
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
			artifactsJSON, _ := json.Marshal(j.Artifacts)
			matrixJSON, _ := json.Marshal(j.Matrix.Include)
			steps := make([]string, 0, len(j.Steps))
			for _, step := range j.Steps {
				steps = append(steps, step.Run)
			}
			stepsJSON, _ := json.Marshal(steps)

			if _, err := tx.Exec(`
				INSERT INTO pipeline_jobs (pipeline_id, job_id, position, runs_on_json, timeout_seconds, artifacts_json, matrix_json, steps_json)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, pipelineDBID, j.ID, i, string(runsOnJSON), j.TimeoutSeconds, string(artifactsJSON), string(matrixJSON), string(stepsJSON)); err != nil {
				return fmt.Errorf("insert pipeline job: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
