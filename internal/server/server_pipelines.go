package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type resolveStepReporter func(step, status, message string)

type pipelineDependencyContext struct {
	VersionRaw        string
	Version           string
	SourceRepo        string
	SourceRefRaw      string
	SourceRefResolved string
	ArtifactJobIDs    map[string]string
	ArtifactJobIDsAll map[string][]string
}

type pipelineRunContext struct {
	VersionRaw        string
	Version           string
	SourceRefRaw      string
	SourceRefResolved string
	VersionFile       string
	TagPrefix         string
	AutoBump          string
}

func (s *stateStore) pipelineByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/pipelines/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || (parts[1] != "run-selection" && parts[1] != "version-resolve" && parts[1] != "source-refs") {
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
		s.pipelineSourceRefsHandler(w, p)
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
	resp, err := s.enqueuePersistedPipeline(p, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *stateStore) pipelineChainByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/pipeline-chains/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || (parts[1] != "run" && parts[1] != "source-refs") {
		http.NotFound(w, r)
		return
	}
	chainDBID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || chainDBID <= 0 {
		http.Error(w, "invalid pipeline chain id", http.StatusBadRequest)
		return
	}
	ch, err := s.pipelineStore().GetPipelineChainByDBID(chainDBID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if len(ch.Pipelines) == 0 {
		http.Error(w, "pipeline chain has no pipelines", http.StatusBadRequest)
		return
	}
	if parts[1] == "source-refs" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.pipelineChainSourceRefsHandler(w, ch)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.RunPipelineSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	resp, err := s.enqueuePersistedPipelineChain(ch, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *stateStore) enqueuePersistedPipelineChain(ch store.PersistedPipelineChain, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	if len(ch.Pipelines) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline chain has no pipelines")
	}
	pipelines := make([]store.PersistedPipeline, 0, len(ch.Pipelines))
	for _, pid := range ch.Pipelines {
		p, err := s.pipelineStore().GetPipelineByProjectAndID(ch.ProjectName, strings.TrimSpace(pid))
		if err != nil {
			return protocol.RunPipelineResponse{}, fmt.Errorf("load pipeline %q in chain %q: %w", pid, ch.ChainID, err)
		}
		pipelines = append(pipelines, p)
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
	if firstRun.SourceRefResolved == "" && overrideSourceRef != "" && shouldApplySourceRefOverride(firstVersionPipeline.SourceRepo, overrideRepo) {
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
		meta := map[string]string{
			"chain_run_id":            chainRunID,
			"pipeline_chain_id":       ch.ChainID,
			"pipeline_chain_index":    strconv.Itoa(i),
			"pipeline_chain_position": strconv.Itoa(i + 1),
			"pipeline_chain_total":    strconv.Itoa(total),
		}
		if len(chainDeps) > 0 {
			meta["chain_depends_on_pipelines"] = strings.Join(chainDeps, ",")
		}
		opts := enqueuePipelineOptions{
			metaPatch:             meta,
			blocked:               len(chainDeps) > 0,
			sourceRefOverride:     overrideSourceRef,
			sourceRefOverrideRepo: overrideRepo,
		}
		if i > 0 {
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

	for _, pp := range prepared {
		jobIDs, err := s.persistPendingJobs(pp.pending)
		if err != nil {
			return protocol.RunPipelineResponse{}, err
		}
		allJobIDs = append(allJobIDs, jobIDs...)
	}
	if selection != nil && len(allJobIDs) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("selection matched no matrix entries")
	}
	return protocol.RunPipelineResponse{
		ProjectName:     ch.ProjectName,
		PipelineID:      ch.ChainID,
		Enqueued:        len(allJobIDs),
		JobExecutionIDs: allJobIDs,
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
