package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/store"
)

func TestCommandReceiptRepositoryLifecycleAndCancellation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ciwi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := NewCommandReceiptRepository(db)
	receipt, claimed, err := repository.Claim(context.Background(), "key-1", "run-pipeline", "fingerprint-1")
	if err != nil || !claimed || receipt.Status != "pending" {
		t.Fatalf("claim = %#v, %v, %v", receipt, claimed, err)
	}
	if err := repository.Complete(context.Background(), "key-1", []byte(`{"enqueued":1}`)); err != nil {
		t.Fatal(err)
	}
	receipt, found, err := repository.Get(context.Background(), "key-1")
	if err != nil || !found || receipt.Status != "completed" || string(receipt.Result) != `{"enqueued":1}` {
		t.Fatalf("get = %#v, %v, %v", receipt, found, err)
	}
	if _, found, err := repository.Get(context.Background(), "missing"); err != nil || found {
		t.Fatalf("missing receipt = %v, %v", found, err)
	}
	if _, _, err := repository.Claim(context.Background(), "key-2", "run-pipeline", "fingerprint-2"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Fail(context.Background(), "key-2", []byte(`{"message":"failed"}`)); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := repository.Claim(ctx, "key-3", "run-pipeline", "fingerprint-3"); !errors.Is(err, context.Canceled) {
		t.Fatalf("claim cancellation = %v", err)
	}
	if err := repository.Complete(ctx, "key-1", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("complete cancellation = %v", err)
	}
	if err := repository.Fail(ctx, "key-1", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("fail cancellation = %v", err)
	}
	if _, _, err := repository.Get(ctx, "key-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("get cancellation = %v", err)
	}
}
