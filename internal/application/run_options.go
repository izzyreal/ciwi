package application

import (
	"context"
	"strings"
)

const (
	RunTargetPipeline = "pipeline"
	RunTargetChain    = "chain"
)

type RunOptionsRequest struct {
	PipelineDBID           int64
	ProjectID              int64
	ChainID                string
	PipelineJobID          string
	MatrixName             string
	MatrixIndex            *int
	DryRun                 bool
	SourceRef              string
	AgentID                string
	ExecutionMode          string
	IncludeSourceRefs      bool
	IncludeEligibleAgents  bool
	AllowMissingSourceRepo bool
}

type RunOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type RunOptions struct {
	TargetKind        string      `json:"target_kind"`
	TargetLabel       string      `json:"target_label"`
	PipelineDBID      int64       `json:"pipeline_db_id,omitempty"`
	ProjectID         int64       `json:"project_id,omitempty"`
	ChainID           string      `json:"chain_id,omitempty"`
	SupportsDryRun    bool        `json:"supports_dry_run"`
	SourceRepo        string      `json:"source_repo"`
	DefaultSourceRef  string      `json:"default_source_ref,omitempty"`
	SelectedSourceRef string      `json:"selected_source_ref,omitempty"`
	SelectedAgentID   string      `json:"selected_agent_id,omitempty"`
	SourceRefs        []RunOption `json:"source_refs"`
	EligibleAgents    []RunOption `json:"eligible_agents"`
	PendingJobs       int         `json:"pending_jobs"`
}

type RunOptionsProvider interface {
	GetRunOptions(context.Context, RunOptionsRequest) (RunOptions, error)
}

type RunOptionsQueries struct {
	provider RunOptionsProvider
}

func NewRunOptionsQueries(provider RunOptionsProvider) *RunOptionsQueries {
	return &RunOptionsQueries{provider: provider}
}

func (q *RunOptionsQueries) GetRunOptions(ctx context.Context, request RunOptionsRequest) (RunOptions, error) {
	request.ChainID = strings.TrimSpace(request.ChainID)
	pipelineTarget := request.PipelineDBID > 0
	chainTarget := request.ProjectID > 0 || request.ChainID != ""
	if pipelineTarget == chainTarget {
		return RunOptions{}, NewError(ErrorInvalidArgument, "select exactly one pipeline or pipeline chain", nil)
	}
	if chainTarget && (request.ProjectID <= 0 || request.ChainID == "") {
		return RunOptions{}, NewError(ErrorInvalidArgument, "pipeline chain requires a project id and chain id", nil)
	}
	if q == nil || q.provider == nil {
		return RunOptions{}, NewError(ErrorUnavailable, "run options provider unavailable", nil)
	}
	if !request.IncludeSourceRefs && !request.IncludeEligibleAgents {
		request.IncludeSourceRefs = true
		request.IncludeEligibleAgents = true
	}
	result, err := q.provider.GetRunOptions(ctx, request)
	if err != nil {
		return RunOptions{}, err
	}
	result.SelectedSourceRef = strings.TrimSpace(request.SourceRef)
	if result.SelectedSourceRef == "" {
		result.SelectedSourceRef = result.DefaultSourceRef
	}
	result.SelectedAgentID = strings.TrimSpace(request.AgentID)
	return result, nil
}
