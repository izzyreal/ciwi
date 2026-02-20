package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

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
	cacheStatsJSON := "[]"
	if len(job.CacheStats) > 0 {
		if raw, marshalErr := json.Marshal(job.CacheStats); marshalErr == nil {
			cacheStatsJSON = string(raw)
		}
	}
	if len(req.CacheStats) > 0 {
		if raw, marshalErr := json.Marshal(req.CacheStats); marshalErr == nil {
			cacheStatsJSON = string(raw)
		}
	}
	runtimeCapsJSON := "{}"
	if len(job.RuntimeCapabilities) > 0 {
		if raw, marshalErr := json.Marshal(job.RuntimeCapabilities); marshalErr == nil {
			runtimeCapsJSON = string(raw)
		}
	}
	if len(req.RuntimeCapabilities) > 0 {
		if raw, marshalErr := json.Marshal(req.RuntimeCapabilities); marshalErr == nil {
			runtimeCapsJSON = string(raw)
		}
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
	args := []any{status, nullStringValue(started), nullStringValue(finished), nullIntValue(exitCode), errorText, output, cacheStatsJSON, runtimeCapsJSON, currentStep}
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
			SET status = ?, started_utc = ?, finished_utc = ?, exit_code = ?, error_text = ?, output_text = ?, cache_stats_json = ?, runtime_capabilities_json = ?, current_step_text = ?
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

func (s *Store) MergeJobExecutionEnv(jobID string, patch map[string]string) (map[string]string, error) {
	return s.mergeJobExecutionStringMap(jobID, patch, "env_json", "update env", func(job protocol.JobExecution) map[string]string {
		return job.Env
	})
}

func (s *Store) MergeJobExecutionMetadata(jobID string, patch map[string]string) (map[string]string, error) {
	return s.mergeJobExecutionStringMap(jobID, patch, "metadata_json", "update metadata", func(job protocol.JobExecution) map[string]string {
		return job.Metadata
	})
}

func (s *Store) mergeJobExecutionStringMap(jobID string, patch map[string]string, column string, updateErrPrefix string, currentMap func(protocol.JobExecution) map[string]string) (map[string]string, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job id is required")
	}
	if len(patch) == 0 {
		job, err := s.GetJobExecution(jobID)
		if err != nil {
			return nil, err
		}
		return cloneMap(currentMap(job)), nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `SELECT ` + column + ` FROM job_executions WHERE id = ?`
	var raw string
	if err := tx.QueryRow(query, jobID).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("job not found")
		}
		return nil, fmt.Errorf("read %s: %w", strings.TrimSuffix(column, "_json"), err)
	}

	values := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &values)
	if values == nil {
		values = map[string]string{}
	}
	for k, v := range patch {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if strings.TrimSpace(v) == "" {
			delete(values, key)
			continue
		}
		values[key] = v
	}
	updated, _ := json.Marshal(values)
	updateSQL := `UPDATE job_executions SET ` + column + ` = ? WHERE id = ?`
	if _, err := tx.Exec(updateSQL, string(updated), jobID); err != nil {
		return nil, fmt.Errorf("%s: %w", updateErrPrefix, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return cloneMap(values), nil
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

func (s *Store) FlushJobExecutionHistoryByAgent(agentID string) ([]string, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	jobs, err := s.ListJobExecutions()
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	candidates := make([]string, 0)
	for _, job := range jobs {
		status := protocol.NormalizeJobExecutionStatus(job.Status)
		if status == protocol.JobExecutionStatusQueued || status == protocol.JobExecutionStatusLeased || status == protocol.JobExecutionStatusRunning {
			continue
		}
		leasedBy := strings.TrimSpace(job.LeasedByAgentID)
		adhocAgent := strings.TrimSpace(job.Metadata["adhoc_agent_id"])
		if leasedBy == agentID || adhocAgent == agentID {
			candidates = append(candidates, job.ID)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	var deleted []string
	if err := retrySQLiteBusy(func() error {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
		attemptDeleted := make([]string, 0, len(candidates))
		for _, id := range candidates {
			res, err := tx.Exec(`
				DELETE FROM job_executions
				WHERE id = ? AND status NOT IN (?, ?, ?)
			`, id, protocol.JobExecutionStatusQueued, protocol.JobExecutionStatusLeased, protocol.JobExecutionStatusRunning)
			if err != nil {
				return fmt.Errorf("delete job %q: %w", id, err)
			}
			if affected, _ := res.RowsAffected(); affected > 0 {
				attemptDeleted = append(attemptDeleted, id)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		committed = true
		deleted = attemptDeleted
		return nil
	}); err != nil {
		return nil, fmt.Errorf("flush job history by agent: %w", err)
	}
	return deleted, nil
}

func (s *Store) RequeueStaleLeasedJobExecutions(now time.Time, maxAge time.Duration) (int64, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if maxAge <= 0 {
		return 0, fmt.Errorf("maxAge must be > 0")
	}
	jobs, err := s.ListJobExecutions()
	if err != nil {
		return 0, fmt.Errorf("list jobs: %w", err)
	}
	var requeued int64
	for _, job := range jobs {
		if protocol.NormalizeJobExecutionStatus(job.Status) != protocol.JobExecutionStatusLeased {
			continue
		}
		if !job.LeasedUTC.IsZero() && now.Sub(job.LeasedUTC) < maxAge {
			continue
		}
		var res sql.Result
		if err := retrySQLiteBusy(func() error {
			var execErr error
			res, execErr = s.db.Exec(`
				UPDATE job_executions
				SET status = ?, leased_by_agent_id = '', leased_utc = NULL, current_step_text = ''
				WHERE id = ? AND status = ?
			`, protocol.JobExecutionStatusQueued, job.ID, protocol.JobExecutionStatusLeased)
			return execErr
		}); err != nil {
			return requeued, fmt.Errorf("requeue stale leased job %s: %w", job.ID, err)
		}
		affected, _ := res.RowsAffected()
		requeued += affected
	}
	return requeued, nil
}

func (s *Store) FailTimedOutRunningJobExecutions(now time.Time, grace time.Duration, reason string) (int64, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if grace < 0 {
		grace = 0
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "job timed out while running (server maintenance)"
	}
	jobs, err := s.ListJobExecutions()
	if err != nil {
		return 0, fmt.Errorf("list jobs: %w", err)
	}
	var failed int64
	for _, job := range jobs {
		if protocol.NormalizeJobExecutionStatus(job.Status) != protocol.JobExecutionStatusRunning {
			continue
		}
		if job.TimeoutSeconds <= 0 {
			continue
		}
		started := job.StartedUTC
		if started.IsZero() {
			continue
		}
		deadline := started.Add(time.Duration(job.TimeoutSeconds)*time.Second + grace)
		if now.Before(deadline) {
			continue
		}
		marker := "[control] " + reason
		var res sql.Result
		if err := retrySQLiteBusy(func() error {
			var execErr error
			res, execErr = s.db.Exec(`
				UPDATE job_executions
				SET status = ?,
				    finished_utc = ?,
				    error_text = ?,
				    current_step_text = '',
				    output_text = CASE
				      WHEN TRIM(COALESCE(output_text, '')) = '' THEN ?
				      ELSE output_text || CHAR(10) || ?
				    END
				WHERE id = ? AND status = ?
			`, protocol.JobExecutionStatusFailed, now.Format(time.RFC3339Nano), reason, marker, marker, job.ID, protocol.JobExecutionStatusRunning)
			return execErr
		}); err != nil {
			return failed, fmt.Errorf("fail timed-out running job %s: %w", job.ID, err)
		}
		affected, _ := res.RowsAffected()
		failed += affected
	}
	return failed, nil
}
