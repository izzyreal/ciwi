package sqlite

import (
	"context"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/store"
)

type CommandReceiptRepository struct {
	store *store.Store
}

func NewCommandReceiptRepository(db *store.Store) *CommandReceiptRepository {
	return &CommandReceiptRepository{store: db}
}

func (r *CommandReceiptRepository) Claim(ctx context.Context, key, operation, fingerprint string) (application.CommandReceipt, bool, error) {
	if err := ctx.Err(); err != nil {
		return application.CommandReceipt{}, false, err
	}
	receipt, claimed, err := r.store.ClaimCommandReceipt(key, operation, fingerprint)
	if err != nil {
		return application.CommandReceipt{}, false, err
	}
	return application.CommandReceipt{
		Key: receipt.Key, Operation: receipt.Operation, Fingerprint: receipt.Fingerprint,
		Status: receipt.Status, Result: []byte(receipt.ResultJSON),
	}, claimed, nil
}

func (r *CommandReceiptRepository) Complete(ctx context.Context, key string, result []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.store.CompleteCommandReceipt(key, string(result))
}

func (r *CommandReceiptRepository) Fail(ctx context.Context, key string, result []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.store.FailCommandReceipt(key, string(result))
}

func (r *CommandReceiptRepository) Get(ctx context.Context, key string) (application.CommandReceipt, bool, error) {
	if err := ctx.Err(); err != nil {
		return application.CommandReceipt{}, false, err
	}
	receipt, found, err := r.store.FindCommandReceipt(key)
	if err != nil || !found {
		return application.CommandReceipt{}, found, err
	}
	return application.CommandReceipt{
		Key: receipt.Key, Operation: receipt.Operation, Fingerprint: receipt.Fingerprint,
		Status: receipt.Status, Result: []byte(receipt.ResultJSON),
	}, true, nil
}
