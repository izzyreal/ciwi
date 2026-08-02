package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	cancelExecutionOperation = "cancel_execution"
	rerunExecutionOperation  = "rerun_execution"
)

type ExecutionControlRequest struct {
	JobExecutionID string
	IdempotencyKey string
}

type CancelExecutionResult struct {
	JobExecutionID string `json:"job_execution_id"`
	Status         string `json:"status"`
}

type RerunExecutionResult struct {
	OriginalJobExecutionID string `json:"original_job_execution_id"`
	JobExecutionID         string `json:"job_execution_id"`
	Status                 string `json:"status"`
}

type ExecutionController interface {
	CancelExecution(context.Context, string) (CancelExecutionResult, error)
	RerunExecution(context.Context, string) (RerunExecutionResult, error)
}

type ExecutionControlCommands struct {
	controller ExecutionController
	receipts   CommandReceiptRepository
	changes    *ChangeHub
}

func NewExecutionControlCommands(controller ExecutionController, receipts CommandReceiptRepository, changes *ChangeHub) *ExecutionControlCommands {
	return &ExecutionControlCommands{controller: controller, receipts: receipts, changes: changes}
}

func (c *ExecutionControlCommands) Cancel(ctx context.Context, request ExecutionControlRequest) (CancelExecutionResult, error) {
	jobID, key, err := validateExecutionControlRequest(c, request)
	if err != nil {
		return CancelExecutionResult{}, err
	}
	execute := func() (CancelExecutionResult, error) {
		result, err := c.controller.CancelExecution(ctx, jobID)
		if err != nil {
			return CancelExecutionResult{}, err
		}
		if c.changes != nil {
			c.changes.Publish(ChangeQueue, ChangeHistory)
		}
		return result, nil
	}
	return executeIdempotentCommand(ctx, c.receipts, key, cancelExecutionOperation, executionControlFingerprint(jobID), execute)
}

func (c *ExecutionControlCommands) Rerun(ctx context.Context, request ExecutionControlRequest) (RerunExecutionResult, error) {
	jobID, key, err := validateExecutionControlRequest(c, request)
	if err != nil {
		return RerunExecutionResult{}, err
	}
	execute := func() (RerunExecutionResult, error) {
		result, err := c.controller.RerunExecution(ctx, jobID)
		if err != nil {
			return RerunExecutionResult{}, err
		}
		if c.changes != nil {
			c.changes.Publish(ChangeQueue)
		}
		return result, nil
	}
	return executeIdempotentCommand(ctx, c.receipts, key, rerunExecutionOperation, executionControlFingerprint(jobID), execute)
}

func validateExecutionControlRequest(c *ExecutionControlCommands, request ExecutionControlRequest) (string, string, error) {
	if c == nil || c.controller == nil {
		return "", "", NewError(ErrorUnavailable, "execution controller unavailable", nil)
	}
	jobID := strings.TrimSpace(request.JobExecutionID)
	if jobID == "" {
		return "", "", NewError(ErrorInvalidArgument, "job execution id is required", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	return jobID, key, err
}

func executionControlFingerprint(jobID string) string {
	sum := sha256.Sum256([]byte(jobID))
	return hex.EncodeToString(sum[:])
}
