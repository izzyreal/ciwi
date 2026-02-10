package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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

func upsertProject(tx *sql.Tx, name, configPath, repoURL, repoRef, configFile, now string) (int64, error) {
	if _, err := tx.Exec(`
		INSERT INTO projects (name, config_path, repo_url, repo_ref, config_file, created_utc, updated_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			config_path=excluded.config_path,
			repo_url=excluded.repo_url,
			repo_ref=excluded.repo_ref,
			config_file=excluded.config_file,
			updated_utc=excluded.updated_utc
	`, name, configPath, repoURL, repoRef, configFile, now, now); err != nil {
		return 0, fmt.Errorf("upsert project: %w", err)
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM projects WHERE name = ?`, name).Scan(&id); err != nil {
		return 0, fmt.Errorf("fetch project id: %w", err)
	}
	return id, nil
}

func upsertPipeline(tx *sql.Tx, projectID int64, p config.Pipeline, now string) (int64, error) {
	if _, err := tx.Exec(`
		INSERT INTO pipelines (project_id, pipeline_id, trigger_mode, source_repo, source_ref, created_utc, updated_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, pipeline_id)
		DO UPDATE SET trigger_mode=excluded.trigger_mode, source_repo=excluded.source_repo, source_ref=excluded.source_ref, updated_utc=excluded.updated_utc
	`, projectID, p.ID, p.Trigger, p.Source.Repo, p.Source.Ref, now, now); err != nil {
		return 0, fmt.Errorf("upsert pipeline: %w", err)
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM pipelines WHERE project_id = ? AND pipeline_id = ?`, projectID, p.ID).Scan(&id); err != nil {
		return 0, fmt.Errorf("fetch pipeline id: %w", err)
	}
	return id, nil
}

