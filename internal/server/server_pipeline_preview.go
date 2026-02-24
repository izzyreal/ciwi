package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type runPreviewRequest struct {
	PipelineJobID     string `json:"pipeline_job_id,omitempty"`
	MatrixName        string `json:"matrix_name,omitempty"`
	MatrixIndex       *int   `json:"matrix_index,omitempty"`
	SourceRef         string `json:"source_ref,omitempty"`
	AgentID           string `json:"agent_id,omitempty"`
	OfflineCachedOnly bool   `json:"offline_cached_only,omitempty"`
}

type runPreviewJobView struct {
	PipelineJobID     string            `json:"pipeline_job_id"`
	MatrixName        string            `json:"matrix_name,omitempty"`
	RequiredCaps      map[string]string `json:"required_capabilities,omitempty"`
	SourceRepo        string            `json:"source_repo,omitempty"`
	SourceRef         string            `json:"source_ref,omitempty"`
	StepCount         int               `json:"step_count"`
	ArtifactGlobs     []string          `json:"artifact_globs,omitempty"`
	DependencyBlocked bool              `json:"dependency_blocked,omitempty"`
}

type runPreviewResponse struct {
	Mode              string              `json:"mode"`
	OfflineCachedOnly bool                `json:"offline_cached_only"`
	CacheUsed         bool                `json:"cache_used"`
	CacheSource       string              `json:"cache_source,omitempty"`
	PipelineID        string              `json:"pipeline_id"`
	PendingJobs       []runPreviewJobView `json:"pending_jobs"`
	EligibleAgentIDs  []string            `json:"eligible_agent_ids"`
	Warnings          []string            `json:"warnings,omitempty"`
}

func (s *stateStore) pipelineDryRunPreviewHandler(w http.ResponseWriter, p store.PersistedPipeline, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, sel, err := decodeRunPreviewRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pending, cacheUsed, cacheSource, warns, err := s.previewSinglePipelineDryRun(p, req, sel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, runPreviewResponse{
		Mode:              s.computeRuntimeState(time.Now().UTC()).Mode,
		OfflineCachedOnly: req.OfflineCachedOnly,
		CacheUsed:         cacheUsed,
		CacheSource:       cacheSource,
		PipelineID:        p.PipelineID,
		PendingJobs:       toRunPreviewJobs(pending),
		EligibleAgentIDs:  s.eligibleAgentsForPendingJobs(pending),
		Warnings:          warns,
	})
}

func (s *stateStore) pipelineChainDryRunPreviewHandler(w http.ResponseWriter, ch store.PersistedPipelineChain, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, sel, err := decodeRunPreviewRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pending, cacheUsed, cacheSource, warns, err := s.previewPipelineChainDryRun(ch, req, sel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, runPreviewResponse{
		Mode:              s.computeRuntimeState(time.Now().UTC()).Mode,
		OfflineCachedOnly: req.OfflineCachedOnly,
		CacheUsed:         cacheUsed,
		CacheSource:       cacheSource,
		PipelineID:        ch.ChainID,
		PendingJobs:       toRunPreviewJobs(pending),
		EligibleAgentIDs:  s.eligibleAgentsForPendingJobs(pending),
		Warnings:          warns,
	})
}

func decodeRunPreviewRequest(r *http.Request) (runPreviewRequest, *protocol.RunPipelineSelectionRequest, error) {
	var req runPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return runPreviewRequest{}, nil, fmt.Errorf("invalid JSON body")
	}
	sel := &protocol.RunPipelineSelectionRequest{
		PipelineJobID: strings.TrimSpace(req.PipelineJobID),
		MatrixName:    strings.TrimSpace(req.MatrixName),
		MatrixIndex:   req.MatrixIndex,
		DryRun:        true,
		SourceRef:     strings.TrimSpace(req.SourceRef),
		AgentID:       strings.TrimSpace(req.AgentID),
	}
	return req, sel, nil
}

