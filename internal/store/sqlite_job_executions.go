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

	var sourceRepo, sourceRef string
	if req.Source != nil {
		sourceRepo = req.Source.Repo
		sourceRef = req.Source.Ref
	}

	if _, err := s.db.Exec(`
		INSERT INTO job_executions (id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json, status, created_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, jobID, req.Script, string(envJSON), string(requiredJSON), req.TimeoutSeconds, string(artifactGlobsJSON), string(cachesJSON), sourceRepo, sourceRef, string(metadataJSON), protocol.JobExecutionStatusQueued, now.Format(time.RFC3339Nano)); err != nil {
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
		Status:               protocol.JobExecutionStatusQueued,
		CreatedUTC:           now,
	}, nil
}

func (s *Store) ListJobExecutions() ([]protocol.JobExecution, error) {
	rows, err := s.db.Query(`
		SELECT id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, output_text, current_step_text
		FROM job_executions
		ORDER BY created_utc DESC
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
		SELECT id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, output_text, current_step_text
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
		SELECT id, script, env_json, required_capabilities_json, timeout_seconds, artifact_globs_json, caches_json, source_repo, source_ref, metadata_json,
		       status, created_utc, started_utc, finished_utc, leased_by_agent_id, leased_utc, exit_code, error_text, output_text, current_step_text
		FROM job_executions WHERE status = ?
		ORDER BY created_utc ASC
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

func (s *Store) UpdateJobExecutionStatus(jobID string, req protocol.JobExecutionStatusUpdateRequest) (protocol.JobExecution, error) {
	job, err := s.GetJobExecution(jobID)
	if err != nil {
		return protocol.JobExecution{}, err
	}

	reqStatus := protocol.NormalizeJobExecutionStatus(req.Status)

	// Terminal status is sticky. Ignore late running updates (for example
	// periodic log-stream updates racing with final succeeded/failed update).
	if protocol.IsTerminalJobExecutionStatus(job.Status) {
		if reqStatus == protocol.JobExecutionStatusRunning {
			return job, nil
		}
		if protocol.IsTerminalJobExecutionStatus(reqStatus) && reqStatus != protocol.NormalizeJobExecutionStatus(job.Status) {
			return job, nil
		}
	}

	if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
		return protocol.JobExecution{}, fmt.Errorf("job is leased by another agent")
	}

	now := req.TimestampUTC
	if now.IsZero() {
		now = time.Now().UTC()
	}

	status := reqStatus
	started := nullableTime(job.StartedUTC)
	finished := nullableTime(job.FinishedUTC)
	errorText := req.Error
	output := req.Output
	exitCode := nullableInt(req.ExitCode)
	currentStep := strings.TrimSpace(req.CurrentStep)
	if currentStep == "" {
		currentStep = strings.TrimSpace(job.CurrentStep)
	}
	// Treat status updates as partial patches: when output is omitted by the caller,
	// keep the latest persisted log snapshot instead of clearing it.
	if output == "" {
		output = job.Output
	}

	if status == protocol.JobExecutionStatusRunning && !job.StartedUTC.IsZero() {
		started = nullableTime(job.StartedUTC)
	}
	if status == protocol.JobExecutionStatusRunning && job.StartedUTC.IsZero() {
		started = sql.NullString{String: now.Format(time.RFC3339Nano), Valid: true}
	}

	if protocol.IsTerminalJobExecutionStatus(status) {
		if job.StartedUTC.IsZero() {
			started = sql.NullString{String: now.Format(time.RFC3339Nano), Valid: true}
		}
		finished = sql.NullString{String: now.Format(time.RFC3339Nano), Valid: true}
		if status == protocol.JobExecutionStatusSucceeded {
			errorText = ""
		}
		currentStep = ""
	}

	where := "id = ?"
	args := []any{status, nullStringValue(started), nullStringValue(finished), nullIntValue(exitCode), errorText, output, currentStep}
	if status == protocol.JobExecutionStatusRunning {
		// Never allow a running heartbeat/log-stream update to overwrite a terminal state.
		where = "id = ? AND status NOT IN (?, ?)"
		args = append(args, jobID, protocol.JobExecutionStatusSucceeded, protocol.JobExecutionStatusFailed)
	} else if protocol.IsTerminalJobExecutionStatus(status) {
		// First terminal status wins under races; later terminal writes become no-ops.
		where = "id = ? AND status NOT IN (?, ?)"
		args = append(args, jobID, protocol.JobExecutionStatusSucceeded, protocol.JobExecutionStatusFailed)
	} else {
		args = append(args, jobID)
	}

	var res sql.Result
	if err := retrySQLiteBusy(func() error {
		var execErr error
		res, execErr = s.db.Exec(`
			UPDATE job_executions
			SET status = ?, started_utc = ?, finished_utc = ?, exit_code = ?, error_text = ?, output_text = ?, current_step_text = ?
			WHERE `+where+`
		`, args...)
		return execErr
	}); err != nil {
		return protocol.JobExecution{}, fmt.Errorf("update job status: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		// Another concurrent writer won (typically terminal status); return latest state.
		return s.GetJobExecution(jobID)
	}

	return s.GetJobExecution(jobID)
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}

func retrySQLiteBusy(fn func() error) error {
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if !isSQLiteBusyError(err) || attempt >= 2 {
			return err
		}
		time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
	}
}

