package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

const (
	jobStatusRunning   = "running"
	jobStatusSucceeded = "succeeded"
	jobStatusFailed    = "failed"
)

type agentState struct {
	Hostname     string            `json:"hostname"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Capabilities map[string]string `json:"capabilities"`
	LastSeenUTC  time.Time         `json:"last_seen_utc"`
}

type stateStore struct {
	mu           sync.Mutex
	agents       map[string]agentState
	db           *store.Store
	artifactsDir string
}

func Run(ctx context.Context) error {
	addr := envOrDefault("CIWI_SERVER_ADDR", ":8080")
	dbPath := envOrDefault("CIWI_DB_PATH", "ciwi.db")
	artifactsDir := envOrDefault("CIWI_ARTIFACTS_DIR", "ciwi-artifacts")

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}

	s := &stateStore{agents: make(map[string]agentState), db: db, artifactsDir: artifactsDir}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.uiHandler)
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/api/v1/heartbeat", s.heartbeatHandler)
	mux.HandleFunc("/api/v1/agents", s.listAgentsHandler)
	mux.HandleFunc("/api/v1/config/load", s.loadConfigHandler)
	mux.HandleFunc("/api/v1/projects/import", s.importProjectHandler)
	mux.HandleFunc("/api/v1/projects", s.listProjectsHandler)
	mux.HandleFunc("/api/v1/projects/", s.projectByIDHandler)
	mux.HandleFunc("/api/v1/jobs", s.jobsHandler)
	mux.HandleFunc("/api/v1/jobs/", s.jobByIDHandler)
	mux.HandleFunc("/api/v1/jobs/clear-queue", s.clearQueueHandler)
	mux.HandleFunc("/api/v1/jobs/flush-history", s.flushHistoryHandler)
	mux.HandleFunc("/api/v1/agent/lease", s.leaseJobHandler)
	mux.HandleFunc("/api/v1/pipelines/run", s.runPipelineFromConfigHandler)
	mux.HandleFunc("/api/v1/pipelines/", s.pipelineByIDHandler)
	mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", http.FileServer(http.Dir(artifactsDir))))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("ciwi server started on %s", addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		log.Println("ciwi server stopped")
		return nil
	case err := <-errCh:
		if err != nil {
			return err
		}
		log.Println("ciwi server stopped")
		return nil
	}
}

func (s *stateStore) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var hb protocol.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if hb.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	if hb.TimestampUTC.IsZero() {
		hb.TimestampUTC = time.Now().UTC()
	}

	s.mu.Lock()
	s.agents[hb.AgentID] = agentState{
		Hostname:     hb.Hostname,
		OS:           hb.OS,
		Arch:         hb.Arch,
		Capabilities: hb.Capabilities,
		LastSeenUTC:  hb.TimestampUTC,
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, protocol.HeartbeatResponse{Accepted: true})
}

func (s *stateStore) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	agents := make([]protocol.AgentInfo, 0, len(s.agents))
	for id, a := range s.agents {
		agents = append(agents, protocol.AgentInfo{AgentID: id, Hostname: a.Hostname, OS: a.OS, Arch: a.Arch, Capabilities: a.Capabilities, LastSeenUTC: a.LastSeenUTC})
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *stateStore) loadConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.LoadConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ConfigPath == "" {
		req.ConfigPath = "ciwi.yaml"
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

	writeJSON(w, http.StatusOK, protocol.LoadConfigResponse{ProjectName: cfg.Project.Name, ConfigPath: fullPath, Pipelines: len(cfg.Pipelines)})
}

func (s *stateStore) importProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.ImportProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		http.Error(w, "repo_url is required", http.StatusBadRequest)
		return
	}
	if req.ConfigFile == "" {
		req.ConfigFile = "ciwi-project.yaml"
	}
	configFile := filepath.Clean(req.ConfigFile)
	if configFile == "." || configFile == "" || filepath.Base(configFile) != configFile {
		http.Error(w, "config_file must point to a root-level file", http.StatusBadRequest)
		return
	}
	req.ConfigFile = configFile
	if _, err := exec.LookPath("git"); err != nil {
		http.Error(w, "git not found on server", http.StatusInternalServerError)
		return
	}

	tmpDir, err := os.MkdirTemp("", "ciwi-import-*")
	if err != nil {
		http.Error(w, fmt.Sprintf("create temp dir: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	importCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	cfgContent, err := fetchConfigFileFromRepo(importCtx, tmpDir, req.RepoURL, req.RepoRef, req.ConfigFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.persistImportedProject(req, cfgContent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *stateStore) projectByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	projectID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || projectID <= 0 {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		detail, err := s.db.GetProjectDetail(projectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"project": detail})
		return
	}

	if len(parts) != 2 || parts[1] != "reload" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	project, err := s.db.GetProjectByID(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if strings.TrimSpace(project.RepoURL) == "" {
		http.Error(w, "project has no repo_url configured", http.StatusBadRequest)
		return
	}
	configFile := project.ConfigFile
	if configFile == "" {
		configFile = "ciwi-project.yaml"
	}

	tmpDir, err := os.MkdirTemp("", "ciwi-reload-*")
	if err != nil {
		http.Error(w, fmt.Sprintf("create temp dir: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	reloadCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	cfgContent, err := fetchConfigFileFromRepo(reloadCtx, tmpDir, project.RepoURL, project.RepoRef, configFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.persistImportedProject(protocol.ImportProjectRequest{
		RepoURL:    project.RepoURL,
		RepoRef:    project.RepoRef,
		ConfigFile: configFile,
	}, cfgContent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func fetchConfigFileFromRepo(ctx context.Context, tmpDir, repoURL, repoRef, configFile string) (string, error) {
	if out, err := runCmd(ctx, "", "git", "init", "-q", tmpDir); err != nil {
		return "", fmt.Errorf("git init failed: %v\n%s", err, out)
	}
	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "remote", "add", "origin", repoURL); err != nil {
		return "", fmt.Errorf("git remote add failed: %v\n%s", err, out)
	}

	ref := strings.TrimSpace(repoRef)
	if ref == "" {
		ref = "HEAD"
	}

	if out, err := runCmd(ctx, "", "git", "-C", tmpDir, "fetch", "-q", "--depth", "1", "origin", ref); err != nil {
		return "", fmt.Errorf("git fetch failed: %v\n%s", err, out)
	}

	out, err := runCmd(ctx, "", "git", "-C", tmpDir, "show", "FETCH_HEAD:"+configFile)
	if err != nil {
		return "", fmt.Errorf("repo is not a valid ciwi project: missing root file %q", configFile)
	}

	return out, nil
}

func (s *stateStore) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projects, err := s.db.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *stateStore) jobsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs, err := s.db.ListJobs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	case http.MethodPost:
		var req protocol.CreateJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		job, err := s.db.CreateJob(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, protocol.CreateJobResponse{Job: job})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *stateStore) jobByIDHandler(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/"), "/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rel, "/")
	jobID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			job, err := s.db.GetJob(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"job": job})
		case http.MethodDelete:
			err := s.db.DeleteQueuedJob(jobID)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "job_id": jobID})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "status" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req protocol.JobStatusUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.AgentID == "" {
			http.Error(w, "agent_id is required", http.StatusBadRequest)
			return
		}
		if !isValidUpdateStatus(req.Status) {
			http.Error(w, "status must be running, succeeded or failed", http.StatusBadRequest)
			return
		}
		job, err := s.db.UpdateJobStatus(jobID, req)
		if err != nil {
			if strings.Contains(err.Error(), "another agent") {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job": job})
		return
	}

	if len(parts) == 2 && parts[1] == "artifacts" {
		switch r.Method {
		case http.MethodGet:
			artifacts, err := s.db.ListJobArtifacts(jobID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for i := range artifacts {
				artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
			}
			writeJSON(w, http.StatusOK, protocol.JobArtifactsResponse{Artifacts: artifacts})
		case http.MethodPost:
			var req protocol.UploadArtifactsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			if req.AgentID == "" {
				http.Error(w, "agent_id is required", http.StatusBadRequest)
				return
			}
			job, err := s.db.GetJob(jobID)
			if err != nil {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			if job.LeasedByAgentID != "" && job.LeasedByAgentID != req.AgentID {
				http.Error(w, "job is leased by another agent", http.StatusConflict)
				return
			}

			artifacts, err := s.persistArtifacts(jobID, req.Artifacts)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := s.db.SaveJobArtifacts(jobID, artifacts); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for i := range artifacts {
				artifacts[i].URL = "/artifacts/" + strings.TrimPrefix(filepath.ToSlash(artifacts[i].URL), "/")
			}
			writeJSON(w, http.StatusOK, protocol.JobArtifactsResponse{Artifacts: artifacts})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *stateStore) clearQueueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.db.ClearQueuedJobs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

func (s *stateStore) flushHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.db.FlushJobHistory()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flushed": n})
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
	if len(parts) != 2 || (parts[1] != "run" && parts[1] != "run-selection") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	if parts[1] == "run" {
		resp, err := s.enqueuePersistedPipeline(p, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, resp)
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

func (s *stateStore) enqueuePersistedPipeline(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	if strings.TrimSpace(p.SourceRepo) == "" {
		return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline source.repo is required")
	}

	jobIDs := make([]string, 0)
	for _, pj := range p.SortedJobs() {
		if selection != nil && strings.TrimSpace(selection.PipelineJobID) != "" && selection.PipelineJobID != pj.ID {
			continue
		}
		if len(pj.Steps) == 0 {
			return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline job %q has no steps", pj.ID)
		}
		matrixEntries := pj.MatrixInclude
		if len(matrixEntries) == 0 {
			matrixEntries = []map[string]string{{}}
		}

		for index, vars := range matrixEntries {
			if selection != nil {
				if selection.MatrixIndex != nil && *selection.MatrixIndex != index {
					continue
				}
				if strings.TrimSpace(selection.MatrixName) != "" && vars["name"] != selection.MatrixName {
					continue
				}
			}
			rendered := make([]string, 0, len(pj.Steps))
			for _, step := range pj.Steps {
				line := renderTemplate(step, vars)
				if strings.TrimSpace(line) == "" {
					continue
				}
				rendered = append(rendered, line)
			}
			if len(rendered) == 0 {
				return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline job %q rendered empty script", pj.ID)
			}

			metadata := map[string]string{
				"project":            p.ProjectName,
				"pipeline_id":        p.PipelineID,
				"pipeline_job_id":    pj.ID,
				"pipeline_job_index": strconv.Itoa(index),
			}
			if name := vars["name"]; name != "" {
				metadata["matrix_name"] = name
			}

			job, err := s.db.CreateJob(protocol.CreateJobRequest{
				Script:               strings.Join(rendered, "\n"),
				RequiredCapabilities: cloneMap(pj.RunsOn),
				TimeoutSeconds:       pj.TimeoutSeconds,
				ArtifactGlobs:        append([]string(nil), pj.Artifacts...),
				Source:               &protocol.SourceSpec{Repo: p.SourceRepo, Ref: p.SourceRef},
				Metadata:             metadata,
			})
			if err != nil {
				return protocol.RunPipelineResponse{}, err
			}
			jobIDs = append(jobIDs, job.ID)
		}
	}

	if selection != nil && len(jobIDs) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("selection matched no matrix entries")
	}

	return protocol.RunPipelineResponse{ProjectName: p.ProjectName, PipelineID: p.PipelineID, Enqueued: len(jobIDs), JobIDs: jobIDs}, nil
}

func (s *stateStore) leaseJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.LeaseJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	agentCaps := req.Capabilities
	s.mu.Lock()
	if a, ok := s.agents[req.AgentID]; ok {
		agentCaps = mergeCapabilities(a, req.Capabilities)
	}
	s.mu.Unlock()

	job, err := s.db.LeaseJob(req.AgentID, agentCaps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: false, Message: "no matching queued job"})
		return
	}
	writeJSON(w, http.StatusOK, protocol.LeaseJobResponse{Assigned: true, Job: job})
}

func isValidUpdateStatus(status string) bool {
	switch status {
	case jobStatusRunning, jobStatusSucceeded, jobStatusFailed:
		return true
	default:
		return false
	}
}

func resolveConfigPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("config_path must be relative")
	}
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		return "", fmt.Errorf("config_path is invalid")
	}
	if strings.HasPrefix(cleanPath, "..") {
		return "", fmt.Errorf("config_path must stay within working directory")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Join(cwd, cleanPath), nil
}

func renderTemplate(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeCapabilities(agent agentState, override map[string]string) map[string]string {
	merged := map[string]string{"os": agent.OS, "arch": agent.Arch}
	for k, v := range agent.Capabilities {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode JSON response: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *stateStore) persistArtifacts(jobID string, incoming []protocol.UploadArtifact) ([]protocol.JobArtifact, error) {
	if len(incoming) == 0 {
		return nil, nil
	}
	base := filepath.Join(s.artifactsDir, jobID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}

	artifacts := make([]protocol.JobArtifact, 0, len(incoming))
	for _, in := range incoming {
		rel := filepath.ToSlash(filepath.Clean(in.Path))
		if rel == "." || rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") {
			return nil, fmt.Errorf("invalid artifact path: %q", in.Path)
		}

		decoded, err := base64.StdEncoding.DecodeString(in.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("decode artifact %q: %w", in.Path, err)
		}

		dst := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir artifact parent: %w", err)
		}
		if err := os.WriteFile(dst, decoded, 0o644); err != nil {
			return nil, fmt.Errorf("write artifact %q: %w", in.Path, err)
		}

		storedRel := filepath.ToSlash(filepath.Join(jobID, filepath.FromSlash(rel)))
		artifacts = append(artifacts, protocol.JobArtifact{
			JobID:     jobID,
			Path:      rel,
			URL:       storedRel,
			SizeBytes: int64(len(decoded)),
		})
	}
	return artifacts, nil
}

func (s *stateStore) persistImportedProject(req protocol.ImportProjectRequest, cfgContent string) (protocol.ImportProjectResponse, error) {
	cfg, err := config.Parse([]byte(cfgContent), req.ConfigFile)
	if err != nil {
		return protocol.ImportProjectResponse{}, err
	}

	for i := range cfg.Pipelines {
		if strings.TrimSpace(cfg.Pipelines[i].Source.Repo) == "" {
			cfg.Pipelines[i].Source.Repo = req.RepoURL
		}
		if strings.TrimSpace(cfg.Pipelines[i].Source.Ref) == "" {
			cfg.Pipelines[i].Source.Ref = req.RepoRef
		}
	}

	configPath := fmt.Sprintf("%s:%s", req.RepoURL, req.ConfigFile)
	if req.RepoRef != "" {
		configPath = fmt.Sprintf("%s@%s:%s", req.RepoURL, req.RepoRef, req.ConfigFile)
	}
	if err := s.db.LoadConfig(cfg, configPath, req.RepoURL, req.RepoRef, req.ConfigFile); err != nil {
		return protocol.ImportProjectResponse{}, err
	}

	return protocol.ImportProjectResponse{
		ProjectName: cfg.Project.Name,
		RepoURL:     req.RepoURL,
		RepoRef:     req.RepoRef,
		ConfigFile:  req.ConfigFile,
		Pipelines:   len(cfg.Pipelines),
	}, nil
}
