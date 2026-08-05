package store

import (
	"path/filepath"
	"testing"
)

func TestCommandReceiptLifecycle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	receipt, claimed, err := db.ClaimCommandReceipt("key-1", "run_pipeline", "hash-1")
	if err != nil || !claimed || receipt.Status != "pending" {
		t.Fatalf("claim receipt=%+v claimed=%v err=%v", receipt, claimed, err)
	}
	if err := db.CompleteCommandReceipt("key-1", `{"enqueued":1}`); err != nil {
		t.Fatal(err)
	}
	receipt, claimed, err = db.ClaimCommandReceipt("key-1", "run_pipeline", "hash-1")
	if err != nil || claimed || receipt.Status != "completed" || receipt.ResultJSON == "" {
		t.Fatalf("repeat receipt=%+v claimed=%v err=%v", receipt, claimed, err)
	}
}

func TestFailedCommandReceiptLifecycle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := db.ClaimCommandReceipt("key-failed", "run_pipeline", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := db.FailCommandReceipt("key-failed", `{"kind":"invalid_argument","message":"bad selection"}`); err != nil {
		t.Fatal(err)
	}
	receipt, claimed, err := db.ClaimCommandReceipt("key-failed", "run_pipeline", "hash")
	if err != nil || claimed || receipt.Status != "failed" || receipt.ResultJSON == "" {
		t.Fatalf("receipt=%+v claimed=%v err=%v", receipt, claimed, err)
	}
}

func TestMarkPendingCommandReceiptsUnknown(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := db.ClaimCommandReceipt("pending-key", "run_pipeline", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkPendingCommandReceiptsUnknown(); err != nil {
		t.Fatal(err)
	}
	receipt, err := db.GetCommandReceipt("pending-key")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "outcome_unknown" {
		t.Fatalf("status = %q", receipt.Status)
	}
}
