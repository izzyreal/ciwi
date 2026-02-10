package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

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
