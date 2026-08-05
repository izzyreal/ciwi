//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation/operations"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

type journalReceiptClient interface {
	Welcome() *cnpv1.Welcome
	GetCommandReceiptStatus(context.Context, string) (*cnpv1.CommandReceiptStatus, error)
}

type nativeJournalEntry struct {
	ServerInstallationID string               `json:"serverInstallationId,omitempty"`
	Operation            operations.Operation `json:"operation"`
}

// reconcile verifies journaled mutations against the connected server before
// resuming anything. A safe mutation is only replayed when the stable server
// identity matches and that server has no receipt for its idempotency key.
func (j *nativeOperationJournal) reconcile(ctx context.Context, client journalReceiptClient, coordinator *operations.Coordinator) (int, string, error) {
	serverID := ""
	if client != nil && client.Welcome() != nil {
		serverID = client.Welcome().ServerInstallationId
	}
	if serverID == "" {
		return 0, "", nil
	}
	entries, err := j.Entries()
	if err != nil {
		return 0, "", err
	}
	resumed := 0
	unknown := 0
	for _, entry := range entries {
		operation := entry.Operation
		if entry.ServerInstallationID == "" || entry.ServerInstallationID != serverID {
			unknown++
			continue
		}
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		receipt, receiptErr := client.GetCommandReceiptStatus(requestCtx, operation.IdempotencyKey)
		cancel()
		if receiptErr != nil {
			return resumed, "", receiptErr
		}
		if receipt.Found {
			switch receipt.Status {
			case "completed", "failed":
				_ = j.Delete(operation.ID)
			case "outcome_unknown", "pending":
				unknown++
			}
			continue
		}
		if operation.Persistence != uidsl.ActionPersistenceSafe || len(operation.Arguments) == 0 {
			unknown++
			continue
		}
		if _, restoreErr := coordinator.Restore(operation); restoreErr != nil {
			return resumed, "", restoreErr
		}
		resumed++
	}
	message := ""
	if unknown > 0 {
		message = fmt.Sprintf("%d earlier action(s) have an unknown outcome and were not repeated", unknown)
	}
	return resumed, message, nil
}

type nativeJournalDocument struct {
	Entries map[string]nativeJournalEntry `json:"entries"`
}

type nativeOperationJournal struct {
	mu       sync.Mutex
	path     string
	serverID func() string
}

func newNativeOperationJournal(preferencesPath string, serverID func() string) *nativeOperationJournal {
	return &nativeOperationJournal{path: filepath.Join(filepath.Dir(preferencesPath), "native-operations.json"), serverID: serverID}
}

func (j *nativeOperationJournal) Put(operation operations.Operation) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	document, err := j.load()
	if err != nil {
		return err
	}
	if document.Entries == nil {
		document.Entries = map[string]nativeJournalEntry{}
	}
	persisted := operation
	persisted.Value = nil
	if persisted.Persistence == uidsl.ActionPersistenceReceipt {
		persisted.Arguments = nil
	}
	entry := document.Entries[operation.ID]
	entry.Operation = persisted
	if entry.ServerInstallationID == "" && j.serverID != nil {
		entry.ServerInstallationID = j.serverID()
	}
	document.Entries[operation.ID] = entry
	return j.save(document)
}

func (j *nativeOperationJournal) Delete(id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	document, err := j.load()
	if err != nil {
		return err
	}
	delete(document.Entries, id)
	return j.save(document)
}

func (j *nativeOperationJournal) Entries() ([]nativeJournalEntry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	document, err := j.load()
	if err != nil {
		return nil, err
	}
	entries := make([]nativeJournalEntry, 0, len(document.Entries))
	for _, entry := range document.Entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, k int) bool {
		return entries[i].Operation.CreatedAt.Before(entries[k].Operation.CreatedAt)
	})
	return entries, nil
}

func (j *nativeOperationJournal) load() (nativeJournalDocument, error) {
	payload, err := os.ReadFile(j.path)
	if errors.Is(err, os.ErrNotExist) {
		return nativeJournalDocument{Entries: map[string]nativeJournalEntry{}}, nil
	}
	if err != nil {
		return nativeJournalDocument{}, fmt.Errorf("read native operation journal: %w", err)
	}
	var document nativeJournalDocument
	if err := json.Unmarshal(payload, &document); err != nil {
		return nativeJournalDocument{}, fmt.Errorf("decode native operation journal: %w", err)
	}
	if document.Entries == nil {
		document.Entries = map[string]nativeJournalEntry{}
	}
	return document, nil
}

func (j *nativeOperationJournal) save(document nativeJournalDocument) error {
	payload, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode native operation journal: %w", err)
	}
	directory := filepath.Dir(j.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create native operation journal directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, "native-operations-*.json")
	if err != nil {
		return fmt.Errorf("create native operation journal: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, j.path); err != nil {
		return fmt.Errorf("replace native operation journal: %w", err)
	}
	return nil
}
