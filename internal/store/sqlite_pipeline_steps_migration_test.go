package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/config"
)

func TestPipelineJobStepsMigrationPersistsStructuredSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ciwi.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	cfg := config.File{
		Version: 1,
		Project: config.Project{Name: "bridge"},
		Pipelines: []config.Pipeline{{
			ID:      "build",
			Trigger: "manual",
			Jobs: []config.PipelineJobSpec{{
				ID:             "compile",
				RunsOn:         map[string]string{"executor": "script", "shell": "posix"},
				TimeoutSeconds: 60,
				Steps:          []config.PipelineJobStep{{Run: "current"}},
			}},
		}},
	}
	if err := s.LoadConfig(cfg, "bridge.yaml", "", "", ""); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE pipeline_jobs SET steps_json = ? WHERE job_id = ?`, `["echo one","echo two"]`, "compile"); err != nil {
		t.Fatalf("write string-only steps: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	var firstRaw string
	if err := s.db.QueryRow(`SELECT steps_json FROM pipeline_jobs WHERE job_id = ?`, "compile").Scan(&firstRaw); err != nil {
		t.Fatalf("read migrated steps: %v", err)
	}
	var steps []config.PipelineJobStep
	if err := json.Unmarshal([]byte(firstRaw), &steps); err != nil {
		t.Fatalf("decode migrated steps: %v", err)
	}
	if len(steps) != 2 || steps[0].Run != "echo one" || steps[1].Run != "echo two" {
		t.Fatalf("unexpected migrated steps: %+v", steps)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatalf("reopen migrated store: %v", err)
	}
	defer s.Close()
	var secondRaw string
	if err := s.db.QueryRow(`SELECT steps_json FROM pipeline_jobs WHERE job_id = ?`, "compile").Scan(&secondRaw); err != nil {
		t.Fatalf("read steps after second open: %v", err)
	}
	if secondRaw != firstRaw {
		t.Fatalf("migration was not idempotent: first=%q second=%q", firstRaw, secondRaw)
	}
}

func TestPipelineJobStepsMigrationRejectsUnreadableRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ciwi.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO projects (name, config_path, created_utc, updated_utc)
		VALUES ('bridge', 'bridge.yaml', 'now', 'now')
	`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO pipelines (project_id, pipeline_id, created_utc, updated_utc)
		VALUES (1, 'build', 'now', 'now')
	`); err != nil {
		t.Fatalf("insert pipeline: %v", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO pipeline_jobs (
			pipeline_id, job_id, position, runs_on_json, timeout_seconds, matrix_json, steps_json
		) VALUES
			(1, 'valid-old-row', 0, '{}', 60, '[]', '["echo still old"]'),
			(1, 'broken-row', 1, '{}', 60, '[]', '{broken')
	`); err != nil {
		t.Fatalf("insert invalid steps: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	if _, err := Open(path); err == nil {
		t.Fatal("expected invalid persisted steps to fail migration")
	}
	rawDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	defer rawDB.Close()
	var raw string
	if err := rawDB.QueryRow(`SELECT steps_json FROM pipeline_jobs WHERE job_id = 'valid-old-row'`).Scan(&raw); err != nil {
		t.Fatalf("read valid row after failed migration: %v", err)
	}
	if raw != `["echo still old"]` {
		t.Fatalf("failed migration partially rewrote rows: %q", raw)
	}
}
