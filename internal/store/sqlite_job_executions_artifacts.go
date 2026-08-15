package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

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
			INSERT INTO job_execution_test_reports (job_execution_id, report_json, total_count, passed_count, failed_count, skipped_count, created_utc)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(job_execution_id) DO UPDATE SET
				report_json=excluded.report_json,
				total_count=excluded.total_count,
				passed_count=excluded.passed_count,
				failed_count=excluded.failed_count,
				skipped_count=excluded.skipped_count,
				created_utc=excluded.created_utc
		`, jobID, string(reportJSON), report.Total, report.Passed, report.Failed, report.Skipped, now)
		return err
	}); err != nil {
		return fmt.Errorf("save test report: %w", err)
	}
	return nil
}

func (s *Store) ListJobExecutionTestSummaries(ctx context.Context, jobIDs []string) (map[string]protocol.JobExecutionTestSummary, error) {
	result := make(map[string]protocol.JobExecutionTestSummary)
	unique := make([]string, 0, len(jobIDs))
	seen := make(map[string]struct{}, len(jobIDs))
	for _, jobID := range jobIDs {
		jobID = strings.TrimSpace(jobID)
		if jobID == "" {
			continue
		}
		if _, exists := seen[jobID]; exists {
			continue
		}
		seen[jobID] = struct{}{}
		unique = append(unique, jobID)
	}
	const chunkSize = 250
	for start := 0; start < len(unique); start += chunkSize {
		end := min(start+chunkSize, len(unique))
		chunk := unique[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		arguments := make([]any, len(chunk))
		for i, jobID := range chunk {
			arguments[i] = jobID
		}
		rows, err := s.db.QueryContext(ctx, `
			SELECT job_execution_id, total_count, passed_count, failed_count, skipped_count
			FROM job_execution_test_reports
			WHERE job_execution_id IN (`+placeholders+`)
		`, arguments...)
		if err != nil {
			return nil, fmt.Errorf("list test summaries: %w", err)
		}
		for rows.Next() {
			var jobID string
			var summary protocol.JobExecutionTestSummary
			if err := rows.Scan(&jobID, &summary.Total, &summary.Passed, &summary.Failed, &summary.Skipped); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan test summary: %w", err)
			}
			result[jobID] = summary
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate test summaries: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close test summaries: %w", err)
		}
	}
	return result, nil
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

func (s *Store) AppendJobExecutionEvents(jobID string, events []protocol.JobExecutionEvent) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("job id is required")
	}
	if len(events) == 0 {
		return nil
	}
	return retrySQLiteBusy(func() error {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		var interactiveLogVersion int
		if err := tx.QueryRow(`SELECT interactive_log_version FROM job_executions WHERE id = ?`, jobID).Scan(&interactiveLogVersion); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("job not found")
			}
			return fmt.Errorf("read interactive log version: %w", err)
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, event := range events {
			eventType := strings.TrimSpace(event.Type)
			if eventType == "" {
				continue
			}
			ts := event.TimestampUTC
			if ts.IsZero() {
				ts = time.Now().UTC()
			}
			payload := map[string]any{}
			if event.Step != nil {
				payload["step"] = event.Step
			}
			if event.Phase != nil {
				payload["phase"] = event.Phase
			}
			if strings.TrimSpace(event.Message) != "" {
				payload["message"] = event.Message
			}
			if event.Output != "" {
				payload["output"] = event.Output
			}
			if strings.TrimSpace(event.Error) != "" {
				payload["error"] = event.Error
			}
			if event.ExitCode != nil {
				payload["exit_code"] = *event.ExitCode
			}
			if event.DurationMS > 0 {
				payload["duration_ms"] = event.DurationMS
			}
			payloadJSON, _ := json.Marshal(payload)
			tsRaw := ts.UTC().Format(time.RFC3339Nano)
			result, err := tx.Exec(`
				INSERT INTO job_execution_events (job_execution_id, event_type, timestamp_utc, payload_json, created_utc)
				SELECT ?, ?, ?, ?, ?
				WHERE NOT EXISTS (
					SELECT 1 FROM job_execution_events
					WHERE job_execution_id = ? AND event_type = ? AND timestamp_utc = ? AND payload_json = ?
				)
			`, jobID, eventType, tsRaw, string(payloadJSON), now, jobID, eventType, tsRaw, string(payloadJSON))
			if err != nil {
				return fmt.Errorf("insert event: %w", err)
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("event rows affected: %w", err)
			}
			if inserted == 1 && interactiveLogVersion == domain.InteractiveJobLogVersion {
				eventID, err := result.LastInsertId()
				if err != nil {
					return fmt.Errorf("event id: %w", err)
				}
				if err := appendIndexedLogEvent(tx, jobID, eventID, event); err != nil {
					return err
				}
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		return nil
	})
}

func (s *Store) ListJobExecutionEvents(jobID string) ([]protocol.JobExecutionEvent, error) {
	return s.ListJobExecutionEventsAfter(jobID, 0)
}

// ListJobExecutionTimelineEvents excludes output and system-message payloads.
// Snapshot consumers can derive phase/step state without loading potentially
// very large logs on every queue/history invalidation.
func (s *Store) ListJobExecutionTimelineEvents(jobID string) ([]protocol.JobExecutionEvent, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job id is required")
	}
	rows, err := s.db.Query(`
		SELECT id, event_type, timestamp_utc, payload_json
		FROM job_execution_events
		WHERE job_execution_id = ? AND event_type IN (?, ?, ?, ?)
		ORDER BY id ASC
	`, jobID,
		protocol.JobExecutionEventTypePhaseStarted, protocol.JobExecutionEventTypePhaseFinished,
		protocol.JobExecutionEventTypeStepStarted, protocol.JobExecutionEventTypeStepFinished,
	)
	if err != nil {
		return nil, fmt.Errorf("list timeline events: %w", err)
	}
	defer rows.Close()
	out := []protocol.JobExecutionEvent{}
	for rows.Next() {
		var id int64
		var eventType, tsRaw, payloadRaw string
		if err := rows.Scan(&id, &eventType, &tsRaw, &payloadRaw); err != nil {
			return nil, fmt.Errorf("scan timeline event: %w", err)
		}
		out = append(out, decodeJobExecutionEvent(id, eventType, tsRaw, payloadRaw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timeline events: %w", err)
	}
	return out, nil
}

func (s *Store) ListJobExecutionEventsAfter(jobID string, afterID int64) ([]protocol.JobExecutionEvent, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job id is required")
	}
	if afterID < 0 {
		afterID = 0
	}
	rows, err := s.db.Query(`
		SELECT id, event_type, timestamp_utc, payload_json
		FROM job_execution_events
		WHERE job_execution_id = ? AND id > ?
		ORDER BY id ASC
	`, jobID, afterID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	out := []protocol.JobExecutionEvent{}
	for rows.Next() {
		var id int64
		var eventType, tsRaw, payloadRaw string
		if err := rows.Scan(&id, &eventType, &tsRaw, &payloadRaw); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, decodeJobExecutionEvent(id, eventType, tsRaw, payloadRaw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return out, nil
}

func (s *Store) ListJobExecutionEventsPageAfter(jobID string, afterID int64, limit int) ([]protocol.JobExecutionEvent, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job id is required")
	}
	if afterID < 0 {
		afterID = 0
	}
	if limit <= 0 || limit > 512 {
		limit = 128
	}
	rows, err := s.db.Query(`
		SELECT id, event_type, timestamp_utc, payload_json
		FROM job_execution_events
		WHERE job_execution_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, jobID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list event page: %w", err)
	}
	defer rows.Close()
	out := []protocol.JobExecutionEvent{}
	for rows.Next() {
		var id int64
		var eventType, tsRaw, payloadRaw string
		if err := rows.Scan(&id, &eventType, &tsRaw, &payloadRaw); err != nil {
			return nil, fmt.Errorf("scan event page: %w", err)
		}
		out = append(out, decodeJobExecutionEvent(id, eventType, tsRaw, payloadRaw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event page: %w", err)
	}
	return out, nil
}

func (s *Store) ListJobExecutionEventsForJobs(jobIDs []string, eventType string) (map[string][]protocol.JobExecutionEvent, error) {
	out := make(map[string][]protocol.JobExecutionEvent, len(jobIDs))
	if len(jobIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, 0, len(jobIDs))
	args := make([]any, 0, len(jobIDs)+1)
	for _, jobID := range jobIDs {
		jobID = strings.TrimSpace(jobID)
		if jobID == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, jobID)
	}
	if len(placeholders) == 0 {
		return out, nil
	}
	query := `
		SELECT id, job_execution_id, event_type, timestamp_utc, payload_json
		FROM job_execution_events
		WHERE job_execution_id IN (` + strings.Join(placeholders, ",") + `)`
	if strings.TrimSpace(eventType) != "" {
		query += ` AND event_type = ?`
		args = append(args, strings.TrimSpace(eventType))
	}
	query += ` ORDER BY job_execution_id ASC, id ASC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events for jobs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var jobID, rowType, tsRaw, payloadRaw string
		if err := rows.Scan(&id, &jobID, &rowType, &tsRaw, &payloadRaw); err != nil {
			return nil, fmt.Errorf("scan event for jobs: %w", err)
		}
		out[jobID] = append(out[jobID], decodeJobExecutionEvent(id, rowType, tsRaw, payloadRaw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events for jobs: %w", err)
	}
	return out, nil
}

func (s *Store) ListJobExecutionDurationEventsForJobs(jobIDs []string) (map[string][]protocol.JobExecutionEvent, error) {
	const batchSize = 400
	out := make(map[string][]protocol.JobExecutionEvent, len(jobIDs))
	uniqueIDs := make([]string, 0, len(jobIDs))
	seen := make(map[string]struct{}, len(jobIDs))
	for _, jobID := range jobIDs {
		jobID = strings.TrimSpace(jobID)
		if jobID == "" {
			continue
		}
		if _, ok := seen[jobID]; ok {
			continue
		}
		seen[jobID] = struct{}{}
		uniqueIDs = append(uniqueIDs, jobID)
	}
	for start := 0; start < len(uniqueIDs); start += batchSize {
		end := min(start+batchSize, len(uniqueIDs))
		placeholders := make([]string, end-start)
		args := make([]any, 0, end-start+2)
		for i, jobID := range uniqueIDs[start:end] {
			placeholders[i] = "?"
			args = append(args, jobID)
		}
		args = append(args, protocol.JobExecutionEventTypeStepFinished, protocol.JobExecutionEventTypePhaseFinished)
		rows, err := s.db.Query(`
			SELECT id, job_execution_id, event_type, timestamp_utc, payload_json
			FROM job_execution_events
			WHERE job_execution_id IN (`+strings.Join(placeholders, ",")+`)
			  AND event_type IN (?, ?)
			ORDER BY job_execution_id ASC, id ASC
		`, args...)
		if err != nil {
			return nil, fmt.Errorf("list duration events for jobs: %w", err)
		}
		for rows.Next() {
			var id int64
			var jobID, eventType, tsRaw, payloadRaw string
			if err := rows.Scan(&id, &jobID, &eventType, &tsRaw, &payloadRaw); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan duration event for jobs: %w", err)
			}
			out[jobID] = append(out[jobID], decodeJobExecutionEvent(id, eventType, tsRaw, payloadRaw))
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate duration events for jobs: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close duration events for jobs: %w", err)
		}
	}
	return out, nil
}

func decodeJobExecutionEvent(id int64, eventType, tsRaw, payloadRaw string) protocol.JobExecutionEvent {
	event := protocol.JobExecutionEvent{ID: id, Type: strings.TrimSpace(eventType)}
	if ts, err := time.Parse(time.RFC3339Nano, tsRaw); err == nil {
		event.TimestampUTC = ts
	}
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		return event
	}
	if raw := payload["step"]; len(raw) > 0 {
		var step protocol.JobStepPlanItem
		if err := json.Unmarshal(raw, &step); err == nil {
			event.Step = &step
		}
	}
	if raw := payload["phase"]; len(raw) > 0 {
		var phase protocol.JobExecutionPhase
		if err := json.Unmarshal(raw, &phase); err == nil {
			event.Phase = &phase
		}
	}
	if raw := payload["message"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &event.Message)
	}
	if raw := payload["output"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &event.Output)
	}
	if raw := payload["error"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &event.Error)
	}
	if raw := payload["exit_code"]; len(raw) > 0 {
		var exitCode int
		if err := json.Unmarshal(raw, &exitCode); err == nil {
			event.ExitCode = &exitCode
		}
	}
	if raw := payload["duration_ms"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &event.DurationMS)
	}
	return event
}
