//go:build darwin || ios || linux || windows

package gio

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestNativeOperationJournalPersistsStableTargetAndSafeArguments(t *testing.T) {
	directory := t.TempDir()
	journal := newNativeOperationJournal(filepath.Join(directory, "preferences.json"), func() string { return "server-1" })
	operation := operations.Operation{
		ID: "operation-1", IdempotencyKey: "command-1", Command: "run-pipeline",
		Arguments: map[string]string{"pipelineDbId": "7"}, Fingerprint: "fingerprint-1",
		Scope: "pipeline:7", Class: operations.ClassMutation, Persistence: uidsl.ActionPersistenceSafe,
		State: operations.StateRunning, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := journal.Put(operation); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ServerInstallationID != "server-1" || entries[0].Operation.Arguments["pipelineDbId"] != "7" {
		t.Fatalf("entries = %#v", entries)
	}
	info, err := os.Stat(journal.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o", info.Mode().Perm())
	}
	if err := journal.Delete(operation.ID); err != nil {
		t.Fatal(err)
	}
	entries, err = journal.Entries()
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries after delete = %#v, %v", entries, err)
	}
}

func TestNativeOperationJournalOmitsReceiptOnlyArguments(t *testing.T) {
	journal := newNativeOperationJournal(filepath.Join(t.TempDir(), "preferences.json"), func() string { return "server-1" })
	operation := operations.Operation{
		ID: "operation-2", IdempotencyKey: "command-2", Command: "server-update-action",
		Arguments: map[string]string{"targetVersion": "v1.2.3"}, Fingerprint: "fingerprint-2",
		Scope: "server:update", Class: operations.ClassMutation, Persistence: uidsl.ActionPersistenceReceipt,
	}
	if err := journal.Put(operation); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.Entries()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
	if len(entries[0].Operation.Arguments) != 0 {
		t.Fatalf("receipt-only arguments persisted: %#v", entries[0].Operation.Arguments)
	}
}
