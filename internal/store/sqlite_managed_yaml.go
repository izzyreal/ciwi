package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

var (
	ErrManagedYAMLNameConflict     = errors.New("managed YAML project name already exists")
	ErrManagedYAMLRevisionConflict = errors.New("managed YAML project changed since it was loaded")
	ErrProjectIsNotManagedYAML     = errors.New("project is not managed YAML")
)

func ManagedYAMLRevision(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateManagedYAMLProject(cfg config.File, raw string) (protocol.ManagedYAMLDefinition, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("begin managed YAML create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	name := strings.TrimSpace(cfg.Project.Name)
	if err := ensureManagedYAMLNameAvailable(tx, name, 0); err != nil {
		return protocol.ManagedYAMLDefinition{}, err
	}
	key, err := newManagedYAMLConfigPath()
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, err
	}
	revision := ManagedYAMLRevision(raw)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.Exec(`
		INSERT INTO projects (name, source_kind, config_path, config_yaml, config_revision, repo_url, repo_ref, config_file, loaded_commit, created_utc, updated_utc)
		VALUES (?, ?, ?, ?, ?, '', '', '', '', ?, ?)
	`, name, protocol.ProjectSourceManagedYAML, key, raw, revision, now, now)
	if err != nil {
		if isManagedYAMLNameConstraint(err) {
			return protocol.ManagedYAMLDefinition{}, ErrManagedYAMLNameConflict
		}
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("insert managed YAML project: %w", err)
	}
	projectID, err := res.LastInsertId()
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("read managed YAML project id: %w", err)
	}
	cfg.Project.Name = name
	if err := replaceProjectDefinition(tx, projectID, cfg, now); err != nil {
		return protocol.ManagedYAMLDefinition{}, err
	}
	if err := tx.Commit(); err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("commit managed YAML create: %w", err)
	}
	return managedYAMLDefinition(projectID, name, raw, revision, cfg), nil
}

func (s *Store) GetManagedYAMLProject(projectID int64) (protocol.ManagedYAMLDefinition, error) {
	var definition protocol.ManagedYAMLDefinition
	var sourceKind string
	err := s.db.QueryRow(`
		SELECT id, name, source_kind, config_yaml, config_revision
		FROM projects
		WHERE id = ?
	`, projectID).Scan(&definition.ProjectID, &definition.ProjectName, &sourceKind, &definition.YAML, &definition.Revision)
	if err == sql.ErrNoRows {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("project not found")
	}
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("get managed YAML project: %w", err)
	}
	if sourceKind != protocol.ProjectSourceManagedYAML {
		return protocol.ManagedYAMLDefinition{}, ErrProjectIsNotManagedYAML
	}
	var errCount error
	errCount = s.db.QueryRow(`SELECT COUNT(*) FROM pipelines WHERE project_id = ?`, projectID).Scan(&definition.Pipelines)
	if errCount != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("count managed YAML pipelines: %w", errCount)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pipeline_chains WHERE project_id = ?`, projectID).Scan(&definition.PipelineChains); err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("count managed YAML chains: %w", err)
	}
	return definition, nil
}

func (s *Store) ValidateManagedYAMLName(name string, exceptProjectID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin managed YAML name validation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if exceptProjectID > 0 {
		var sourceKind string
		if err := tx.QueryRow(`SELECT source_kind FROM projects WHERE id = ?`, exceptProjectID).Scan(&sourceKind); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("project not found")
			}
			return fmt.Errorf("get managed YAML validation project: %w", err)
		}
		if sourceKind != protocol.ProjectSourceManagedYAML {
			return ErrProjectIsNotManagedYAML
		}
	}
	return ensureManagedYAMLNameAvailable(tx, strings.TrimSpace(name), exceptProjectID)
}

func (s *Store) UpdateManagedYAMLProject(projectID int64, expectedRevision string, cfg config.File, raw string) (protocol.ManagedYAMLDefinition, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("begin managed YAML update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceKind string
	if err := tx.QueryRow(`SELECT source_kind FROM projects WHERE id = ?`, projectID).Scan(&sourceKind); err != nil {
		if err == sql.ErrNoRows {
			return protocol.ManagedYAMLDefinition{}, fmt.Errorf("project not found")
		}
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("get managed YAML project source: %w", err)
	}
	if sourceKind != protocol.ProjectSourceManagedYAML {
		return protocol.ManagedYAMLDefinition{}, ErrProjectIsNotManagedYAML
	}
	name := strings.TrimSpace(cfg.Project.Name)
	if err := ensureManagedYAMLNameAvailable(tx, name, projectID); err != nil {
		return protocol.ManagedYAMLDefinition{}, err
	}
	revision := ManagedYAMLRevision(raw)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.Exec(`
		UPDATE projects
		SET name = ?, config_yaml = ?, config_revision = ?, updated_utc = ?
		WHERE id = ? AND config_revision = ?
	`, name, raw, revision, now, projectID, expectedRevision)
	if err != nil {
		if isManagedYAMLNameConstraint(err) {
			return protocol.ManagedYAMLDefinition{}, ErrManagedYAMLNameConflict
		}
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("update managed YAML project: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("managed YAML update rows affected: %w", err)
	}
	if updated == 0 {
		return protocol.ManagedYAMLDefinition{}, ErrManagedYAMLRevisionConflict
	}
	cfg.Project.Name = name
	if err := replaceProjectDefinition(tx, projectID, cfg, now); err != nil {
		return protocol.ManagedYAMLDefinition{}, err
	}
	if err := tx.Commit(); err != nil {
		return protocol.ManagedYAMLDefinition{}, fmt.Errorf("commit managed YAML update: %w", err)
	}
	return managedYAMLDefinition(projectID, name, raw, revision, cfg), nil
}

func ensureManagedYAMLNameAvailable(tx *sql.Tx, name string, exceptProjectID int64) error {
	var id int64
	err := tx.QueryRow(`
		SELECT id
		FROM projects
		WHERE source_kind = ? AND lower(name) = lower(?) AND id != ?
		LIMIT 1
	`, protocol.ProjectSourceManagedYAML, name, exceptProjectID).Scan(&id)
	if err == nil {
		return ErrManagedYAMLNameConflict
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check managed YAML project name: %w", err)
	}
	return nil
}

func managedYAMLDefinition(projectID int64, name, raw, revision string, cfg config.File) protocol.ManagedYAMLDefinition {
	return protocol.ManagedYAMLDefinition{
		ProjectID:      projectID,
		ProjectName:    name,
		YAML:           raw,
		Revision:       revision,
		Pipelines:      len(cfg.Pipelines),
		PipelineChains: len(cfg.PipelineChains),
	}
}

func newManagedYAMLConfigPath() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate managed YAML identity: %w", err)
	}
	return "managed-yaml:" + hex.EncodeToString(raw), nil
}

func isManagedYAMLNameConstraint(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "idx_projects_managed_yaml_name") ||
		(strings.Contains(text, "unique constraint failed") && strings.Contains(text, "projects.name"))
}
