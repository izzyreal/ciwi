package server

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

const executionModeOfflineCached = "offline_cached"

var commitSHARe = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func normalizeExecutionMode(selection *protocol.RunPipelineSelectionRequest) string {
	if selection == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(selection.ExecutionMode))
}

func (s *stateStore) enqueuePersistedPipelineOfflineCached(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	pending, err := s.preparePendingPipelineJobsOfflineCached(p, selection)
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
	return protocol.RunPipelineResponse{ProjectName: displayProjectName(p.ProjectName), PipelineID: p.PipelineID, Enqueued: len(jobIDs), JobExecutionIDs: jobIDs}, nil
}

func (s *stateStore) preparePendingPipelineJobsOfflineCached(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest) ([]pendingJob, error) {
	depCtx, err := s.checkPipelineDependenciesWithReporter(p, nil)
	if err != nil {
		return nil, err
	}
	runCtx, cacheUsed, _, _, err := s.resolveCachedPreviewRunContext(p, depCtx, selection)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.SourceRepo) != "" && !cacheUsed {
		return nil, fmt.Errorf("offline_cached execution requires cached source context for pipeline %q", p.PipelineID)
	}
	pCopy := p
	if strings.TrimSpace(runCtx.SourceRefRaw) != "" {
		pCopy.SourceRef = strings.TrimSpace(runCtx.SourceRefRaw)
	}
	runID := fmt.Sprintf("offline-%d", time.Now().UTC().UnixNano())
	pending, err := s.buildPendingPipelineJobs(
		pCopy,
		selection,
		enqueuePipelineOptions{forcedDep: &depCtx, forcedRun: &runCtx},
		runCtx,
		depCtx,
		runID,
	)
	if err != nil {
		return nil, err
	}
	if err := validateOfflinePendingJobsSafety(pCopy, pending, selection); err != nil {
		return nil, err
	}
	return pending, nil
}

func (s *stateStore) enqueuePersistedPipelineChainOfflineCached(ch store.PersistedPipelineChain, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	pending, err := s.preparePendingPipelineChainJobsOfflineCached(ch, selection)
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
	return protocol.RunPipelineResponse{
		ProjectName:     displayProjectName(ch.ProjectName),
		PipelineID:      ch.ChainID,
		Enqueued:        len(jobIDs),
		JobExecutionIDs: jobIDs,
	}, nil
}

func (s *stateStore) preparePendingPipelineChainJobsOfflineCached(ch store.PersistedPipelineChain, selection *protocol.RunPipelineSelectionRequest) ([]pendingJob, error) {
	if len(ch.Pipelines) == 0 {
		return nil, fmt.Errorf("pipeline chain has no pipelines")
	}
	pipelines := make([]store.PersistedPipeline, 0, len(ch.Pipelines))
	for _, pid := range ch.Pipelines {
		p, err := s.pipelineStore().GetPipelineByProjectAndID(ch.ProjectName, strings.TrimSpace(pid))
		if err != nil {
			return nil, fmt.Errorf("load pipeline %q in chain %q: %w", pid, ch.ChainID, err)
		}
		pipelines = append(pipelines, p)
	}
	firstDep, err := s.checkPipelineDependenciesWithReporter(pipelines[0], nil)
	if err != nil {
		return nil, err
	}
	firstRun, used, _, _, err := s.resolveCachedPreviewRunContext(pipelines[0], firstDep, selection)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(pipelines[0].SourceRepo) != "" && !used {
		return nil, fmt.Errorf("offline_cached execution requires cached source context for first chain pipeline %q", pipelines[0].PipelineID)
	}
	firstRepo := strings.TrimSpace(pipelines[0].SourceRepo)
	for _, p := range pipelines[1:] {
		if strings.TrimSpace(p.SourceRepo) != "" && !sameSourceRepo(firstRepo, p.SourceRepo) {
			return nil, fmt.Errorf("offline_cached chain execution currently requires same source repo across chain pipelines; %q differs", p.PipelineID)
		}
	}

	total := len(pipelines)
	chainPipelineSet := map[string]struct{}{}
	for _, p := range pipelines {
		chainPipelineSet[strings.TrimSpace(p.PipelineID)] = struct{}{}
	}
	all := make([]pendingJob, 0)
	for i, p := range pipelines {
		prevPipelineID := ""
		if i > 0 {
			prevPipelineID = strings.TrimSpace(pipelines[i-1].PipelineID)
		}
		chainDeps := deriveChainPipelineDependencies(p, chainPipelineSet, prevPipelineID)
		meta := map[string]string{
			"chain_run_id":            fmt.Sprintf("offline-chain-%d", time.Now().UTC().UnixNano()),
			"pipeline_chain_id":       ch.ChainID,
			"pipeline_chain_index":    fmt.Sprintf("%d", i),
			"pipeline_chain_position": fmt.Sprintf("%d", i+1),
			"pipeline_chain_total":    fmt.Sprintf("%d", total),
		}
		if len(chainDeps) > 0 {
			meta["chain_depends_on_pipelines"] = strings.Join(chainDeps, ",")
		}
		opts := enqueuePipelineOptions{
			metaPatch: meta,
			blocked:   len(chainDeps) > 0,
		}
		dep := firstDep
		run := firstRun
		if i > 0 {
			dep = pipelineDependencyContext{
				VersionRaw:        firstRun.VersionRaw,
				Version:           firstRun.Version,
				SourceRepo:        strings.TrimSpace(pipelines[0].SourceRepo),
				SourceRefRaw:      firstRun.SourceRefRaw,
				SourceRefResolved: firstRun.SourceRefResolved,
			}
		}
		pCopy := p
		if strings.TrimSpace(run.SourceRefRaw) != "" {
			pCopy.SourceRef = strings.TrimSpace(run.SourceRefRaw)
		}
		pending, err := s.buildPendingPipelineJobs(pCopy, selection, opts, run, dep, fmt.Sprintf("offline-chain-%d-%d", time.Now().UTC().UnixNano(), i))
		if err != nil {
			return nil, err
		}
		if err := validateOfflinePendingJobsSafety(pCopy, pending, selection); err != nil {
			return nil, err
		}
		all = append(all, pending...)
	}
	if selection != nil && len(all) == 0 {
		return nil, fmt.Errorf("selection matched no matrix entries")
	}
	return all, nil
}

func validateOfflinePendingJobsSafety(p store.PersistedPipeline, pending []pendingJob, selection *protocol.RunPipelineSelectionRequest) error {
	jobSkipDryRun := map[string]bool{}
	for _, j := range p.SortedJobs() {
		for _, step := range j.Steps {
			if step.SkipDryRun {
				jobSkipDryRun[strings.TrimSpace(j.ID)] = true
				break
			}
		}
	}
	dryRun := selection != nil && selection.DryRun
	for _, spec := range pending {
		if strings.TrimSpace(spec.sourceRepo) != "" {
			ref := strings.TrimSpace(spec.sourceRef)
			if !commitSHARe.MatchString(ref) {
				return fmt.Errorf("offline_cached execution requires pinned cached source commit for job %q; got ref=%q", spec.pipelineJobID, ref)
			}
		}
		if !dryRun && jobSkipDryRun[strings.TrimSpace(spec.pipelineJobID)] {
			return fmt.Errorf("offline_cached execution blocks job %q because it contains skip_dry_run step(s); run as dry_run or split wet steps", spec.pipelineJobID)
		}
	}
	return nil
}