func (s *stateStore) previewSinglePipelineDryRun(p store.PersistedPipeline, req runPreviewRequest, sel *protocol.RunPipelineSelectionRequest) ([]pendingJob, bool, string, []string, error) {
	opts := enqueuePipelineOptions{}
	if sourceRef := normalizeSourceRef(sel); sourceRef != "" {
		if strings.TrimSpace(p.SourceRepo) == "" {
			return nil, false, "", nil, fmt.Errorf("source_ref override requires pipeline vcs_source.repo")
		}
		opts.sourceRefOverride = sourceRef
		opts.sourceRefOverrideRepo = strings.TrimSpace(p.SourceRepo)
	}
	if !req.OfflineCachedOnly {
		_, pending, err := s.preparePendingPipelineJobs(p, sel, opts)
		if err != nil {
			return nil, false, "", nil, err
		}
		return pending, false, "", nil, nil
	}

	depCtx, err := s.checkPipelineDependenciesWithReporter(p, nil)
	if err != nil {
		return nil, false, "", nil, err
	}
	runCtx, used, source, warns, err := s.resolveCachedPreviewRunContext(p, depCtx, sel)
	if err != nil {
		return nil, false, "", nil, err
	}
	pCopy := p
	if strings.TrimSpace(runCtx.SourceRefRaw) != "" {
		pCopy.SourceRef = strings.TrimSpace(runCtx.SourceRefRaw)
	}
	runID := fmt.Sprintf("preview-%d", time.Now().UTC().UnixNano())
	pending, err := s.buildPendingPipelineJobs(pCopy, sel, enqueuePipelineOptions{forcedDep: &depCtx, forcedRun: &runCtx}, runCtx, depCtx, runID)
	if err != nil {
		return nil, false, "", nil, err
	}
	return pending, used, source, warns, nil
}

