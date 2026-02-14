package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

type resolveStepReporter func(step, status, message string)

type pipelineDependencyContext struct {
	VersionRaw        string
	Version           string
	SourceRefResolved string
	ArtifactJobIDs    map[string]string
	ArtifactJobIDsAll map[string][]string
}

type pipelineRunContext struct {
	VersionRaw        string
	Version           string
	SourceRefResolved string
	VersionFile       string
	TagPrefix         string
	AutoBump          string
}

func (s *stateStore) runPipelineFromConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req protocol.RunPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ConfigPath == "" {
		req.ConfigPath = "ciwi.yaml"
	}
	if req.PipelineID == "" {
		http.Error(w, "pipeline_id is required", http.StatusBadRequest)
		return
	}

	fullPath, err := resolveConfigPath(req.ConfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := config.Load(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.pipelineStore().LoadConfig(cfg, fullPath, "", "", filepath.Base(fullPath)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p, err := s.pipelineStore().GetPipelineByProjectAndID(cfg.Project.Name, req.PipelineID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	resp, err := s.enqueuePersistedPipeline(p, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *stateStore) pipelineByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/pipelines/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || (parts[1] != "run" && parts[1] != "run-selection" && parts[1] != "version-preview" && parts[1] != "version-resolve") {
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
	if parts[1] == "version-preview" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		depCtx, depErr := s.checkPipelineDependenciesWithReporter(p, nil)
		if depErr != nil {
			writeJSON(w, http.StatusOK, buildPipelineVersionPreviewErrorResponse(depErr.Error()))
			return
		}
		runCtx, runErr := resolvePipelineRunContextWithReporter(p, depCtx, nil)
		if runErr != nil {
			writeJSON(w, http.StatusOK, buildPipelineVersionPreviewErrorResponse(runErr.Error()))
			return
		}
		writeJSON(w, http.StatusOK, buildPipelineVersionPreviewSuccessResponse(runCtx))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.RunPipelineSelectionRequest
	if parts[1] == "run" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		resp, err := s.enqueuePersistedPipeline(p, &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, resp)
		return
	}

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
	if len(parts) != 2 || (parts[1] != "run" && parts[1] != "version-preview" && parts[1] != "version-resolve") {
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
	first, err := s.pipelineStore().GetPipelineByProjectAndID(ch.ProjectName, strings.TrimSpace(ch.Pipelines[0]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "version-resolve":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.streamVersionResolve(w, first)
		return
	case "version-preview":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		depCtx, depErr := s.checkPipelineDependenciesWithReporter(first, nil)
		if depErr != nil {
			writeJSON(w, http.StatusOK, buildPipelineVersionPreviewErrorResponse(depErr.Error()))
			return
		}
		runCtx, runErr := resolvePipelineRunContextWithReporter(first, depCtx, nil)
		if runErr != nil {
			writeJSON(w, http.StatusOK, buildPipelineVersionPreviewErrorResponse(runErr.Error()))
			return
		}
		writeJSON(w, http.StatusOK, buildPipelineVersionPreviewSuccessResponse(runCtx))
		return
	default:
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
	firstRun, err := resolvePipelineRunContextWithReporter(pipelines[0], firstDep, nil)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	allJobIDs := make([]string, 0)
	total := len(pipelines)
	for i, p := range pipelines {
		meta := map[string]string{
			"chain_run_id":            chainRunID,
			"pipeline_chain_id":       ch.ChainID,
			"pipeline_chain_index":    strconv.Itoa(i),
			"pipeline_chain_position": strconv.Itoa(i + 1),
			"pipeline_chain_total":    strconv.Itoa(total),
		}
		opts := enqueuePipelineOptions{
			metaPatch: meta,
			blocked:   i > 0,
		}
		if i > 0 {
			opts.forcedDep = &pipelineDependencyContext{
				VersionRaw:        firstRun.VersionRaw,
				Version:           firstRun.Version,
				SourceRefResolved: firstRun.SourceRefResolved,
			}
		}
		resp, err := s.enqueuePersistedPipelineWithOptions(p, selection, opts)
		if err != nil {
			return protocol.RunPipelineResponse{}, err
		}
		allJobIDs = append(allJobIDs, resp.JobExecutionIDs...)
	}
	return protocol.RunPipelineResponse{
		ProjectName:     ch.ProjectName,
		PipelineID:      ch.ChainID,
		Enqueued:        len(allJobIDs),
		JobExecutionIDs: allJobIDs,
	}, nil
}
