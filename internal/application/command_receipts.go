package application

import (
	"context"
	"encoding/json"
	"strings"
)

type CommandReceipt struct {
	Key         string
	Operation   string
	Fingerprint string
	Status      string
	Result      []byte
}

type CommandReceiptRepository interface {
	Claim(context.Context, string, string, string) (CommandReceipt, bool, error)
	Complete(context.Context, string, []byte) error
	Fail(context.Context, string, []byte) error
}

type CommandReceiptSource interface {
	Get(context.Context, string) (CommandReceipt, bool, error)
}

type CommandReceiptStatus struct {
	Found       bool
	Key         string
	Operation   string
	Fingerprint string
	Status      string
	Result      []byte
}

type CommandReceiptQueries struct{ source CommandReceiptSource }

func NewCommandReceiptQueries(source CommandReceiptSource) *CommandReceiptQueries {
	return &CommandReceiptQueries{source: source}
}

func (q *CommandReceiptQueries) Get(ctx context.Context, key string) (CommandReceiptStatus, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return CommandReceiptStatus{}, NewError(ErrorInvalidArgument, "command receipt key is required", nil)
	}
	if q == nil || q.source == nil {
		return CommandReceiptStatus{}, NewError(ErrorUnavailable, "command receipts unavailable", nil)
	}
	receipt, found, err := q.source.Get(ctx, key)
	if err != nil {
		return CommandReceiptStatus{}, WrapInternal("get command receipt", err)
	}
	if !found {
		return CommandReceiptStatus{Found: false, Key: key}, nil
	}
	return CommandReceiptStatus{
		Found: true, Key: receipt.Key, Operation: receipt.Operation, Fingerprint: receipt.Fingerprint,
		Status: receipt.Status, Result: append([]byte(nil), receipt.Result...),
	}, nil
}

type commandErrorReceipt struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

func validateCommandKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if len(key) > 200 {
		return "", NewError(ErrorInvalidArgument, "idempotency key exceeds 200 characters", nil)
	}
	return key, nil
}

func executeIdempotentCommand[T any](ctx context.Context, receipts CommandReceiptRepository, key, operation, fingerprint string, execute func() (T, error)) (T, error) {
	var zero T
	if key == "" || receipts == nil {
		return execute()
	}
	receipt, claimed, err := receipts.Claim(ctx, key, operation, fingerprint)
	if err != nil {
		return zero, WrapInternal("claim command receipt", err)
	}
	if !claimed {
		if receipt.Operation != operation || receipt.Fingerprint != fingerprint {
			return zero, NewError(ErrorConflict, "idempotency key was already used for a different command", nil)
		}
		if receipt.Status != "completed" {
			if receipt.Status == "failed" {
				var failed commandErrorReceipt
				if err := json.Unmarshal(receipt.Result, &failed); err != nil {
					return zero, WrapInternal("decode failed command receipt", err)
				}
				return zero, NewError(failed.Kind, failed.Message, nil)
			}
			if receipt.Status == "outcome_unknown" {
				return zero, NewError(ErrorFailedPrecondition, "the server restarted before the command outcome was recorded", nil)
			}
			return zero, NewError(ErrorUnavailable, "the original command is still pending or its outcome is unknown", nil)
		}
		var result T
		if err := json.Unmarshal(receipt.Result, &result); err != nil {
			return zero, WrapInternal("decode command receipt", err)
		}
		return result, nil
	}

	result, err := execute()
	if err != nil {
		failed, encodeErr := json.Marshal(commandErrorReceipt{Kind: ErrorKindOf(err), Message: err.Error()})
		if encodeErr != nil {
			return zero, WrapInternal("encode failed command receipt", encodeErr)
		}
		if receiptErr := receipts.Fail(context.Background(), key, failed); receiptErr != nil {
			return zero, WrapInternal("persist failed command receipt", receiptErr)
		}
		return zero, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return zero, WrapInternal("encode command receipt", err)
	}
	if err := receipts.Complete(context.Background(), key, encoded); err != nil {
		return zero, WrapInternal("complete command receipt", err)
	}
	return result, nil
}

// ExecuteIdempotentCommand exposes the shared receipt protocol to server-side
// adapters that implement commands outside the core application packages.
func ExecuteIdempotentCommand[T any](ctx context.Context, receipts CommandReceiptRepository, key, operation, fingerprint string, execute func() (T, error)) (T, error) {
	return executeIdempotentCommand(ctx, receipts, key, operation, fingerprint, execute)
}
