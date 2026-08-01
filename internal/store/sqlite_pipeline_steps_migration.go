package store

import (
	"encoding/json"
	"fmt"

	"github.com/izzyreal/ciwi/internal/config"
)

type pipelineJobStepsMigrationRow struct {
	id    int64
	raw   string
	steps []config.PipelineJobStep
}

// migratePipelineJobSteps persists the structured representation that replaced
// the original string-only step list. It is intentionally part of the bridge
// release so the read path can become strict in the following release.
func (s *Store) migratePipelineJobSteps() error {
	rows, err := s.db.Query(`SELECT id, steps_json FROM pipeline_jobs ORDER BY id`)
	if err != nil {
		return fmt.Errorf("inspect pipeline job steps for migration: %w", err)
	}

	var pending []pipelineJobStepsMigrationRow
	for rows.Next() {
		var row pipelineJobStepsMigrationRow
		if err := rows.Scan(&row.id, &row.raw); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan pipeline job steps for migration: %w", err)
		}
		if err := json.Unmarshal([]byte(row.raw), &row.steps); err == nil {
			continue
		}
		var commands []string
		if err := json.Unmarshal([]byte(row.raw), &commands); err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode pipeline job %d steps for migration: %w", row.id, err)
		}
		row.steps = make([]config.PipelineJobStep, 0, len(commands))
		for _, command := range commands {
			row.steps = append(row.steps, config.PipelineJobStep{Run: command})
		}
		pending = append(pending, row)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close pipeline job steps migration rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pipeline job steps for migration: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin pipeline job steps migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, row := range pending {
		raw, err := json.Marshal(row.steps)
		if err != nil {
			return fmt.Errorf("encode pipeline job %d steps for migration: %w", row.id, err)
		}
		if _, err := tx.Exec(`UPDATE pipeline_jobs SET steps_json = ? WHERE id = ?`, string(raw), row.id); err != nil {
			return fmt.Errorf("migrate pipeline job %d steps: %w", row.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pipeline job steps migration: %w", err)
	}
	return nil
}
