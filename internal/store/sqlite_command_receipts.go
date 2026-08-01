package store

import (
	"database/sql"
	"fmt"
	"time"
)

type CommandReceipt struct {
	Key         string
	Operation   string
	Fingerprint string
	Status      string
	ResultJSON  string
}

func (s *Store) ClaimCommandReceipt(key, operation, fingerprint string) (CommandReceipt, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`
		INSERT OR IGNORE INTO command_receipts
		(command_key, operation, request_fingerprint, status, result_json, created_utc, updated_utc)
		VALUES (?, ?, ?, 'pending', '', ?, ?)
	`, key, operation, fingerprint, now, now)
	if err != nil {
		return CommandReceipt{}, false, fmt.Errorf("claim command receipt: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return CommandReceipt{}, false, fmt.Errorf("command receipt rows affected: %w", err)
	}
	receipt, err := s.GetCommandReceipt(key)
	if err != nil {
		return CommandReceipt{}, false, err
	}
	return receipt, rows == 1, nil
}

func (s *Store) GetCommandReceipt(key string) (CommandReceipt, error) {
	var receipt CommandReceipt
	err := s.db.QueryRow(`
		SELECT command_key, operation, request_fingerprint, status, result_json
		FROM command_receipts WHERE command_key = ?
	`, key).Scan(&receipt.Key, &receipt.Operation, &receipt.Fingerprint, &receipt.Status, &receipt.ResultJSON)
	if err == sql.ErrNoRows {
		return CommandReceipt{}, fmt.Errorf("command receipt not found")
	}
	if err != nil {
		return CommandReceipt{}, fmt.Errorf("get command receipt: %w", err)
	}
	return receipt, nil
}

func (s *Store) CompleteCommandReceipt(key, resultJSON string) error {
	return s.finishCommandReceipt(key, "completed", resultJSON)
}

func (s *Store) FailCommandReceipt(key, resultJSON string) error {
	return s.finishCommandReceipt(key, "failed", resultJSON)
}

func (s *Store) finishCommandReceipt(key, status, resultJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`
		UPDATE command_receipts SET status = ?, result_json = ?, updated_utc = ?
		WHERE command_key = ? AND status = 'pending'
	`, status, resultJSON, now, key)
	if err != nil {
		return fmt.Errorf("finish command receipt: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish command receipt rows affected: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("pending command receipt not found")
	}
	return nil
}

func (s *Store) AbandonCommandReceipt(key string) error {
	if _, err := s.db.Exec(`DELETE FROM command_receipts WHERE command_key = ? AND status = 'pending'`, key); err != nil {
		return fmt.Errorf("abandon command receipt: %w", err)
	}
	return nil
}
