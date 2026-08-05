//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation/operations"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

type journalReceiptFake struct {
	serverID string
	receipts map[string]*cnpv1.CommandReceiptStatus
	err      error
}

func (f journalReceiptFake) Welcome() *cnpv1.Welcome {
	return &cnpv1.Welcome{ServerInstallationId: f.serverID}
}

func (f journalReceiptFake) GetCommandReceiptStatus(_ context.Context, key string) (*cnpv1.CommandReceiptStatus, error) {
	if f.err != nil {
		return nil, f.err
	}
	if receipt := f.receipts[key]; receipt != nil {
		return receipt, nil
	}
	return &cnpv1.CommandReceiptStatus{}, nil
}

type journalExecutorFunc func(context.Context, operations.Operation) operations.Result

func (f journalExecutorFunc) Execute(ctx context.Context, operation operations.Operation) operations.Result {
	return f(ctx, operation)
}

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

func TestNativeOperationJournalRejectsMalformedDocument(t *testing.T) {
	journal := newNativeOperationJournal(filepath.Join(t.TempDir(), "preferences.json"), nil)
	if err := os.WriteFile(journal.path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Entries(); err == nil {
		t.Fatal("expected malformed journal to fail")
	}
}

func TestNativeOperationJournalReconcileDeletesTerminalReceipt(t *testing.T) {
	journal := newNativeOperationJournal(filepath.Join(t.TempDir(), "preferences.json"), func() string { return "server-1" })
	operation := operations.Operation{
		ID: "operation-1", IdempotencyKey: "command-1", Command: "clear-queue",
		Class: operations.ClassMutation, Persistence: uidsl.ActionPersistenceSafe,
		Arguments: map[string]string{"confirm": "true"}, CreatedAt: time.Now().UTC(),
	}
	if err := journal.Put(operation); err != nil {
		t.Fatal(err)
	}
	var executions atomic.Int32
	coordinator := operations.New(context.Background(), 1, journalExecutorFunc(func(context.Context, operations.Operation) operations.Result {
		executions.Add(1)
		return operations.Result{}
	}), journal)
	defer coordinator.Close()
	resumed, message, err := journal.reconcile(context.Background(), journalReceiptFake{
		serverID: "server-1", receipts: map[string]*cnpv1.CommandReceiptStatus{
			"command-1": {Found: true, Status: "completed"},
		},
	}, coordinator)
	if err != nil || resumed != 0 || message != "" {
		t.Fatalf("reconcile = %d, %q, %v", resumed, message, err)
	}
	if executions.Load() != 0 {
		t.Fatal("terminal receipt was replayed")
	}
	entries, err := journal.Entries()
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
}

func TestNativeOperationJournalReconcileReplaysOnlySafeMatchingOperation(t *testing.T) {
	serverID := "server-1"
	journal := newNativeOperationJournal(filepath.Join(t.TempDir(), "preferences.json"), func() string { return serverID })
	safe := operations.Operation{
		ID: "safe", IdempotencyKey: "safe-key", Command: "clear-queue", Fingerprint: "safe-fingerprint",
		Scope: "queue", Class: operations.ClassMutation, Persistence: uidsl.ActionPersistenceSafe,
		Arguments: map[string]string{"confirm": "true"}, State: operations.StateOutcomeUnknown, CreatedAt: time.Now().UTC(),
	}
	if err := journal.Put(safe); err != nil {
		t.Fatal(err)
	}
	serverID = "server-2"
	mismatch := safe
	mismatch.ID, mismatch.IdempotencyKey, mismatch.Fingerprint = "mismatch", "mismatch-key", "mismatch-fingerprint"
	if err := journal.Put(mismatch); err != nil {
		t.Fatal(err)
	}
	executed := make(chan operations.Operation, 1)
	coordinator := operations.New(context.Background(), 1, journalExecutorFunc(func(_ context.Context, operation operations.Operation) operations.Result {
		executed <- operation
		return operations.Result{State: operations.StateSucceeded}
	}), journal)
	defer coordinator.Close()
	resumed, message, err := journal.reconcile(context.Background(), journalReceiptFake{serverID: "server-1"}, coordinator)
	if err != nil || resumed != 1 || message == "" {
		t.Fatalf("reconcile = %d, %q, %v", resumed, message, err)
	}
	select {
	case operation := <-executed:
		if operation.ID != safe.ID || operation.IdempotencyKey != safe.IdempotencyKey {
			t.Fatalf("replayed operation = %#v", operation)
		}
	case <-time.After(time.Second):
		t.Fatal("safe operation was not replayed")
	}
}

func TestNativeOperationJournalReconcilePropagatesReceiptFailure(t *testing.T) {
	journal := newNativeOperationJournal(filepath.Join(t.TempDir(), "preferences.json"), func() string { return "server-1" })
	if err := journal.Put(operations.Operation{
		ID: "operation-1", IdempotencyKey: "command-1", Command: "clear-queue",
		Class: operations.ClassMutation, Persistence: uidsl.ActionPersistenceSafe,
		Arguments: map[string]string{"confirm": "true"}, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	want := errors.New("receipt unavailable")
	_, _, err := journal.reconcile(context.Background(), journalReceiptFake{serverID: "server-1", err: want}, nil)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
