package store

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) SetAppState(key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`
		INSERT INTO app_state (key, value, updated_utc)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_utc=excluded.updated_utc
	`, key, value, now); err != nil {
		return fmt.Errorf("set app state: %w", err)
	}
	return nil
}

func (s *Store) GetAppState(key string) (string, bool, error) {
	var value string
	row := s.db.QueryRow(`SELECT value FROM app_state WHERE key = ?`, key)
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get app state: %w", err)
	}
	return value, true, nil
}

func (s *Store) ListAppState() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM app_state`)
	if err != nil {
		return nil, fmt.Errorf("list app state: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan app state: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate app state: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteAppState(key string) error {
	if _, err := s.db.Exec(`DELETE FROM app_state WHERE key = ?`, key); err != nil {
		return fmt.Errorf("delete app state: %w", err)
	}
	return nil
}
