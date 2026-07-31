package store

import (
	"encoding/json"
	"fmt"

	"github.com/izzyreal/ciwi/internal/pipelinechain"
)

type pipelineChainMigrationRow struct {
	id            int64
	projectID     int64
	chainID       string
	chainName     string
	pipelines     []string
	pipelinesJSON string
}

func (s *Store) migratePipelineChainIdentity() error {
	rows, err := s.db.Query(`
		SELECT id, project_id, chain_id, chain_name, pipelines_json
		FROM pipeline_chains
		ORDER BY project_id, id
	`)
	if err != nil {
		return fmt.Errorf("inspect pipeline chains for identity migration: %w", err)
	}
	var existing []pipelineChainMigrationRow
	for rows.Next() {
		var row pipelineChainMigrationRow
		if err := rows.Scan(&row.id, &row.projectID, &row.chainID, &row.chainName, &row.pipelinesJSON); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan pipeline chain for identity migration: %w", err)
		}
		if err := json.Unmarshal([]byte(row.pipelinesJSON), &row.pipelines); err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode pipeline chain %d for identity migration: %w", row.id, err)
		}
		existing = append(existing, row)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close pipeline chain identity rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pipeline chains for identity migration: %w", err)
	}

	projectsToMigrate := map[int64]bool{}
	for _, row := range existing {
		if row.chainID != pipelinechain.ID(row.pipelines) || row.chainName == "" {
			projectsToMigrate[row.projectID] = true
		}
	}
	if len(projectsToMigrate) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin pipeline chain identity migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	seen := map[string]pipelineChainMigrationRow{}
	positions := map[int64]int{}
	for _, row := range existing {
		if !projectsToMigrate[row.projectID] {
			continue
		}
		pipelines := pipelinechain.NormalizePipelines(row.pipelines)
		canonical, _ := json.Marshal(pipelines)
		chainID := pipelinechain.ID(pipelines)
		key := fmt.Sprintf("%d:%s", row.projectID, chainID)
		if previous, ok := seen[key]; ok {
			if previous.pipelinesJSON != string(canonical) {
				return fmt.Errorf("derived pipeline chain id collision in project %d", row.projectID)
			}
			if _, err := tx.Exec(`DELETE FROM pipeline_chains WHERE id = ?`, row.id); err != nil {
				return fmt.Errorf("remove duplicate pipeline chain %d: %w", row.id, err)
			}
			continue
		}
		row.pipelinesJSON = string(canonical)
		seen[key] = row
		position := positions[row.projectID]
		positions[row.projectID] = position + 1
		if _, err := tx.Exec(`
			UPDATE pipeline_chains
			SET chain_id = ?, chain_name = ?, position = ?, pipelines_json = ?
			WHERE id = ?
		`, chainID, pipelinechain.DisplayName("", pipelines), position, string(canonical), row.id); err != nil {
			return fmt.Errorf("migrate pipeline chain %d identity: %w", row.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pipeline chain identity migration: %w", err)
	}
	return nil
}
