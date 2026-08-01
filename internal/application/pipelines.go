package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const runPipelineOperation = "run_pipeline"

type RunPipelineRequest struct {
	PipelineDBID   int64
	PipelineJobID  string
	MatrixName     string
	MatrixIndex    *int
	DryRun         bool
	SourceRef      string
	AgentID        string
	ExecutionMode  string
	IdempotencyKey string
}

type RunPipelineResult struct {
	ProjectName     string   `json:"project_name"`
	PipelineID      string   `json:"pipeline_id"`
	Enqueued        int      `json:"enqueued"`
	JobExecutionIDs []string `json:"job_execution_ids"`
}

type PipelineRunner interface {
	RunPipeline(context.Context, RunPipelineRequest) (RunPipelineResult, error)
}

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

type commandErrorReceipt struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

type PipelineCommands struct {
	runner   PipelineRunner
	receipts CommandReceiptRepository
	changes  *ChangeHub
}

func NewPipelineCommands(runner PipelineRunner, receipts CommandReceiptRepository, changes *ChangeHub) *PipelineCommands {
	return &PipelineCommands{runner: runner, receipts: receipts, changes: changes}
}

func (c *PipelineCommands) RunPipeline(ctx context.Context, request RunPipelineRequest) (RunPipelineResult, error) {
	if request.PipelineDBID <= 0 {
		return RunPipelineResult{}, NewError(ErrorInvalidArgument, "pipeline id must be positive", nil)
	}
	if c == nil || c.runner == nil {
		return RunPipelineResult{}, NewError(ErrorUnavailable, "pipeline runner unavailable", nil)
	}
	key := strings.TrimSpace(request.IdempotencyKey)
	if len(key) > 200 {
		return RunPipelineResult{}, NewError(ErrorInvalidArgument, "idempotency key exceeds 200 characters", nil)
	}
	if key == "" || c.receipts == nil {
		return c.run(ctx, request)
	}
	fingerprint, err := pipelineRequestFingerprint(request)
	if err != nil {
		return RunPipelineResult{}, WrapInternal("fingerprint pipeline request", err)
	}
	receipt, claimed, err := c.receipts.Claim(ctx, key, runPipelineOperation, fingerprint)
	if err != nil {
		return RunPipelineResult{}, WrapInternal("claim command receipt", err)
	}
	if !claimed {
		if receipt.Operation != runPipelineOperation || receipt.Fingerprint != fingerprint {
			return RunPipelineResult{}, NewError(ErrorConflict, "idempotency key was already used for a different command", nil)
		}
		if receipt.Status != "completed" {
			if receipt.Status == "failed" {
				var failed commandErrorReceipt
				if err := json.Unmarshal(receipt.Result, &failed); err != nil {
					return RunPipelineResult{}, WrapInternal("decode failed command receipt", err)
				}
				return RunPipelineResult{}, NewError(failed.Kind, failed.Message, nil)
			}
			return RunPipelineResult{}, NewError(ErrorUnavailable, "the original command is still pending or its outcome is unknown", nil)
		}
		var result RunPipelineResult
		if err := json.Unmarshal(receipt.Result, &result); err != nil {
			return RunPipelineResult{}, WrapInternal("decode command receipt", err)
		}
		return result, nil
	}

	result, err := c.run(ctx, request)
	if err != nil {
		failed, encodeErr := json.Marshal(commandErrorReceipt{Kind: ErrorKindOf(err), Message: err.Error()})
		if encodeErr != nil {
			return RunPipelineResult{}, WrapInternal("encode failed command receipt", encodeErr)
		}
		if receiptErr := c.receipts.Fail(context.Background(), key, failed); receiptErr != nil {
			return RunPipelineResult{}, WrapInternal("persist failed command receipt", receiptErr)
		}
		return RunPipelineResult{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return RunPipelineResult{}, WrapInternal("encode command receipt", err)
	}
	if err := c.receipts.Complete(context.Background(), key, encoded); err != nil {
		return RunPipelineResult{}, WrapInternal("complete command receipt", err)
	}
	return result, nil
}

func (c *PipelineCommands) run(ctx context.Context, request RunPipelineRequest) (RunPipelineResult, error) {
	result, err := c.runner.RunPipeline(ctx, request)
	if err != nil {
		if ErrorKindOf(err) != ErrorInternal {
			return RunPipelineResult{}, err
		}
		return RunPipelineResult{}, NewError(ErrorInvalidArgument, err.Error(), err)
	}
	if c.changes != nil {
		c.changes.Publish(ChangeQueue)
	}
	return result, nil
}

func pipelineRequestFingerprint(request RunPipelineRequest) (string, error) {
	request.IdempotencyKey = ""
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
