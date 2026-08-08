package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type resolveStepReporter func(step, status, message string)

type pipelineRunResponse struct {
	protocol.RunPipelineResponse
	Notice presentation.TransientNotice `json:"notice"`
}

type pipelineDependencyContext struct {
	VersionRaw         string
	Version            string
	SourceRepo         string
	SourceRefRaw       string
	SourceRefResolved  string
	ArtifactExecutions map[string][]dependencyArtifactExecution
}

type dependencyArtifactExecution struct {
	ID          string
	Pipeline    string
	Job         string
	MatrixIndex int
	Matrix      map[string]string
}

type pipelineRunContext struct {
	VersionRaw        string
	Version           string
	SourceRefRaw      string
	SourceRefResolved string
	VersionFile       string
	TagPrefix         string
	AutoBump          string
	AutoBumpVCSToken  string
	AutoBumpVaultConn string
	AutoBumpSecrets   []protocol.ProjectSecretSpec
}

func (s *stateStore) pipelineByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/pipelines/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || (parts[1] != "run-selection" && parts[1] != "version-resolve" && parts[1] != "source-refs" && parts[1] != "eligible-agents" && parts[1] != "dry-run-preview") {
		http.NotFound(w, r)
		return
	}
	pipelineDBID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || pipelineDBID <= 0 {
		http.Error(w, "invalid pipeline id", http.StatusBadRequest)
		return
	}
	p, err := s.pipelineStore().GetPipelineByDBID(pipelineDBID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if parts[1] == "version-resolve" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.streamVersionResolve(w, p)
		return
	}
	if parts[1] == "source-refs" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.pipelineSourceRefsHandler(r.Context(), w, p)
		return
	}
	if parts[1] == "eligible-agents" {
		s.pipelineEligibleAgentsHandler(w, p, r)
		return
	}
	if parts[1] == "dry-run-preview" {
		s.pipelineDryRunPreviewHandler(w, p, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.RunPipelineSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	resp, err := s.app().pipelines.RunPipeline(r.Context(), application.RunPipelineRequest{
		PipelineDBID:   pipelineDBID,
		PipelineJobID:  req.PipelineJobID,
		MatrixName:     req.MatrixName,
		MatrixIndex:    req.MatrixIndex,
		DryRun:         req.DryRun,
		SourceRef:      req.SourceRef,
		AgentID:        req.AgentID,
		ExecutionMode:  req.ExecutionMode,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
		return
	}
	result := pipelineRunResponse{
		RunPipelineResponse: protocol.RunPipelineResponse{
			ProjectName: resp.ProjectName, PipelineID: resp.PipelineID, Enqueued: resp.Enqueued,
			JobExecutionIDs: append([]string(nil), resp.JobExecutionIDs...),
		},
	}
	result.Notice = presentation.QueuedPipelineNotice(
		resp.ProjectName, resp.PipelineID, req.PipelineJobID, resp.Enqueued, req.DryRun, resp.JobExecutionIDs,
	)
	writeJSON(w, http.StatusCreated, result)
}

func (s *stateStore) pipelineChainActionHandler(w http.ResponseWriter, r *http.Request, projectID int64, chainID, action string) {
	chainID = strings.TrimSpace(chainID)
	if chainID == "" || (action != "run" && action != "source-refs" && action != "eligible-agents" && action != "dry-run-preview") {
		http.NotFound(w, r)
		return
	}
	if action == "run" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req protocol.RunPipelineSelectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		resp, err := s.app().pipelineChains.RunPipelineChain(r.Context(), application.RunPipelineChainRequest{
			ProjectID: projectID, ChainID: chainID, PipelineJobID: req.PipelineJobID, MatrixName: req.MatrixName,
			MatrixIndex: req.MatrixIndex, DryRun: req.DryRun, SourceRef: req.SourceRef, AgentID: req.AgentID,
			ExecutionMode: req.ExecutionMode, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		})
		if err != nil {
			http.Error(w, err.Error(), applicationErrorHTTPStatus(err))
			return
		}
		result := pipelineRunResponse{
			RunPipelineResponse: protocol.RunPipelineResponse{
				ProjectName: resp.ProjectName, PipelineChainID: resp.ChainID, PipelineChainName: resp.ChainName, Enqueued: resp.Enqueued,
				JobExecutionIDs: append([]string(nil), resp.JobExecutionIDs...),
			},
		}
		result.Notice = presentation.QueuedChainNotice(resp.ProjectName, resp.ChainName, resp.Enqueued, req.DryRun)
		writeJSON(w, http.StatusCreated, result)
		return
	}
	ch, err := s.pipelineStore().GetPipelineChain(projectID, chainID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if len(ch.Pipelines) == 0 {
		http.Error(w, "pipeline chain has no pipelines", http.StatusBadRequest)
		return
	}
	if action == "source-refs" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.pipelineChainSourceRefsHandler(r.Context(), w, ch)
		return
	}
	if action == "eligible-agents" {
		s.pipelineChainEligibleAgentsHandler(w, ch, r)
		return
	}
	if action == "dry-run-preview" {
		s.pipelineChainDryRunPreviewHandler(w, ch, r)
		return
	}
}

func (s *stateStore) enqueuePersistedPipelineChain(ch store.PersistedPipelineChain, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	if normalizeExecutionMode(selection) == executionModeOfflineCached {
		return s.enqueuePersistedPipelineChainOfflineCached(ch, selection)
	}
	if len(ch.Pipelines) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline chain has no pipelines")
	}
	pipelines, err := s.loadPipelineChainPipelines(ch)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	chainRunID := fmt.Sprintf("chain-%d", time.Now().UTC().UnixNano())
	firstDep, err := s.checkPipelineDependenciesWithReporter(pipelines[0], nil)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	overrideSourceRef := normalizeSourceRef(selection)
	overrideRepo := strings.TrimSpace(pipelines[0].SourceRepo)
	if overrideSourceRef != "" && overrideRepo == "" {
		return protocol.RunPipelineResponse{}, fmt.Errorf("source_ref override requires first chain pipeline vcs_source.repo")
	}
	firstVersionPipeline := pipelines[0]
	if overrideSourceRef != "" && shouldApplySourceRefOverride(firstVersionPipeline.SourceRepo, overrideRepo) {
		firstVersionPipeline.SourceRef = overrideSourceRef
	}
	firstRun, err := resolvePipelineRunContextWithReporter(firstVersionPipeline, firstDep, nil)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	if firstRun.SourceRefResolved == "" && strings.TrimSpace(firstVersionPipeline.SourceRepo) != "" {
		resolved, err := resolveSourceRefFromRepo(strings.TrimSpace(firstVersionPipeline.SourceRepo), strings.TrimSpace(firstVersionPipeline.SourceRef))
		if err != nil {
			return protocol.RunPipelineResponse{}, err
		}
		firstRun.SourceRefResolved = resolved
	}
	allJobIDs := make([]string, 0)
	total := len(pipelines)
	chainPipelineSet := map[string]struct{}{}
	for _, p := range pipelines {
		chainPipelineSet[strings.TrimSpace(p.PipelineID)] = struct{}{}
	}
	type chainPreparedPipeline struct {
		pipeline store.PersistedPipeline
		pending  []pendingJob
	}
	prepared := make([]chainPreparedPipeline, 0, len(pipelines))

	for i, p := range pipelines {
		prevPipelineID := ""
		if i > 0 {
			prevPipelineID = strings.TrimSpace(pipelines[i-1].PipelineID)
		}
		chainDeps := deriveChainPipelineDependencies(p, chainPipelineSet, prevPipelineID)
		meta := domain.ExecutionMetadata{
			domain.ExecutionMetadataChainRunID:            chainRunID,
			domain.ExecutionMetadataPipelineChainID:       ch.ChainID,
			domain.ExecutionMetadataPipelineChainName:     ch.ChainName,
			domain.ExecutionMetadataPipelineChainIndex:    strconv.Itoa(i),
			domain.ExecutionMetadataPipelineChainPosition: strconv.Itoa(i + 1),
			domain.ExecutionMetadataPipelineChainTotal:    strconv.Itoa(total),
		}
		if len(chainDeps) > 0 {
			meta.Set(domain.ExecutionMetadataChainDependsOnPipelines, strings.Join(chainDeps, ","))
		}
		opts := enqueuePipelineOptions{
			metaPatch:             meta,
			blocked:               len(chainDeps) > 0,
			sourceRefOverride:     overrideSourceRef,
			sourceRefOverrideRepo: overrideRepo,
		}
		if i == 0 {
			opts.forcedDep = &firstDep
			opts.forcedRun = &firstRun
		} else {
			opts.forcedDep = &pipelineDependencyContext{
				VersionRaw:        firstRun.VersionRaw,
				Version:           firstRun.Version,
				SourceRepo:        strings.TrimSpace(pipelines[0].SourceRepo),
				SourceRefRaw:      firstRun.SourceRefRaw,
				SourceRefResolved: firstRun.SourceRefResolved,
			}
		}
		_, pending, err := s.preparePendingPipelineJobs(p, selection, opts)
		if err != nil {
			return protocol.RunPipelineResponse{}, err
		}
		prepared = append(prepared, chainPreparedPipeline{
			pipeline: p,
			pending:  pending,
		})
	}

	allPending := make([]pendingJob, 0)
	for _, pp := range prepared {
		allPending = append(allPending, pp.pending...)
	}
	allJobIDs, err = s.persistPendingJobs(allPending)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	if selection != nil && len(allJobIDs) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("selection matched no matrix entries")
	}
	return protocol.RunPipelineResponse{
		ProjectName:       ch.ProjectName,
		PipelineChainID:   ch.ChainID,
		PipelineChainName: ch.ChainName,
		Enqueued:          len(allJobIDs),
		JobExecutionIDs:   allJobIDs,
	}, nil
}

func deriveChainPipelineDependencies(p store.PersistedPipeline, chainPipelineSet map[string]struct{}, fallbackPrev string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, dep := range p.DependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		if _, ok := chainPipelineSet[dep]; !ok {
			continue
		}
		if _, dup := seen[dep]; dup {
			continue
		}
		seen[dep] = struct{}{}
		out = append(out, dep)
	}
	if len(out) > 0 {
		return out
	}
	fallbackPrev = strings.TrimSpace(fallbackPrev)
	if fallbackPrev != "" {
		return []string{fallbackPrev}
	}
	return nil
}
