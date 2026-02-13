package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type resolveStepReporter func(step, status, message string)

type pipelineDependencyContext struct {
	VersionRaw        string
	Version           string
	SourceRefResolved string
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
	if err := s.db.LoadConfig(cfg, fullPath, "", "", filepath.Base(fullPath)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p, err := s.db.GetPipelineByProjectAndID(cfg.Project.Name, req.PipelineID)
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
	p, err := s.db.GetPipelineByDBID(pipelineDBID)
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
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
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
