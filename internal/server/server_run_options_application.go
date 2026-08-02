package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type runOptionsAdapter struct {
	state *stateStore
}

func (a runOptionsAdapter) GetRunOptions(ctx context.Context, request application.RunOptionsRequest) (application.RunOptions, error) {
	if err := ctx.Err(); err != nil {
		return application.RunOptions{}, err
	}
	selection := &protocol.RunPipelineSelectionRequest{
		PipelineJobID: request.PipelineJobID, MatrixName: request.MatrixName, MatrixIndex: request.MatrixIndex,
		DryRun: request.DryRun, SourceRef: request.SourceRef, AgentID: request.AgentID, ExecutionMode: request.ExecutionMode,
	}
	if request.PipelineDBID > 0 {
		pipeline, err := a.state.pipelineStore().GetPipelineByDBID(request.PipelineDBID)
		if err != nil {
			return application.RunOptions{}, application.NewError(application.ErrorNotFound, err.Error(), err)
		}
		return a.pipelineOptions(ctx, pipeline, selection, request)
	}
	chain, err := a.state.pipelineStore().GetPipelineChain(request.ProjectID, request.ChainID)
	if err != nil {
		return application.RunOptions{}, application.NewError(application.ErrorNotFound, err.Error(), err)
	}
	return a.chainOptions(ctx, chain, selection, request)
}

func (a runOptionsAdapter) pipelineOptions(ctx context.Context, pipeline store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest, request application.RunOptionsRequest) (application.RunOptions, error) {
	result := application.RunOptions{
		TargetKind: application.RunTargetPipeline, TargetLabel: pipeline.PipelineID, PipelineDBID: pipeline.DBID,
		ProjectID: pipeline.ProjectID, SupportsDryRun: persistedPipelineSupportsDryRun(pipeline),
		SourceRepo: strings.TrimSpace(pipeline.SourceRepo),
	}
	if request.IncludeSourceRefs {
		if result.SourceRepo == "" && request.AllowMissingSourceRepo {
			request.IncludeSourceRefs = false
		}
	}
	if request.IncludeSourceRefs {
		refs, err := buildSourceRefsViewContext(ctx, result.SourceRepo, strings.TrimSpace(pipeline.SourceRef))
		if err != nil {
			return application.RunOptions{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
		}
		result.DefaultSourceRef = refs.DefaultRef
		result.SourceRefs = sourceRefRunOptions(refs.Refs)
	}
	if request.IncludeEligibleAgents {
		_, pending, err := a.state.preparePendingPipelineJobs(pipeline, selection, enqueuePipelineOptions{
			allowSelectionNeedsGap: true, allowUnsatisfiedDeps: true,
		})
		if err != nil {
			return application.RunOptions{}, application.NewError(application.ErrorInvalidArgument, err.Error(), err)
		}
		result.EligibleAgents = agentRunOptions(a.state.eligibleAgentsForPendingJobs(pending))
		result.PendingJobs = len(pending)
	}
	return result, nil
}

func (a runOptionsAdapter) chainOptions(ctx context.Context, chain store.PersistedPipelineChain, selection *protocol.RunPipelineSelectionRequest, request application.RunOptionsRequest) (application.RunOptions, error) {
	if len(chain.Pipelines) == 0 {
		return application.RunOptions{}, application.NewError(application.ErrorInvalidArgument, "pipeline chain has no pipelines", nil)
	}
	firstID := strings.TrimSpace(chain.Pipelines[0])
	first, err := a.state.pipelineStore().GetPipelineByProjectIDAndID(chain.ProjectID, firstID)
	if err != nil {
		message := fmt.Sprintf("load pipeline %q in chain %q: %v", firstID, chain.ChainName, err)
		return application.RunOptions{}, application.NewError(application.ErrorInvalidArgument, message, err)
	}
	supportsDryRun := false
	for _, pipelineID := range chain.Pipelines {
		pipeline, loadErr := a.state.pipelineStore().GetPipelineByProjectIDAndID(chain.ProjectID, pipelineID)
		if loadErr != nil {
			return application.RunOptions{}, application.NewError(application.ErrorInvalidArgument, loadErr.Error(), loadErr)
		}
		supportsDryRun = supportsDryRun || persistedPipelineSupportsDryRun(pipeline)
	}
	label := strings.TrimSpace(chain.ChainName)
	if label == "" {
		label = chain.ChainID
	}
	result := application.RunOptions{
		TargetKind: application.RunTargetChain, TargetLabel: label, ProjectID: chain.ProjectID, ChainID: chain.ChainID,
		SupportsDryRun: supportsDryRun, SourceRepo: strings.TrimSpace(first.SourceRepo),
	}
	if request.IncludeSourceRefs {
		if result.SourceRepo == "" && request.AllowMissingSourceRepo {
			request.IncludeSourceRefs = false
		}
	}
	if request.IncludeSourceRefs {
		refs, refsErr := buildSourceRefsViewContext(ctx, result.SourceRepo, strings.TrimSpace(first.SourceRef))
		if refsErr != nil {
			return application.RunOptions{}, application.NewError(application.ErrorInvalidArgument, refsErr.Error(), refsErr)
		}
		result.DefaultSourceRef = refs.DefaultRef
		result.SourceRefs = sourceRefRunOptions(refs.Refs)
	}
	if request.IncludeEligibleAgents {
		pending, pendingErr := a.state.preparePendingPipelineChainJobs(chain, selection)
		if pendingErr != nil {
			return application.RunOptions{}, application.NewError(application.ErrorInvalidArgument, pendingErr.Error(), pendingErr)
		}
		result.EligibleAgents = agentRunOptions(a.state.eligibleAgentsForPendingJobs(pending))
		result.PendingJobs = len(pending)
	}
	return result, nil
}

func persistedPipelineSupportsDryRun(pipeline store.PersistedPipeline) bool {
	for _, job := range pipeline.Jobs {
		for _, step := range job.Steps {
			if step.SkipDryRun {
				return true
			}
		}
	}
	return false
}

func sourceRefRunOptions(refs []sourceRefOptionView) []application.RunOption {
	result := make([]application.RunOption, 0, len(refs))
	for _, ref := range refs {
		result = append(result, application.RunOption{Value: ref.Ref, Label: ref.Name})
	}
	return result
}

func agentRunOptions(agentIDs []string) []application.RunOption {
	result := []application.RunOption{{Value: "", Label: "Any eligible agent"}}
	for _, agentID := range agentIDs {
		result = append(result, application.RunOption{Value: agentID, Label: agentID})
	}
	return result
}