func (s *Store) MergeJobExecutionMetadata(jobID string, patch map[string]string) (map[string]string, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job id is required")
	}
	if len(patch) == 0 {
		job, err := s.GetJobExecution(jobID)
		if err != nil {
			return nil, err
		}
		return cloneMap(job.Metadata), nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var raw string
	if err := tx.QueryRow(`SELECT metadata_json FROM job_executions WHERE id = ?`, jobID).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("job not found")
		}
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	meta := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &meta)
	if meta == nil {
		meta = map[string]string{}
	}
	for k, v := range patch {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if strings.TrimSpace(v) == "" {
			delete(meta, key)
			continue
		}
		meta[key] = v
	}
	updated, _ := json.Marshal(meta)
	if _, err := tx.Exec(`UPDATE job_executions SET metadata_json = ? WHERE id = ?`, string(updated), jobID); err != nil {
		return nil, fmt.Errorf("update metadata: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return cloneMap(meta), nil
}

func (s *Store) DeleteQueuedJobExecution(jobID string) error {
	res, err := s.db.Exec(`DELETE FROM job_executions WHERE id = ? AND status IN (?, ?)`, jobID, protocol.JobExecutionStatusQueued, protocol.JobExecutionStatusLeased)
	if err != nil {
		return fmt.Errorf("delete queued job: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_, getErr := s.GetJobExecution(jobID)
		if getErr != nil {
			return fmt.Errorf("job not found")
		}
		return fmt.Errorf("job is not pending")
	}
	return nil
}

func (s *Store) ClearQueuedJobExecutions() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM job_executions WHERE status IN (?, ?)`, protocol.JobExecutionStatusQueued, protocol.JobExecutionStatusLeased)
	if err != nil {
		return 0, fmt.Errorf("clear queued jobs: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (s *Store) FlushJobExecutionHistory() (int64, error) {
	res, err := s.db.Exec(`
		DELETE FROM job_executions
		WHERE status NOT IN (?, ?, ?)
	`, protocol.JobExecutionStatusQueued, protocol.JobExecutionStatusLeased, protocol.JobExecutionStatusRunning)
	if err != nil {
		return 0, fmt.Errorf("flush job history: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func (s *Store) SaveJobExecutionArtifacts(jobID string, artifacts []protocol.JobExecutionArtifact) error {
	return retrySQLiteBusy(func() error {
		return s.saveJobExecutionArtifactsOnce(jobID, artifacts)
	})
}

func (s *Store) saveJobExecutionArtifactsOnce(jobID string, artifacts []protocol.JobExecutionArtifact) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM job_execution_artifacts WHERE job_execution_id = ?`, jobID); err != nil {
		return fmt.Errorf("clear job artifacts: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, a := range artifacts {
		if _, err := tx.Exec(`
			INSERT INTO job_execution_artifacts (job_execution_id, path, stored_rel, size_bytes, created_utc)
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

func (s *Store) ListJobExecutionArtifacts(jobID string) ([]protocol.JobExecutionArtifact, error) {
	rows, err := s.db.Query(`
		SELECT id, job_execution_id, path, stored_rel, size_bytes
		FROM job_execution_artifacts
		WHERE job_execution_id = ?
		ORDER BY id
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()

	artifacts := []protocol.JobExecutionArtifact{}
	for rows.Next() {
		var a protocol.JobExecutionArtifact
		if err := rows.Scan(&a.ID, &a.JobExecutionID, &a.Path, &a.URL, &a.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		artifacts = append(artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifacts: %w", err)
	}
	return artifacts, nil
}

func (s *Store) SaveJobExecutionTestReport(jobID string, report protocol.JobExecutionTestReport) error {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal test report: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := retrySQLiteBusy(func() error {
		_, err := s.db.Exec(`
			INSERT INTO job_execution_test_reports (job_execution_id, report_json, created_utc)
			VALUES (?, ?, ?)
			ON CONFLICT(job_execution_id) DO UPDATE SET report_json=excluded.report_json, created_utc=excluded.created_utc
		`, jobID, string(reportJSON), now)
		return err
	}); err != nil {
		return fmt.Errorf("save test report: %w", err)
	}
	return nil
}

func (s *Store) GetJobExecutionTestReport(jobID string) (protocol.JobExecutionTestReport, bool, error) {
	var reportJSON string
	row := s.db.QueryRow(`SELECT report_json FROM job_execution_test_reports WHERE job_execution_id = ?`, jobID)
	if err := row.Scan(&reportJSON); err != nil {
		if err == sql.ErrNoRows {
			return protocol.JobExecutionTestReport{}, false, nil
		}
		return protocol.JobExecutionTestReport{}, false, fmt.Errorf("get test report: %w", err)
	}

	var report protocol.JobExecutionTestReport
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		return protocol.JobExecutionTestReport{}, false, fmt.Errorf("decode test report: %w", err)
	}
	return report, true, nil
}