func (s *stateStore) previewPipelineChainDryRun(ch store.PersistedPipelineChain, req runPreviewRequest, sel *protocol.RunPipelineSelectionRequest) ([]pendingJob, bool, string, []string, error) {
	if len(ch.Pipelines) == 0 {
		return nil, false, "", nil, fmt.Errorf("pipeline chain has no pipelines")
	}
	if !req.OfflineCachedOnly {
		pending, err := s.preparePendingPipelineChainJobs(ch, sel)
		return pending, false, "", nil, err
	}
	pipelines := make([]store.PersistedPipeline, 0, len(ch.Pipelines))
	for _, pid := range ch.Pipelines {
		p, err := s.pipelineStore().GetPipelineByProjectAndID(ch.ProjectName, strings.TrimSpace(pid))
		if err != nil {
			return nil, false, "", nil, fmt.Errorf("load pipeline %q in chain %q: %w", pid, ch.ChainID, err)
		}
		pipelines = append(pipelines, p)
	}
	firstDep, err := s.checkPipelineDependenciesWithReporter(pipelines[0], nil)
	if err != nil {
		return nil, false, "", nil, err
	}
	firstCtx, used, source, warns, err := s.resolveCachedPreviewRunContext(pipelines[0], firstDep, sel)
	if err != nil {
		return nil, false, "", nil, err
	}
	out := make([]pendingJob, 0)
	total := len(pipelines)
	chainPipelineSet := map[string]struct{}{}
	for _, p := range pipelines {
		chainPipelineSet[strings.TrimSpace(p.PipelineID)] = struct{}{}
	}
	for i, p := range pipelines {
		prevPipelineID := ""
		if i > 0 {
			prevPipelineID = strings.TrimSpace(pipelines[i-1].PipelineID)
		}
		chainDeps := deriveChainPipelineDependencies(p, chainPipelineSet, prevPipelineID)
		meta := map[string]string{
			"chain_run_id":            "preview",
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
		depCtx := firstDep
		runCtx := firstCtx
		if i > 0 {
			depCtx = pipelineDependencyContext{
				VersionRaw:        firstCtx.VersionRaw,
				Version:           firstCtx.Version,
				SourceRepo:        strings.TrimSpace(pipelines[0].SourceRepo),
				SourceRefRaw:      firstCtx.SourceRefRaw,
				SourceRefResolved: firstCtx.SourceRefResolved,
			}
		}
		pCopy := p
		if strings.TrimSpace(runCtx.SourceRefRaw) != "" {
			pCopy.SourceRef = strings.TrimSpace(runCtx.SourceRefRaw)
		}
		runID := fmt.Sprintf("preview-chain-%d-%d", time.Now().UTC().UnixNano(), i)
		pending, err := s.buildPendingPipelineJobs(pCopy, sel, opts, runCtx, depCtx, runID)
		if err != nil {
			return nil, false, "", nil, err
		}
		out = append(out, pending...)
	}
	return out, used, source, warns, nil
}

func (s *stateStore) resolveCachedPreviewRunContext(p store.PersistedPipeline, depCtx pipelineDependencyContext, sel *protocol.RunPipelineSelectionRequest) (pipelineRunContext, bool, string, []string, error) {
	if depCtx.Version != "" {
		ctx, err := resolvePipelineRunContextWithReporter(p, depCtx, nil)
		if err != nil {
			return pipelineRunContext{}, false, "", nil, err
		}
		ctx.SourceRefRaw = strings.TrimSpace(p.SourceRef)
		if depCtx.SourceRefRaw != "" && sameSourceRepo(depCtx.SourceRepo, p.SourceRepo) {
			ctx.SourceRefRaw = depCtx.SourceRefRaw
		}
		if sourceRef := normalizeSourceRef(sel); sourceRef != "" {
			if !sourceRefMatchesCached(sourceRef, ctx.SourceRefRaw) {
				return pipelineRunContext{}, false, "", nil, fmt.Errorf("offline_cached_only source_ref %q does not match cached/dependency source ref %q", sourceRef, ctx.SourceRefRaw)
			}
			ctx.SourceRefRaw = sourceRef
		}
		return ctx, true, "dependency", nil, nil
	}

	jobs, err := s.pipelineStore().ListJobExecutions()
	if err != nil {
		return pipelineRunContext{}, false, "", nil, fmt.Errorf("load job history for cached preview: %w", err)
	}
	cached, err := verifyDependencyRun(jobs, p.ProjectName, p.PipelineID)
	if err != nil {
		if hasPipelineVersioning(p) {
			return pipelineRunContext{}, false, "", nil, fmt.Errorf("offline_cached_only requires a prior successful %q run: %w", p.PipelineID, err)
		}
		ctx := pipelineRunContext{SourceRefRaw: strings.TrimSpace(p.SourceRef)}
		if sourceRef := normalizeSourceRef(sel); sourceRef != "" {
			ctx.SourceRefRaw = sourceRef
		}
		return ctx, false, "", []string{"no cached successful run context; using pipeline source ref only"}, nil
	}

	ctx := pipelineRunContext{
		VersionRaw:        strings.TrimSpace(cached.VersionRaw),
		Version:           strings.TrimSpace(cached.Version),
		SourceRefRaw:      strings.TrimSpace(cached.SourceRefRaw),
		SourceRefResolved: strings.TrimSpace(cached.SourceRefResolved),
	}
	if ctx.SourceRefRaw == "" {
		ctx.SourceRefRaw = strings.TrimSpace(p.SourceRef)
	}
	if sourceRef := normalizeSourceRef(sel); sourceRef != "" {
		if !sourceRefMatchesCached(sourceRef, ctx.SourceRefRaw) {
			return pipelineRunContext{}, false, "", nil, fmt.Errorf("offline_cached_only source_ref %q does not match cached source ref %q", sourceRef, ctx.SourceRefRaw)
		}
		ctx.SourceRefRaw = sourceRef
	}
	return ctx, true, "pipeline_history", nil, nil
}

func hasPipelineVersioning(p store.PersistedPipeline) bool {
	return strings.TrimSpace(p.Versioning.File) != "" ||
		strings.TrimSpace(p.Versioning.TagPrefix) != "" ||
		strings.TrimSpace(p.Versioning.AutoBump) != ""
}

func sourceRefMatchesCached(reqRef, cachedRaw string) bool {
	reqRef = strings.TrimSpace(reqRef)
	cachedRaw = strings.TrimSpace(cachedRaw)
	if reqRef == "" || cachedRaw == "" {
		return reqRef == cachedRaw
	}
	if reqRef == cachedRaw {
		return true
	}
	if strings.HasPrefix(reqRef, "refs/heads/") && strings.TrimPrefix(reqRef, "refs/heads/") == cachedRaw {
		return true
	}
	if strings.HasPrefix(cachedRaw, "refs/heads/") && strings.TrimPrefix(cachedRaw, "refs/heads/") == reqRef {
		return true
	}
	return false
}

func toRunPreviewJobs(in []pendingJob) []runPreviewJobView {
	out := make([]runPreviewJobView, 0, len(in))
	for _, p := range in {
		out = append(out, runPreviewJobView{
			PipelineJobID:     strings.TrimSpace(p.pipelineJobID),
			MatrixName:        strings.TrimSpace(p.metadata["matrix_name"]),
			RequiredCaps:      cloneMap(p.requiredCaps),
			SourceRepo:        strings.TrimSpace(p.sourceRepo),
			SourceRef:         strings.TrimSpace(p.sourceRef),
			StepCount:         len(p.stepPlan),
			ArtifactGlobs:     append([]string(nil), p.artifactGlobs...),
			DependencyBlocked: strings.TrimSpace(p.metadata["needs_blocked"]) == "1" || strings.TrimSpace(p.metadata["chain_blocked"]) == "1",
		})
	}
	return out
}
