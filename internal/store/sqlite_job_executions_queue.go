package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *Store) CreateJobExecution(req protocol.CreateJobExecutionRequest) (protocol.JobExecution, error) {
	if strings.TrimSpace(req.Script) == "" {
		return protocol.JobExecution{}, fmt.Errorf("script is required")
	}
	if req.TimeoutSeconds < 0 {
		return protocol.JobExecution{}, fmt.Errorf("timeout_seconds must be >= 0")
	}

	now := time.Now().UTC()
	jobID := fmt.Sprintf("job-%d", now.UnixNano())

	requiredJSON, _ := json.Marshal(req.RequiredCapabilities)
	envJSON, _ := json.Marshal(req.Env)
	artifactGlobsJSON, _ := json.Marshal(req.ArtifactGlobs)
	cachesJSON, _ := json.Marshal(req.Caches)
	metadataJSON, _ := json.Marshal(req.Metadata)
	stepPlanJSON, _ := json.Marshal(req.StepPlan)

	var sourceRepo, sourceRef string
	if req.Source != nil {
		sourceRepo = req.Source.Repo
		sourceRef = req.Source.Ref
	}

	if _, err := s.db.Exec(`
		INSERT INTO job_executions (id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json, step_plan_json, status, created_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, jobID, req.Script, string(envJSON), string(requiredJSON), req.TimeoutSeconds, string(artifactGlobsJSON), string(cachesJSON), sourceRepo, sourceRef, string(metadataJSON), string(stepPlanJSON), protocol.JobExecutionStatusQueued, now.Format(time.RFC3339Nano)); err != nil {
		return protocol.JobExecution{}, fmt.Errorf("insert job: %w", err)
	}

	return protocol.JobExecution{
		ID:                   jobID,
		Script:               req.Script,
		Env:                  cloneMap(req.Env),
		RequiredCapabilities: cloneMap(req.RequiredCapabilities),
		TimeoutSeconds:       req.TimeoutSeconds,
		ArtifactGlobs:        append([]string(nil), req.ArtifactGlobs...),
		Caches:               cloneJobCaches(req.Caches),
		Source:               cloneSource(req.Source),
		Metadata:             cloneMap(req.Metadata),
		StepPlan:             cloneJobStepPlan(req.StepPlan),
		Status:               protocol.JobExecutionStatusQueued,
		CreatedUTC:           now,
	}, nil
}

func (s *Store) ListJobExecutions() ([]protocol.JobExecution, error) {
	rows, err := s.db.Query(`
		SELECT id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json, step_plan_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, '' AS output_text, cache_stats_json, runtime_capabilities_json, current_step_text
		FROM job_executions
		ORDER BY created_utc DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := []protocol.JobExecution{}
	for rows.Next() {
		job, err := scanJobExecution(rows)
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

func (s *Store) GetJobExecution(id string) (protocol.JobExecution, error) {
	row := s.db.QueryRow(`
		SELECT id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json, step_plan_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, output_text, cache_stats_json, runtime_capabilities_json, current_step_text
		FROM job_executions WHERE id = ?
	`, id)
	job, err := scanJobExecution(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return protocol.JobExecution{}, fmt.Errorf("job not found")
		}
		return protocol.JobExecution{}, err
	}
	return job, nil
}

func (s *Store) LeaseJobExecution(agentID string, agentCaps map[string]string) (*protocol.JobExecution, error) {
	jobs, err := s.ListQueuedJobExecutions()
	if err != nil {
		return nil, err
	}

	for _, job := range jobs {
		if strings.TrimSpace(job.Metadata["chain_blocked"]) == "1" {
			continue
		}
		if strings.TrimSpace(job.Metadata["needs_blocked"]) == "1" {
			continue
		}
		if !capabilitiesMatch(agentCaps, job.RequiredCapabilities) {
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.Exec(`
			UPDATE job_executions SET status = ?, leased_by_agent_id = ?, leased_utc = ?
			WHERE id = ? AND status = ?
		`, protocol.JobExecutionStatusLeased, agentID, now, job.ID, protocol.JobExecutionStatusQueued)
		if err != nil {
			return nil, fmt.Errorf("lease job: %w", err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			continue
		}

		leased, err := s.GetJobExecution(job.ID)
		if err != nil {
			return nil, err
		}
		return &leased, nil
	}

	return nil, nil
}

func (s *Store) AgentHasActiveJobExecution(agentID string) (bool, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false, fmt.Errorf("agent id is required")
	}
	var count int64
	if err := s.db.QueryRow(`
		SELECT COUNT(1)
		FROM job_executions
		WHERE leased_by_agent_id = ?
		  AND status IN (?, ?)
	`, agentID, protocol.JobExecutionStatusLeased, protocol.JobExecutionStatusRunning).Scan(&count); err != nil {
		return false, fmt.Errorf("check active jobs for agent: %w", err)
	}
	return count > 0, nil
}

func (s *Store) ListQueuedJobExecutions() ([]protocol.JobExecution, error) {
	rows, err := s.db.Query(`
		SELECT id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json, step_plan_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, '' AS output_text, cache_stats_json, runtime_capabilities_json, current_step_text
		FROM job_executions WHERE status = ?
		ORDER BY created_utc ASC, id ASC
	`, protocol.JobExecutionStatusQueued)
	if err != nil {
		return nil, fmt.Errorf("list queued jobs: %w", err)
	}
	defer rows.Close()

	jobs := []protocol.JobExecution{}
	for rows.Next() {
		job, err := scanJobExecution(rows)
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
