package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/config"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

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

func (s *stateStore) enqueuePersistedPipeline(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	if strings.TrimSpace(p.SourceRepo) == "" {
		return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline source.repo is required")
	}
	if err := s.checkPipelineDependencies(p); err != nil {
		return protocol.RunPipelineResponse{}, err
	}

	jobIDs := make([]string, 0)
	runID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
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
			rendered := make([]string, 0, len(pj.Steps)*3)
			env := make(map[string]string)
			for idx, step := range pj.Steps {
				for k, v := range step.Env {
					env[k] = renderTemplate(v, vars)
				}
				if step.Test != nil {
					command := renderTemplate(step.Test.Command, vars)
					if strings.TrimSpace(command) == "" {
						continue
					}
					name := strings.TrimSpace(step.Test.Name)
					if name == "" {
						name = fmt.Sprintf("%s-test-%d", pj.ID, idx+1)
					}
					format := strings.TrimSpace(step.Test.Format)
					if format == "" {
						format = "go-test-json"
					}
					rendered = append(rendered,
						fmt.Sprintf("echo \"__CIWI_TEST_BEGIN__ name=%s format=%s\"", sanitizeMarkerToken(name), sanitizeMarkerToken(format)),
						command,
						`echo "__CIWI_TEST_END__"`,
					)
					continue
				}
				line := renderTemplate(step.Run, vars)
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
				"pipeline_run_id":    runID,
				"pipeline_job_id":    pj.ID,
				"pipeline_job_index": strconv.Itoa(index),
			}
			if selection != nil && selection.DryRun {
				metadata["dry_run"] = "1"
			}
			if name := vars["name"]; name != "" {
				metadata["matrix_name"] = name
			}
			if selection != nil && selection.DryRun {
				env["CIWI_DRY_RUN"] = "1"
			}
			requiredCaps := cloneMap(pj.RunsOn)
			for tool, constraint := range pj.RequiresTools {
				tool = strings.TrimSpace(tool)
				if tool == "" {
					continue
				}
				if requiredCaps == nil {
					requiredCaps = map[string]string{}
				}
				requiredCaps["requires.tool."+tool] = strings.TrimSpace(constraint)
			}

			job, err := s.db.CreateJob(protocol.CreateJobRequest{
				Script:               strings.Join(rendered, "\n"),
				Env:                  cloneMap(env),
				RequiredCapabilities: requiredCaps,
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

func (s *stateStore) checkPipelineDependencies(p store.PersistedPipeline) error {
	if len(p.DependsOn) == 0 {
		return nil
	}
	jobs, err := s.db.ListJobs()
	if err != nil {
		return fmt.Errorf("check dependencies: %w", err)
	}
	for _, depID := range p.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		if err := verifyDependencyRun(jobs, p.ProjectName, depID); err != nil {
			return fmt.Errorf("pipeline %q dependency %q not satisfied: %w", p.PipelineID, depID, err)
		}
	}
	return nil
}

func verifyDependencyRun(jobs []protocol.Job, projectName, pipelineID string) error {
	type runState struct {
		lastCreated time.Time
		statuses    []string
	}
	byRun := map[string]runState{}
	for _, j := range jobs {
		if strings.TrimSpace(j.Metadata["project"]) != projectName {
			continue
		}
		if strings.TrimSpace(j.Metadata["pipeline_id"]) != pipelineID {
			continue
		}
		runID := strings.TrimSpace(j.Metadata["pipeline_run_id"])
		if runID == "" {
			runID = j.ID
		}
		st := byRun[runID]
		if j.CreatedUTC.After(st.lastCreated) {
			st.lastCreated = j.CreatedUTC
		}
		st.statuses = append(st.statuses, strings.ToLower(strings.TrimSpace(j.Status)))
		byRun[runID] = st
	}
	if len(byRun) == 0 {
		return fmt.Errorf("no previous run found")
	}

	latestRunID := ""
	latest := time.Time{}
	for runID, st := range byRun {
		if latestRunID == "" || st.lastCreated.After(latest) {
			latestRunID = runID
			latest = st.lastCreated
		}
	}
	statuses := byRun[latestRunID].statuses
	for _, st := range statuses {
		if st == "queued" || st == "leased" || st == "running" {
			return fmt.Errorf("latest run is still in progress")
		}
		if st == "failed" {
			return fmt.Errorf("latest run failed")
		}
	}
	return nil
}
