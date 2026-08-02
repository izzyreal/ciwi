package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return RunPipelineResult{}, err
	}
	if key == "" || c.receipts == nil {
		return c.run(ctx, request)
	}
	fingerprint, err := pipelineRequestFingerprint(request)
	if err != nil {
		return RunPipelineResult{}, WrapInternal("fingerprint pipeline request", err)
	}
	return executeIdempotentCommand(ctx, c.receipts, key, runPipelineOperation, fingerprint, func() (RunPipelineResult, error) {
		return c.run(ctx, request)
	})
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
