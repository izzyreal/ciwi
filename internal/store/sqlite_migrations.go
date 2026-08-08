package store

import (
	"database/sql"
	"fmt"
	"time"
)

const currentSchemaVersion = 1

type schemaMigration struct {
	version int
	name    string
	apply   func(*sql.Tx) error
}

var schemaMigrations = []schemaMigration{
	{
		version: 1,
		name:    "baseline current schema",
		apply:   migrateToCurrentSchema,
	},
}

func (s *Store) migrateSchema() error {
	if err := validateSchemaMigrations(schemaMigrations); err != nil {
		return err
	}
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
	} {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	return applySchemaMigrations(s.db, schemaMigrations)
}

func validateSchemaMigrations(migrations []schemaMigration) error {
	if len(migrations) != currentSchemaVersion {
		return fmt.Errorf("schema migration catalog has %d entries, want %d", len(migrations), currentSchemaVersion)
	}
	for i, migration := range migrations {
		if migration.version != i+1 {
			return fmt.Errorf("schema migration catalog is not contiguous at version %d", i+1)
		}
		if migration.name == "" || migration.apply == nil {
			return fmt.Errorf("schema migration %d is incomplete", migration.version)
		}
	}
	return nil
}

func applySchemaMigrations(db *sql.DB, migrations []schemaMigration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_utc TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema migration ledger: %w", err)
	}

	applied, err := appliedSchemaVersions(tx)
	if err != nil {
		return err
	}
	for version := range applied {
		if version > currentSchemaVersion {
			return fmt.Errorf("database schema version %d is newer than supported version %d", version, currentSchemaVersion)
		}
	}
	for version := 1; version < currentSchemaVersion; version++ {
		if !applied[version] && applied[version+1] {
			return fmt.Errorf("database schema migration ledger has a gap before version %d", version+1)
		}
	}

	for _, migration := range migrations {
		if applied[migration.version] {
			continue
		}
		if err := migration.apply(tx); err != nil {
			return fmt.Errorf("apply schema migration %d (%s): %w", migration.version, migration.name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, name, applied_utc) VALUES (?, ?, ?)`,
			migration.version,
			migration.name,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record schema migration %d: %w", migration.version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migrations: %w", err)
	}
	return nil
}

func appliedSchemaVersions(tx *sql.Tx) (map[int]bool, error) {
	rows, err := tx.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("list applied schema migrations: %w", err)
	}
	defer rows.Close()

	versions := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied schema migration: %w", err)
		}
		versions[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied schema migrations: %w", err)
	}
	return versions, nil
}

func migrateToCurrentSchema(tx *sql.Tx) error {
	if err := initializeCurrentSchema(tx); err != nil {
		return err
	}

	// Databases created before the migration ledger may have any of these
	// tables without the columns added by later ciwi releases. Keeping this
	// bridge in the baseline migration makes adopting the ledger non-breaking.
	columns := []struct {
		table      string
		name       string
		definition string
	}{
		{"projects", "repo_url", "TEXT"},
		{"projects", "repo_ref", "TEXT"},
		{"projects", "config_file", "TEXT"},
		{"projects", "loaded_commit", "TEXT NOT NULL DEFAULT ''"},
		{"projects", "source_kind", "TEXT NOT NULL DEFAULT 'vcs'"},
		{"projects", "config_yaml", "TEXT NOT NULL DEFAULT ''"},
		{"projects", "config_revision", "TEXT NOT NULL DEFAULT ''"},
		{"pipelines", "depends_on_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"pipelines", "versioning_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"pipeline_chains", "chain_name", "TEXT NOT NULL DEFAULT ''"},
		{"pipeline_chains", "position", "INTEGER NOT NULL DEFAULT 0"},
		{"pipeline_jobs", "needs_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"pipeline_jobs", "artifact_sources_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"pipeline_jobs", "requires_tools_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"pipeline_jobs", "requires_container_tools_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"pipeline_jobs", "requires_capabilities_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"pipeline_jobs", "artifacts_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"pipeline_jobs", "caches_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"job_executions", "env_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"job_executions", "artifact_globs_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"job_executions", "dependency_artifact_job_ids_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"job_executions", "caches_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"job_executions", "step_plan_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"job_executions", "cache_stats_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"job_executions", "runtime_capabilities_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"job_executions", "current_step_text", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(tx, column.table, column.name, column.definition); err != nil {
			return err
		}
	}

	for _, stmt := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_managed_yaml_name ON projects(lower(name)) WHERE source_kind = 'managed_yaml'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_managed_yaml_path ON projects(config_path) WHERE source_kind = 'managed_yaml'`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("create current schema index: %w", err)
		}
	}
	return nil
}

func addColumnIfMissing(tx *sql.Tx, table, column, definition string) error {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("inspect column %s.%s: %w", table, column, err)
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan columns for %s: %w", table, err)
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close columns for %s: %w", table, err)
	}
	if found {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}
