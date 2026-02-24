package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *Store) UpsertVaultConnection(req protocol.UpsertVaultConnectionRequest) (protocol.VaultConnection, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	auth := strings.TrimSpace(req.AuthMethod)
	if auth == "" {
		auth = "approle"
	}
	mount := strings.TrimSpace(req.AppRoleMount)
	if mount == "" {
		mount = "approle"
	}
	kvVer := req.KVDefaultVer
	if kvVer == 0 {
		kvVer = 2
	}
	if _, err := s.db.Exec(`
		INSERT INTO vault_connections (name, url, auth_method, approle_mount, role_id, secret_id_env, namespace, kv_default_mount, kv_default_version, created_utc, updated_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			url=excluded.url,
			auth_method=excluded.auth_method,
			approle_mount=excluded.approle_mount,
			role_id=excluded.role_id,
			secret_id_env=excluded.secret_id_env,
			namespace=excluded.namespace,
			kv_default_mount=excluded.kv_default_mount,
			kv_default_version=excluded.kv_default_version,
			updated_utc=excluded.updated_utc
	`, req.Name, req.URL, auth, mount, req.RoleID, req.SecretIDEnv, req.Namespace, req.KVDefaultMount, kvVer, now, now); err != nil {
		return protocol.VaultConnection{}, fmt.Errorf("upsert vault connection: %w", err)
	}
	return s.GetVaultConnectionByName(req.Name)
}

func (s *Store) ListVaultConnections() ([]protocol.VaultConnection, error) {
	rows, err := s.db.Query(`
		SELECT id, name, url, auth_method, approle_mount, role_id, secret_id_env, namespace, kv_default_mount, kv_default_version
		FROM vault_connections
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list vault connections: %w", err)
	}
	defer rows.Close()
	out := []protocol.VaultConnection{}
	for rows.Next() {
		var c protocol.VaultConnection
		if err := rows.Scan(&c.ID, &c.Name, &c.URL, &c.AuthMethod, &c.AppRoleMount, &c.RoleID, &c.SecretIDEnv, &c.Namespace, &c.KVDefaultMount, &c.KVDefaultVer); err != nil {
			return nil, fmt.Errorf("scan vault connection: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vault connections: %w", err)
	}
	return out, nil
}

func (s *Store) GetVaultConnectionByID(id int64) (protocol.VaultConnection, error) {
	var c protocol.VaultConnection
	row := s.db.QueryRow(`
		SELECT id, name, url, auth_method, approle_mount, role_id, secret_id_env, namespace, kv_default_mount, kv_default_version
		FROM vault_connections WHERE id = ?
	`, id)
	if err := row.Scan(&c.ID, &c.Name, &c.URL, &c.AuthMethod, &c.AppRoleMount, &c.RoleID, &c.SecretIDEnv, &c.Namespace, &c.KVDefaultMount, &c.KVDefaultVer); err != nil {
		if err == sql.ErrNoRows {
			return protocol.VaultConnection{}, fmt.Errorf("vault connection not found")
		}
		return protocol.VaultConnection{}, fmt.Errorf("get vault connection: %w", err)
	}
	return c, nil
}

func (s *Store) GetVaultConnectionByName(name string) (protocol.VaultConnection, error) {
	var c protocol.VaultConnection
	row := s.db.QueryRow(`
		SELECT id, name, url, auth_method, approle_mount, role_id, secret_id_env, namespace, kv_default_mount, kv_default_version
		FROM vault_connections WHERE name = ?
	`, name)
	if err := row.Scan(&c.ID, &c.Name, &c.URL, &c.AuthMethod, &c.AppRoleMount, &c.RoleID, &c.SecretIDEnv, &c.Namespace, &c.KVDefaultMount, &c.KVDefaultVer); err != nil {
		if err == sql.ErrNoRows {
			return protocol.VaultConnection{}, fmt.Errorf("vault connection not found")
		}
		return protocol.VaultConnection{}, fmt.Errorf("get vault connection: %w", err)
	}
	return c, nil
}

func (s *Store) DeleteVaultConnection(id int64) error {
	res, err := s.db.Exec(`DELETE FROM vault_connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete vault connection: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("vault connection not found")
	}
	return nil
}
