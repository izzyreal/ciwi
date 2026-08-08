package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRecordsCurrentSchemaVersion(t *testing.T) {
	s := openTestStore(t)

	var version int
	var name string
	if err := s.db.QueryRow(`SELECT version, name FROM schema_migrations`).Scan(&version, &name); err != nil {
		t.Fatalf("read schema migration: %v", err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, currentSchemaVersion)
	}
	if name == "" {
		t.Fatal("schema migration name is empty")
	}
}

func TestOpenUpgradesPreLedgerDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		config_path TEXT NOT NULL,
		created_utc TEXT NOT NULL,
		updated_utc TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy projects table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO projects (name, config_path, created_utc, updated_utc)
		VALUES ('legacy', 'ciwi-project.yaml', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	defer s.Close()

	var sourceKind, configYAML string
	if err := s.db.QueryRow(`SELECT source_kind, config_yaml FROM projects WHERE name = 'legacy'`).Scan(&sourceKind, &configYAML); err != nil {
		t.Fatalf("read upgraded project: %v", err)
	}
	if sourceKind != "vcs" || configYAML != "" {
		t.Fatalf("unexpected upgraded defaults: source_kind=%q config_yaml=%q", sourceKind, configYAML)
	}
	var version int
	if err := s.db.QueryRow(`SELECT version FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read upgraded schema version: %v", err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("upgraded schema version = %d, want %d", version, currentSchemaVersion)
	}
}

func TestOpenRejectsNewerSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "newer.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open newer database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_utc TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create migration ledger: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations VALUES (?, 'future', '2026-01-01T00:00:00Z')`, currentSchemaVersion+1); err != nil {
		t.Fatalf("insert newer version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close newer database: %v", err)
	}

	_, err = Open(path)
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected newer schema rejection, got %v", err)
	}
}

func TestSchemaMigrationRollsBackOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open rollback database: %v", err)
	}
	defer db.Close()

	baseline := schemaMigration{version: 1, name: "test baseline", apply: func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE durable (id INTEGER PRIMARY KEY)`)
		return err
	}}
	if err := applySchemaMigrations(db, []schemaMigration{baseline}); err != nil {
		t.Fatalf("apply baseline migration: %v", err)
	}
	failing := schemaMigration{version: 2, name: "failing", apply: func(tx *sql.Tx) error {
		if _, err := tx.Exec(`CREATE TABLE transient (id INTEGER PRIMARY KEY)`); err != nil {
			return err
		}
		return errors.New("injected migration failure")
	}}
	if err := applySchemaMigrations(db, []schemaMigration{baseline, failing}); err == nil {
		t.Fatal("expected migration failure")
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("applied migration count = %d, want 1", count)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'transient'`).Scan(&count); err != nil {
		t.Fatalf("inspect rolled back table: %v", err)
	}
	if count != 0 {
		t.Fatal("failed migration left its table behind")
	}
}
