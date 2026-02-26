package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type enqueuePipelineOptions struct {
	forcedDep              *pipelineDependencyContext
	forcedRun              *pipelineRunContext
	metaPatch              map[string]string
	blocked                bool
	allowSelectionNeedsGap bool
	sourceRefOverride      string
	sourceRefOverrideRepo  string
}

type pendingJob struct {
	pipelineJobID  string
	needs          []string
	script         string
	env            map[string]string
	requiredCaps   map[string]string
	timeoutSeconds int
	artifactGlobs  []string
	caches         []protocol.JobCacheSpec
	sourceRepo     string
	sourceRef      string
	metadata       map[string]string
	stepPlan       []protocol.JobStepPlanItem
}

func (s *stateStore) enqueuePersistedPipeline(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	if normalizeExecutionMode(selection) == executionModeOfflineCached {
		return s.enqueuePersistedPipelineOfflineCached(p, selection)
	}
	opts := enqueuePipelineOptions{}
	if sourceRef := normalizeSourceRef(selection); sourceRef != "" {
		if strings.TrimSpace(p.SourceRepo) == "" {
			return protocol.RunPipelineResponse{}, fmt.Errorf("source_ref override requires pipeline vcs_source.repo")
		}
		opts.sourceRefOverride = sourceRef
		opts.sourceRefOverrideRepo = strings.TrimSpace(p.SourceRepo)
	}
	return s.enqueuePersistedPipelineWithOptions(p, selection, opts)
}

func (s *stateStore) preparePendingPipelineJobs(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest, opts enqueuePipelineOptions) (pipelineRunContext, []pendingJob, error) {
	overrideSourceRef := strings.TrimSpace(opts.sourceRefOverride)
	if overrideSourceRef != "" && shouldApplySourceRefOverride(p.SourceRepo, opts.sourceRefOverrideRepo) {
		if strings.TrimSpace(p.SourceRepo) == "" {
			return pipelineRunContext{}, nil, fmt.Errorf("source_ref override requires pipeline vcs_source.repo")
		}
		p.SourceRef = overrideSourceRef
	}
	depCtx := pipelineDependencyContext{}
	if opts.forcedDep != nil {
		depCtx = *opts.forcedDep
	} else {
		var err error
		depCtx, err = s.checkPipelineDependenciesWithReporter(p, nil)
		if err != nil {
			return pipelineRunContext{}, nil, err
		}
	}
	runCtx := pipelineRunContext{}
	if opts.forcedRun != nil {
		runCtx = *opts.forcedRun
	} else {
		var err error
		runCtx, err = resolvePipelineRunContextWithReporter(p, depCtx, nil)
		if err != nil {
			return pipelineRunContext{}, nil, err
		}
	}
	if runCtx.SourceRefResolved == "" && overrideSourceRef != "" && shouldApplySourceRefOverride(p.SourceRepo, opts.sourceRefOverrideRepo) {
		resolved, err := resolveSourceRefFromRepo(strings.TrimSpace(p.SourceRepo), strings.TrimSpace(p.SourceRef))
		if err != nil {
			return pipelineRunContext{}, nil, err
		}
		runCtx.SourceRefResolved = resolved
	}
	runID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	pending, err := s.buildPendingPipelineJobs(p, selection, opts, runCtx, depCtx, runID)
	if err != nil {
		return pipelineRunContext{}, nil, err
	}
	if runCtx.AutoBump != "" && selection != nil && selection.DryRun {
		// Explicitly skip auto bump script in dry-run mode.
		runCtx.AutoBump = ""
	}
	if runCtx.AutoBump != "" {
		if len(pending) != 1 {
			return pipelineRunContext{}, nil, fmt.Errorf("versioning.auto_bump requires exactly one job execution in the pipeline run")
		}
		if strings.TrimSpace(runCtx.AutoBumpVCSToken) == "" {
			return pipelineRunContext{}, nil, fmt.Errorf("versioning.auto_bump_vcs_token is required when auto_bump is set")
		}
		autoBumpScript := buildAutoBumpStepScript(runCtx.AutoBump)
		autoBumpEnv := map[string]string{"GITHUB_TOKEN": strings.TrimSpace(runCtx.AutoBumpVCSToken)}
		autoBumpSecrets := append([]protocol.ProjectSecretSpec(nil), runCtx.AutoBumpSecrets...)
		pending[0].script = pending[0].script + "\n" + autoBumpScript
		pending[0].stepPlan = append(pending[0].stepPlan, protocol.JobStepPlanItem{
			Index:           len(pending[0].stepPlan) + 1,
			Total:           len(pending[0].stepPlan) + 1,
			Name:            "auto bump",
			Script:          autoBumpScript,
			Env:             autoBumpEnv,
			VaultConnection: strings.TrimSpace(runCtx.AutoBumpVaultConn),
			VaultSecrets:    autoBumpSecrets,
		})
		if next := buildAutoBumpNextVersion(runCtx.VersionRaw, runCtx.AutoBump); next != "" {
			pending[0].metadata["next_version"] = next
		}
		if branch := deriveAutoBumpBranch(strings.TrimSpace(p.SourceRef)); branch != "" {
			pending[0].metadata["auto_bump_branch"] = branch
		}
		for i := range pending[0].stepPlan {
			pending[0].stepPlan[i].Index = i + 1
			pending[0].stepPlan[i].Total = len(pending[0].stepPlan)
		}
	}
	return runCtx, pending, nil
}

func (s *stateStore) enqueuePersistedPipelineWithOptions(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest, opts enqueuePipelineOptions) (protocol.RunPipelineResponse, error) {
	_, pending, err := s.preparePendingPipelineJobs(p, selection, opts)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	jobIDs, err := s.persistPendingJobs(pending)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	if selection != nil && len(jobIDs) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("selection matched no matrix entries")
	}
	return protocol.RunPipelineResponse{ProjectName: p.ProjectName, PipelineID: p.PipelineID, Enqueued: len(jobIDs), JobExecutionIDs: jobIDs}, nil
}

func (s *stateStore) persistPendingJobs(pending []pendingJob) ([]string, error) {
	jobIDs := make([]string, 0)
	for _, spec := range pending {
		var source *protocol.SourceSpec
		if strings.TrimSpace(spec.sourceRepo) != "" {
			source = &protocol.SourceSpec{Repo: spec.sourceRepo, Ref: spec.sourceRef}
		}
		job, err := s.pipelineStore().CreateJobExecution(protocol.CreateJobExecutionRequest{
			Script:               spec.script,
			Env:                  cloneMap(spec.env),
			RequiredCapabilities: spec.requiredCaps,
			TimeoutSeconds:       spec.timeoutSeconds,
			ArtifactGlobs:        append([]string(nil), spec.artifactGlobs...),
			Caches:               cloneProtocolJobCaches(spec.caches),
			Source:               source,
			Metadata:             spec.metadata,
			StepPlan:             cloneJobStepPlan(spec.stepPlan),
		})
		if err != nil {
			return nil, err
		}
		jobIDs = append(jobIDs, job.ID)
	}
	return jobIDs, nil
}
