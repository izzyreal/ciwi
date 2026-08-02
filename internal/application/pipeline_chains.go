package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const runPipelineChainOperation = "run_pipeline_chain"

type RunPipelineChainRequest struct {
	ProjectID      int64
	ChainID        string
	PipelineJobID  string
	MatrixName     string
	MatrixIndex    *int
	DryRun         bool
	SourceRef      string
	AgentID        string
	ExecutionMode  string
	IdempotencyKey string
}

type RunPipelineChainResult struct {
	ProjectName     string   `json:"project_name"`
	ChainID         string   `json:"chain_id"`
	ChainName       string   `json:"chain_name"`
	Enqueued        int      `json:"enqueued"`
	JobExecutionIDs []string `json:"job_execution_ids"`
}

type PipelineChainRunner interface {
	RunPipelineChain(context.Context, RunPipelineChainRequest) (RunPipelineChainResult, error)
}

type PipelineChainCommands struct {
	runner   PipelineChainRunner
	receipts CommandReceiptRepository
	changes  *ChangeHub
}

func NewPipelineChainCommands(runner PipelineChainRunner, receipts CommandReceiptRepository, changes *ChangeHub) *PipelineChainCommands {
	return &PipelineChainCommands{runner: runner, receipts: receipts, changes: changes}
}

func (c *PipelineChainCommands) RunPipelineChain(ctx context.Context, request RunPipelineChainRequest) (RunPipelineChainResult, error) {
	if request.ProjectID <= 0 {
		return RunPipelineChainResult{}, NewError(ErrorInvalidArgument, "project id must be positive", nil)
	}
	request.ChainID = strings.TrimSpace(request.ChainID)
	if request.ChainID == "" {
		return RunPipelineChainResult{}, NewError(ErrorInvalidArgument, "pipeline chain id is required", nil)
	}
	if c == nil || c.runner == nil {
		return RunPipelineChainResult{}, NewError(ErrorUnavailable, "pipeline chain runner unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return RunPipelineChainResult{}, err
	}
	fingerprint, err := pipelineChainRequestFingerprint(request)
	if err != nil {
		return RunPipelineChainResult{}, WrapInternal("fingerprint pipeline chain request", err)
	}
	execute := func() (RunPipelineChainResult, error) {
		result, err := c.runner.RunPipelineChain(ctx, request)
		if err != nil {
			if ErrorKindOf(err) != ErrorInternal {
				return RunPipelineChainResult{}, err
			}
			return RunPipelineChainResult{}, NewError(ErrorInvalidArgument, err.Error(), err)
		}
		if c.changes != nil {
			c.changes.Publish(ChangeQueue)
		}
		return result, nil
	}
	return executeIdempotentCommand(ctx, c.receipts, key, runPipelineChainOperation, fingerprint, execute)
}

func pipelineChainRequestFingerprint(request RunPipelineChainRequest) (string, error) {
	request.IdempotencyKey = ""
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
