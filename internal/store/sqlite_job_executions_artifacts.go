package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
			if _, err := tx.Exec(`
				INSERT INTO job_execution_events (job_execution_id, event_type, timestamp_utc, payload_json, created_utc)
				VALUES (?, ?, ?, ?, ?)
			`, jobID, eventType, ts.UTC().Format(time.RFC3339Nano), string(payloadJSON), now); err != nil {
				return fmt.Errorf("insert event: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		return nil
	})
}

func (s *Store) ListJobExecutionEvents(jobID string) ([]protocol.JobExecutionEvent, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job id is required")
	}
	rows, err := s.db.Query(`
		SELECT event_type, timestamp_utc, payload_json
		FROM job_execution_events
		WHERE job_execution_id = ?
		ORDER BY id ASC
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	out := []protocol.JobExecutionEvent{}
	for rows.Next() {
		var eventType, tsRaw, payloadRaw string
		if err := rows.Scan(&eventType, &tsRaw, &payloadRaw); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, decodeJobExecutionEvent(eventType, tsRaw, payloadRaw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
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
		SELECT job_execution_id, event_type, timestamp_utc, payload_json
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
		var jobID, rowType, tsRaw, payloadRaw string
		if err := rows.Scan(&jobID, &rowType, &tsRaw, &payloadRaw); err != nil {
			return nil, fmt.Errorf("scan event for jobs: %w", err)
		}
		out[jobID] = append(out[jobID], decodeJobExecutionEvent(rowType, tsRaw, payloadRaw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events for jobs: %w", err)
	}
	return out, nil
}

func decodeJobExecutionEvent(eventType, tsRaw, payloadRaw string) protocol.JobExecutionEvent {
	event := protocol.JobExecutionEvent{Type: strings.TrimSpace(eventType)}
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