func (s *Store) ListProjects() ([]protocol.ProjectSummary, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.config_path, p.repo_url, p.repo_ref, p.config_file, pl.id, pl.pipeline_id, pl.trigger_mode, pl.source_repo, pl.source_ref
		FROM projects p
		LEFT JOIN pipelines pl ON pl.project_id = p.id
		ORDER BY p.name, pl.pipeline_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	projectsByID := map[int64]*protocol.ProjectSummary{}
	order := make([]int64, 0)

	for rows.Next() {
		var projectID int64
		var projectName, configPath string
		var repoURL, repoRef, configFile sql.NullString
		var pipelineID sql.NullInt64
		var pipelineName, trigger, sourceRepo, sourceRef sql.NullString

		if err := rows.Scan(&projectID, &projectName, &configPath, &repoURL, &repoRef, &configFile, &pipelineID, &pipelineName, &trigger, &sourceRepo, &sourceRef); err != nil {
			return nil, fmt.Errorf("scan project row: %w", err)
		}

		project, ok := projectsByID[projectID]
		if !ok {
			project = &protocol.ProjectSummary{
				ID:         projectID,
				Name:       projectName,
				ConfigPath: configPath,
				RepoURL:    repoURL.String,
				RepoRef:    repoRef.String,
				ConfigFile: configFile.String,
			}
			projectsByID[projectID] = project
			order = append(order, projectID)
		}

		if pipelineID.Valid {
			project.Pipelines = append(project.Pipelines, protocol.PipelineSummary{
				ID:         pipelineID.Int64,
				PipelineID: pipelineName.String,
				Trigger:    trigger.String,
				SourceRepo: sourceRepo.String,
				SourceRef:  sourceRef.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project rows: %w", err)
	}

	projects := make([]protocol.ProjectSummary, 0, len(order))
	for _, id := range order {
		projects = append(projects, *projectsByID[id])
	}
	return projects, nil
}

func (s *Store) GetProjectByID(id int64) (protocol.ProjectSummary, error) {
	var p protocol.ProjectSummary
	row := s.db.QueryRow(`
		SELECT id, name, config_path, repo_url, repo_ref, config_file
		FROM projects
		WHERE id = ?
	`, id)
	if err := row.Scan(&p.ID, &p.Name, &p.ConfigPath, &p.RepoURL, &p.RepoRef, &p.ConfigFile); err != nil {
		if err == sql.ErrNoRows {
			return protocol.ProjectSummary{}, fmt.Errorf("project not found")
		}
		return protocol.ProjectSummary{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (s *Store) GetProjectDetail(id int64) (protocol.ProjectDetail, error) {
	project, err := s.GetProjectByID(id)
	if err != nil {
		return protocol.ProjectDetail{}, err
	}

	rows, err := s.db.Query(`
		SELECT id, pipeline_id, trigger_mode, source_repo, source_ref
		FROM pipelines
		WHERE project_id = ?
		ORDER BY pipeline_id
	`, id)
	if err != nil {
		return protocol.ProjectDetail{}, fmt.Errorf("list pipelines: %w", err)
	}
	defer rows.Close()

	detail := protocol.ProjectDetail{
		ID:         project.ID,
		Name:       project.Name,
		RepoURL:    project.RepoURL,
		RepoRef:    project.RepoRef,
		ConfigFile: project.ConfigFile,
	}

	for rows.Next() {
		var p protocol.PipelineDetail
		if err := rows.Scan(&p.ID, &p.PipelineID, &p.Trigger, &p.SourceRepo, &p.SourceRef); err != nil {
			return protocol.ProjectDetail{}, fmt.Errorf("scan pipeline: %w", err)
		}
		persistedJobs, err := s.listPipelineJobs(p.ID)
		if err != nil {
			return protocol.ProjectDetail{}, err
		}
		p.Jobs = make([]protocol.PipelineJobDetail, 0, len(persistedJobs))
		for _, j := range persistedJobs {
			d := protocol.PipelineJobDetail{
				ID:             j.ID,
				TimeoutSeconds: j.TimeoutSeconds,
				RunsOn:         cloneMap(j.RunsOn),
				Artifacts:      append([]string(nil), j.Artifacts...),
				Steps:          append([]string(nil), j.Steps...),
			}
			for idx, vars := range j.MatrixInclude {
				v := cloneMap(vars)
				d.MatrixIncludes = append(d.MatrixIncludes, protocol.MatrixInclude{
					Index: idx,
					Name:  v["name"],
					Vars:  v,
				})
			}
			p.Jobs = append(p.Jobs, d)
		}
		detail.Pipelines = append(detail.Pipelines, p)
	}
	if err := rows.Err(); err != nil {
		return protocol.ProjectDetail{}, fmt.Errorf("iterate pipelines: %w", err)
	}

	return detail, nil
}

func (s *Store) GetPipelineByDBID(id int64) (PersistedPipeline, error) {
	var p PersistedPipeline
	row := s.db.QueryRow(`
		SELECT pl.id, pl.project_id, p.name, pl.pipeline_id, pl.trigger_mode, pl.source_repo, pl.source_ref
		FROM pipelines pl
		JOIN projects p ON p.id = pl.project_id
		WHERE pl.id = ?
	`, id)
	if err := row.Scan(&p.DBID, &p.ProjectID, &p.ProjectName, &p.PipelineID, &p.Trigger, &p.SourceRepo, &p.SourceRef); err != nil {
		if err == sql.ErrNoRows {
			return p, fmt.Errorf("pipeline not found")
		}
		return p, fmt.Errorf("get pipeline: %w", err)
	}

	jobs, err := s.listPipelineJobs(p.DBID)
	if err != nil {
		return p, err
	}
	p.Jobs = jobs
	return p, nil
}

func (s *Store) GetPipelineByProjectAndID(projectName, pipelineID string) (PersistedPipeline, error) {
	var id int64
	row := s.db.QueryRow(`
		SELECT pl.id
		FROM pipelines pl
		JOIN projects p ON p.id = pl.project_id
		WHERE p.name = ? AND pl.pipeline_id = ?
	`, projectName, pipelineID)
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return PersistedPipeline{}, fmt.Errorf("pipeline not found")
		}
		return PersistedPipeline{}, fmt.Errorf("find pipeline: %w", err)
	}
	return s.GetPipelineByDBID(id)
}

func (s *Store) listPipelineJobs(pipelineDBID int64) ([]PersistedPipelineJob, error) {
	rows, err := s.db.Query(`
		SELECT job_id, position, runs_on_json, timeout_seconds, artifacts_json, matrix_json, steps_json
		FROM pipeline_jobs
		WHERE pipeline_id = ?
		ORDER BY position
	`, pipelineDBID)
	if err != nil {
		return nil, fmt.Errorf("list pipeline jobs: %w", err)
	}
	defer rows.Close()

	jobs := []PersistedPipelineJob{}
	for rows.Next() {
		var j PersistedPipelineJob
		var runsOnJSON, artifactsJSON, matrixJSON, stepsJSON string
		if err := rows.Scan(&j.ID, &j.Position, &runsOnJSON, &j.TimeoutSeconds, &artifactsJSON, &matrixJSON, &stepsJSON); err != nil {
			return nil, fmt.Errorf("scan pipeline job: %w", err)
		}
		_ = json.Unmarshal([]byte(runsOnJSON), &j.RunsOn)
		_ = json.Unmarshal([]byte(artifactsJSON), &j.Artifacts)
		_ = json.Unmarshal([]byte(matrixJSON), &j.MatrixInclude)
		_ = json.Unmarshal([]byte(stepsJSON), &j.Steps)
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline jobs: %w", err)
	}
	return jobs, nil
}

func (s *Store) CreateJob(req protocol.CreateJobRequest) (protocol.Job, error) {
	if strings.TrimSpace(req.Script) == "" {
		return protocol.Job{}, fmt.Errorf("script is required")
	}
	if req.TimeoutSeconds < 0 {
		return protocol.Job{}, fmt.Errorf("timeout_seconds must be >= 0")
	}

	now := time.Now().UTC()
	jobID := fmt.Sprintf("job-%d", now.UnixNano())

	requiredJSON, _ := json.Marshal(req.RequiredCapabilities)
	artifactGlobsJSON, _ := json.Marshal(req.ArtifactGlobs)
	metadataJSON, _ := json.Marshal(req.Metadata)

	var sourceRepo, sourceRef string
	if req.Source != nil {
		sourceRepo = req.Source.Repo
		sourceRef = req.Source.Ref
	}

	if _, err := s.db.Exec(`
		INSERT INTO jobs (id, script, required_capabilities_json, timeout_seconds, artifact_globs_json, source_repo, source_ref, metadata_json, status, created_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, jobID, req.Script, string(requiredJSON), req.TimeoutSeconds, string(artifactGlobsJSON), sourceRepo, sourceRef, string(metadataJSON), "queued", now.Format(time.RFC3339Nano)); err != nil {
		return protocol.Job{}, fmt.Errorf("insert job: %w", err)
	}

	return protocol.Job{
		ID:                   jobID,
		Script:               req.Script,
		RequiredCapabilities: cloneMap(req.RequiredCapabilities),
		TimeoutSeconds:       req.TimeoutSeconds,
		ArtifactGlobs:        append([]string(nil), req.ArtifactGlobs...),
		Source:               cloneSource(req.Source),
		Metadata:             cloneMap(req.Metadata),
		Status:               "queued",
		CreatedUTC:           now,
	}, nil
}

func (s *Store) ListJobs() ([]protocol.Job, error) {
	rows, err := s.db.Query(`
		SELECT id, script, required_capabilities_json, timeout_seconds, artifact_globs_json, source_repo, source_ref, metadata_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, output_text
		FROM jobs
		ORDER BY created_utc DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := []protocol.Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}
	return jobs, nil
}

func (s *Store) GetJob(id string) (protocol.Job, error) {
	row := s.db.QueryRow(`
		SELECT id, script, required_capabilities_json, timeout_seconds, artifact_globs_json, source_repo, source_ref, metadata_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, output_text
		FROM jobs WHERE id = ?
	`, id)
	job, err := scanJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return protocol.Job{}, fmt.Errorf("job not found")
		}
		return protocol.Job{}, err
	}
	return job, nil
}

func (s *Store) LeaseJob(agentID string, agentCaps map[string]string) (*protocol.Job, error) {
	jobs, err := s.ListQueuedJobs()
	if err != nil {
		return nil, err
	}

	for _, job := range jobs {
		if !capabilitiesMatch(agentCaps, job.RequiredCapabilities) {
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.Exec(`
			UPDATE jobs SET status = 'leased', leased_by_agent_id = ?, leased_utc = ?
			WHERE id = ? AND status = 'queued'
		`, agentID, now, job.ID)
		if err != nil {
			return nil, fmt.Errorf("lease job: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			continue
		}

		leased, err := s.GetJob(job.ID)
		if err != nil {
			return nil, err
		}
		return &leased, nil
	}

	return nil, nil
}

func (s *Store) ListQueuedJobs() ([]protocol.Job, error) {
	rows, err := s.db.Query(`
		SELECT id, script, required_capabilities_json, timeout_seconds, artifact_globs_json, source_repo, source_ref, metadata_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, output_text
		FROM jobs WHERE status = 'queued'
		ORDER BY created_utc ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list queued jobs: %w", err)
	}
	defer rows.Close()

	jobs := []protocol.Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queued jobs: %w", err)
	}
	return jobs, nil
}

func (s *Store) UpdateJobStatus(jobID string, req protocol.JobStatusUpdateRequest) (protocol.Job, error) {
	job, err := s.GetJob(jobID)
	if err != nil {
		return protocol.Job{}, err
	}

	if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
		return protocol.Job{}, fmt.Errorf("job is leased by another agent")
	}

	now := req.TimestampUTC
	if now.IsZero() {
		now = time.Now().UTC()
	}

	status := req.Status
	started := nullableTime(job.StartedUTC)
	finished := nullableTime(job.FinishedUTC)
	errorText := req.Error
	output := req.Output
	exitCode := nullableInt(req.ExitCode)

	if status == "running" && !job.StartedUTC.IsZero() {
		started = nullableTime(job.StartedUTC)
	}
	if status == "running" && job.StartedUTC.IsZero() {
		started = sql.NullString{String: now.Format(time.RFC3339Nano), Valid: true}
	}

	if status == "succeeded" || status == "failed" {
		if job.StartedUTC.IsZero() {
			started = sql.NullString{String: now.Format(time.RFC3339Nano), Valid: true}
		}
		finished = sql.NullString{String: now.Format(time.RFC3339Nano), Valid: true}
		if status == "succeeded" {
			errorText = ""
		}
	}

	if _, err := s.db.Exec(`
		UPDATE jobs
		SET status = ?, started_utc = ?, finished_utc = ?, exit_code = ?, error_text = ?, output_text = ?
		WHERE id = ?
	`, status, nullStringValue(started), nullStringValue(finished), nullIntValue(exitCode), errorText, output, jobID); err != nil {
		return protocol.Job{}, fmt.Errorf("update job status: %w", err)
	}

	return s.GetJob(jobID)
}

func (s *Store) DeleteQueuedJob(jobID string) error {
	res, err := s.db.Exec(`DELETE FROM jobs WHERE id = ? AND status = 'queued'`, jobID)
	if err != nil {
		return fmt.Errorf("delete queued job: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_, getErr := s.GetJob(jobID)
		if getErr != nil {
			return fmt.Errorf("job not found")
		}
		return fmt.Errorf("job is not queued")
	}
	return nil
}

func (s *Store) ClearQueuedJobs() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM jobs WHERE status = 'queued'`)
	if err != nil {
		return 0, fmt.Errorf("clear queued jobs: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (s *Store) FlushJobHistory() (int64, error) {
	res, err := s.db.Exec(`
		DELETE FROM jobs
		WHERE status NOT IN ('queued', 'leased', 'running')
	`)
	if err != nil {
		return 0, fmt.Errorf("flush job history: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (s *Store) SaveJobArtifacts(jobID string, artifacts []protocol.JobArtifact) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM job_artifacts WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("clear job artifacts: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, a := range artifacts {
		if _, err := tx.Exec(`
			INSERT INTO job_artifacts (job_id, path, stored_rel, size_bytes, created_utc)
			VALUES (?, ?, ?, ?, ?)
		`, jobID, a.Path, a.URL, a.SizeBytes, now); err != nil {
			return fmt.Errorf("insert artifact: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *Store) ListJobArtifacts(jobID string) ([]protocol.JobArtifact, error) {
	rows, err := s.db.Query(`
		SELECT id, job_id, path, stored_rel, size_bytes
		FROM job_artifacts
		WHERE job_id = ?
		ORDER BY id
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()

	artifacts := []protocol.JobArtifact{}
	for rows.Next() {
		var a protocol.JobArtifact
		if err := rows.Scan(&a.ID, &a.JobID, &a.Path, &a.URL, &a.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		artifacts = append(artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifacts: %w", err)
	}
	return artifacts, nil
}

func scanJob(scanner interface{ Scan(dest ...any) error }) (protocol.Job, error) {
	var (
		job                                           protocol.Job
		requiredJSON, artifactGlobsJSON, metadataJSON string
		sourceRepo, sourceRef                         sql.NullString
		createdUTC                                    string
		startedUTC, finishedUTC                       sql.NullString
		leasedByAgentID, leasedUTC                    sql.NullString
		exitCode                                      sql.NullInt64
		errorText, outputText                         sql.NullString
	)

	if err := scanner.Scan(
		&job.ID, &job.Script, &requiredJSON, &job.TimeoutSeconds, &artifactGlobsJSON, &sourceRepo, &sourceRef, &metadataJSON,
		&job.Status, &createdUTC, &startedUTC, &finishedUTC, &leasedByAgentID, &leasedUTC, &exitCode, &errorText, &outputText,
	); err != nil {
		return protocol.Job{}, err
	}

	_ = json.Unmarshal([]byte(requiredJSON), &job.RequiredCapabilities)
	_ = json.Unmarshal([]byte(artifactGlobsJSON), &job.ArtifactGlobs)
	_ = json.Unmarshal([]byte(metadataJSON), &job.Metadata)

	if sourceRepo.Valid && sourceRepo.String != "" {
		job.Source = &protocol.SourceSpec{Repo: sourceRepo.String, Ref: sourceRef.String}
	}
	if createdUTC != "" {
		if t, err := time.Parse(time.RFC3339Nano, createdUTC); err == nil {
			job.CreatedUTC = t
		}
	}
	if startedUTC.Valid {
		if t, err := time.Parse(time.RFC3339Nano, startedUTC.String); err == nil {
			job.StartedUTC = t
		}
	}
	if finishedUTC.Valid {
		if t, err := time.Parse(time.RFC3339Nano, finishedUTC.String); err == nil {
			job.FinishedUTC = t
		}
	}
	if leasedByAgentID.Valid {
		job.LeasedByAgentID = leasedByAgentID.String
	}
	if leasedUTC.Valid {
		if t, err := time.Parse(time.RFC3339Nano, leasedUTC.String); err == nil {
			job.LeasedUTC = t
		}
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		job.ExitCode = &v
	}
	if errorText.Valid {
		job.Error = errorText.String
	}
	if outputText.Valid {
		job.Output = outputText.String
	}

	return job, nil
}

func capabilitiesMatch(agentCapabilities, requiredCapabilities map[string]string) bool {
	if len(requiredCapabilities) == 0 {
		return true
	}
	for k, requiredValue := range requiredCapabilities {
		if agentCapabilities[k] != requiredValue {
			return false
		}
	}
	return true
}

func nullableTime(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339Nano), Valid: true}
}

func nullableInt(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func nullStringValue(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func nullIntValue(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSource(in *protocol.SourceSpec) *protocol.SourceSpec {
	if in == nil {
		return nil
	}
	return &protocol.SourceSpec{Repo: in.Repo, Ref: in.Ref}
}

func (p PersistedPipeline) SortedJobs() []PersistedPipelineJob {
	jobs := make([]PersistedPipelineJob, len(p.Jobs))
	copy(jobs, p.Jobs)
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Position < jobs[j].Position })
	return jobs
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
