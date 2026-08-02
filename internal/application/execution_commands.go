package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	clearExecutionQueueOperation   = "clear_execution_queue"
	flushExecutionHistoryOperation = "flush_execution_history"
)

type ClearExecutionQueueRequest struct {
	IdempotencyKey string
}

type ClearExecutionQueueResult struct {
	Cleared int64 `json:"cleared"`
}

type FlushExecutionHistoryRequest struct {
	All             bool
	JobExecutionIDs []string
	IdempotencyKey  string
}

type FlushExecutionHistoryResult struct {
	Flushed int64 `json:"flushed"`
}

// ExecutionMutator owns the persistence and artifact-cleanup side of execution
// housekeeping. The application service deliberately does not know whether
// those records live in SQLite or where server-side artifacts are stored.
type ExecutionMutator interface {
	ClearQueuedExecutions(context.Context) (int64, error)
	FlushExecutionHistory(context.Context, bool, []string) ([]string, error)
}

type ExecutionCommands struct {
	mutator  ExecutionMutator
	receipts CommandReceiptRepository
	changes  *ChangeHub
}

func NewExecutionCommands(mutator ExecutionMutator, receipts CommandReceiptRepository, changes *ChangeHub) *ExecutionCommands {
	return &ExecutionCommands{mutator: mutator, receipts: receipts, changes: changes}
}

func (c *ExecutionCommands) ClearQueue(ctx context.Context, request ClearExecutionQueueRequest) (ClearExecutionQueueResult, error) {
	if c == nil || c.mutator == nil {
		return ClearExecutionQueueResult{}, NewError(ErrorUnavailable, "execution mutator unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return ClearExecutionQueueResult{}, err
	}
	execute := func() (ClearExecutionQueueResult, error) {
		cleared, err := c.mutator.ClearQueuedExecutions(ctx)
		if err != nil {
			return ClearExecutionQueueResult{}, WrapInternal("clear execution queue", err)
		}
		if cleared > 0 && c.changes != nil {
			c.changes.Publish(ChangeQueue)
		}
		return ClearExecutionQueueResult{Cleared: cleared}, nil
	}
	return executeIdempotentCommand(ctx, c.receipts, key, clearExecutionQueueOperation, clearExecutionQueueOperation, execute)
}

func (c *ExecutionCommands) FlushHistory(ctx context.Context, request FlushExecutionHistoryRequest) (FlushExecutionHistoryResult, error) {
	if c == nil || c.mutator == nil {
		return FlushExecutionHistoryResult{}, NewError(ErrorUnavailable, "execution mutator unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return FlushExecutionHistoryResult{}, err
	}
	request.JobExecutionIDs = normalizeExecutionIDs(request.JobExecutionIDs)
	if request.All && len(request.JobExecutionIDs) > 0 {
		return FlushExecutionHistoryResult{}, NewError(ErrorInvalidArgument, "flush all cannot be combined with execution ids", nil)
	}
	fingerprintRequest := request
	fingerprintRequest.IdempotencyKey = ""
	fingerprint, err := executionCommandFingerprint(fingerprintRequest)
	if err != nil {
		return FlushExecutionHistoryResult{}, WrapInternal("fingerprint flush history request", err)
	}
	execute := func() (FlushExecutionHistoryResult, error) {
		deleted, err := c.mutator.FlushExecutionHistory(ctx, request.All, request.JobExecutionIDs)
		if err != nil {
			return FlushExecutionHistoryResult{}, WrapInternal("flush execution history", err)
		}
		if len(deleted) > 0 && c.changes != nil {
			c.changes.Publish(ChangeHistory)
		}
		return FlushExecutionHistoryResult{Flushed: int64(len(deleted))}, nil
	}
	return executeIdempotentCommand(ctx, c.receipts, key, flushExecutionHistoryOperation, fingerprint, execute)
}

func normalizeExecutionIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func executionCommandFingerprint(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
