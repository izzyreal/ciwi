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

// nativeReconciliationPlan is deliberately inert. Receipt I/O may run off the
// controller goroutine, while journal and coordinator mutations are applied by
// the controller only after it has verified the session generation.
type nativeReconciliationPlan struct {
	deleteIDs []string
	restore   []operations.Operation
	unknown   int
}

func (p nativeReconciliationPlan) message() string {
	if p.unknown == 0 {
		return ""
	}
	return fmt.Sprintf("%d earlier action(s) have an unknown outcome and were not repeated", p.unknown)
}

// inspect verifies journaled mutations against the connected server without
// changing local state. A safe mutation is eligible for replay only when the
// stable server identity matches and that server has no receipt for its key.
func (j *nativeOperationJournal) inspect(ctx context.Context, client journalReceiptClient) (nativeReconciliationPlan, error) {
	var plan nativeReconciliationPlan
	serverID := ""
	if client != nil && client.Welcome() != nil {
		serverID = client.Welcome().ServerInstallationId
	}
	if serverID == "" {
		return plan, nil
	}
	entries, err := j.Entries()
	if err != nil {
		return plan, err
	}
	restoreScopes := map[string]bool{}
	restoreFingerprints := map[string]bool{}
	for _, entry := range entries {
		operation := entry.Operation
		if entry.ServerInstallationID == "" || entry.ServerInstallationID != serverID {
			plan.unknown++
			continue
		}
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		receipt, receiptErr := client.GetCommandReceiptStatus(requestCtx, operation.IdempotencyKey)
		cancel()
		if receiptErr != nil {
			return nativeReconciliationPlan{}, receiptErr
		}
		if receipt == nil {
			return nativeReconciliationPlan{}, fmt.Errorf("receipt status for %q was empty", operation.IdempotencyKey)
		}
		if receipt.Found {
			switch receipt.Status {
			case "completed", "failed":
				plan.deleteIDs = append(plan.deleteIDs, operation.ID)
			case "outcome_unknown", "pending":
				plan.unknown++
			default:
				plan.unknown++
			}
			continue
		}
		if operation.Persistence != uidsl.ActionPersistenceSafe || len(operation.Arguments) == 0 {
			plan.unknown++
			continue
		}
		if (operation.Scope != "" && restoreScopes[operation.Scope]) || (operation.Fingerprint != "" && restoreFingerprints[operation.Fingerprint]) {
			plan.unknown++
			continue
		}
		plan.restore = append(plan.restore, operation)
		restoreScopes[operation.Scope] = operation.Scope != ""
		restoreFingerprints[operation.Fingerprint] = operation.Fingerprint != ""
	}
	return plan, nil
}

func (j *nativeOperationJournal) apply(plan nativeReconciliationPlan, coordinator *operations.Coordinator) (int, error) {
	for _, id := range plan.deleteIDs {
		if err := j.Delete(id); err != nil {
			return 0, err
		}
	}
	resumed := 0
	for _, operation := range plan.restore {
		if coordinator == nil {
			return resumed, fmt.Errorf("restore operation %q: coordinator is unavailable", operation.ID)
		}
		submission, err := coordinator.Restore(operation)
		if err != nil {
			return resumed, err
		}
		switch submission.Disposition {
		case operations.DispositionAccepted:
			resumed++
		case operations.DispositionDuplicate:
			// The exact operation is already owned by the coordinator.
		case operations.DispositionConflict:
			return resumed, fmt.Errorf("restore operation %q conflicts with an active operation", operation.ID)
		default:
			return resumed, fmt.Errorf("restore operation %q returned disposition %q", operation.ID, submission.Disposition)
		}
	}
	return resumed, nil
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
