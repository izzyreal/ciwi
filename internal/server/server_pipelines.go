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
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":      false,
				"message": depErr.Error(),
			})
			return
		}
		runCtx, runErr := resolvePipelineRunContextWithReporter(p, depCtx, nil)
		if runErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":      false,
				"message": runErr.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                   true,
			"pipeline_version":     strings.TrimSpace(runCtx.Version),
			"pipeline_version_raw": strings.TrimSpace(runCtx.VersionRaw),
			"source_ref_resolved":  strings.TrimSpace(runCtx.SourceRefResolved),
			"version_file":         strings.TrimSpace(runCtx.VersionFile),
			"tag_prefix":           strings.TrimSpace(runCtx.TagPrefix),
			"auto_bump":            strings.TrimSpace(runCtx.AutoBump),
		})
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

func (s *stateStore) enqueuePersistedPipeline(p store.PersistedPipeline, selection *protocol.RunPipelineSelectionRequest) (protocol.RunPipelineResponse, error) {
	if strings.TrimSpace(p.SourceRepo) == "" {
		return protocol.RunPipelineResponse{}, fmt.Errorf("pipeline source.repo is required")
	}
	depCtx, err := s.checkPipelineDependenciesWithReporter(p, nil)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}
	runCtx, err := resolvePipelineRunContextWithReporter(p, depCtx, nil)
	if err != nil {
		return protocol.RunPipelineResponse{}, err
	}

	jobIDs := make([]string, 0)
	runID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	type pendingJob struct {
		script         string
		env            map[string]string
		requiredCaps   map[string]string
		timeoutSeconds int
		artifactGlobs  []string
		sourceRepo     string
		sourceRef      string
		metadata       map[string]string
	}
	pending := make([]pendingJob, 0)
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
			renderVars := cloneMap(vars)
			if renderVars == nil {
				renderVars = map[string]string{}
			}
			if runCtx.VersionRaw != "" {
				renderVars["ciwi.version_raw"] = runCtx.VersionRaw
			}
			if runCtx.Version != "" {
				renderVars["ciwi.version"] = runCtx.Version
			}
			if runCtx.TagPrefix != "" {
				renderVars["ciwi.tag_prefix"] = runCtx.TagPrefix
			}
			rendered := make([]string, 0, len(pj.Steps)*3)
			env := make(map[string]string)
			for idx, step := range pj.Steps {
				for k, v := range step.Env {
					env[k] = renderTemplate(v, renderVars)
				}
				if step.Test != nil {
					command := renderTemplate(step.Test.Command, renderVars)
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
				line := renderTemplate(step.Run, renderVars)
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
				metadata["build_target"] = name
			}
			if runCtx.VersionRaw != "" {
				metadata["pipeline_version_raw"] = runCtx.VersionRaw
			}
			if runCtx.Version != "" {
				metadata["pipeline_version"] = runCtx.Version
				metadata["build_version"] = runCtx.Version
			}
			if runCtx.SourceRefResolved != "" {
				metadata["pipeline_source_ref_resolved"] = runCtx.SourceRefResolved
			}
			metadata["pipeline_source_repo"] = p.SourceRepo
			if selection != nil && selection.DryRun {
				env["CIWI_DRY_RUN"] = "1"
			}
			if runCtx.VersionRaw != "" {
				env["CIWI_PIPELINE_VERSION_RAW"] = runCtx.VersionRaw
			}
			if runCtx.Version != "" {
				env["CIWI_PIPELINE_VERSION"] = runCtx.Version
				env["CIWI_PIPELINE_TAG"] = runCtx.Version
			}
			if runCtx.TagPrefix != "" {
				env["CIWI_PIPELINE_TAG_PREFIX"] = runCtx.TagPrefix
			}
			if runCtx.VersionFile != "" {
				env["CIWI_PIPELINE_VERSION_FILE"] = runCtx.VersionFile
			}
			if runCtx.SourceRefResolved != "" {
				env["CIWI_PIPELINE_SOURCE_REF"] = runCtx.SourceRefResolved
			}
			env["CIWI_PIPELINE_SOURCE_REPO"] = p.SourceRepo

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

			sourceRef := p.SourceRef
			if runCtx.SourceRefResolved != "" {
				sourceRef = runCtx.SourceRefResolved
			}
			pending = append(pending, pendingJob{
				script:         strings.Join(rendered, "\n"),
				env:            cloneMap(env),
				requiredCaps:   requiredCaps,
				timeoutSeconds: pj.TimeoutSeconds,
				artifactGlobs:  append([]string(nil), pj.Artifacts...),
				sourceRepo:     p.SourceRepo,
				sourceRef:      sourceRef,
				metadata:       metadata,
			})
		}
	}
	if runCtx.AutoBump != "" && selection != nil && selection.DryRun {
		// Explicitly skip auto bump script in dry-run mode.
		runCtx.AutoBump = ""
	}
	if runCtx.AutoBump != "" {
		if len(pending) != 1 {
			return protocol.RunPipelineResponse{}, fmt.Errorf("versioning.auto_bump requires exactly one job execution in the pipeline run")
		}
		pending[0].script = pending[0].script + "\n" + buildAutoBumpStepScript(runCtx.AutoBump)
	}
	for _, spec := range pending {
		job, err := s.db.CreateJob(protocol.CreateJobRequest{
			Script:               spec.script,
			Env:                  cloneMap(spec.env),
			RequiredCapabilities: spec.requiredCaps,
			TimeoutSeconds:       spec.timeoutSeconds,
			ArtifactGlobs:        append([]string(nil), spec.artifactGlobs...),
			Source:               &protocol.SourceSpec{Repo: spec.sourceRepo, Ref: spec.sourceRef},
			Metadata:             spec.metadata,
		})
		if err != nil {
			return protocol.RunPipelineResponse{}, err
		}
		jobIDs = append(jobIDs, job.ID)
	}

	if selection != nil && len(jobIDs) == 0 {
		return protocol.RunPipelineResponse{}, fmt.Errorf("selection matched no matrix entries")
	}

	return protocol.RunPipelineResponse{ProjectName: p.ProjectName, PipelineID: p.PipelineID, Enqueued: len(jobIDs), JobIDs: jobIDs}, nil
}

func (s *stateStore) checkPipelineDependenciesWithReporter(p store.PersistedPipeline, report resolveStepReporter) (pipelineDependencyContext, error) {
	if len(p.DependsOn) == 0 {
		if report != nil {
			report("dependencies", "ok", "no dependencies declared")
		}
		return pipelineDependencyContext{}, nil
	}
	if report != nil {
		report("dependencies", "running", fmt.Sprintf("checking %d dependency pipeline(s)", len(p.DependsOn)))
	}
	jobs, err := s.db.ListJobs()
	if err != nil {
		if report != nil {
			report("dependencies", "error", "failed to read job history: "+err.Error())
		}
		return pipelineDependencyContext{}, fmt.Errorf("check dependencies: %w", err)
	}
	out := pipelineDependencyContext{}
	for _, depID := range p.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		if report != nil {
			report("dependencies", "running", fmt.Sprintf("checking latest run for dependency %q", depID))
		}
		ctx, err := verifyDependencyRun(jobs, p.ProjectName, depID)
		if err != nil {
			if report != nil {
				report("dependencies", "error", fmt.Sprintf("dependency %q not satisfied: %v", depID, err))
			}
			return pipelineDependencyContext{}, fmt.Errorf("pipeline %q dependency %q not satisfied: %w", p.PipelineID, depID, err)
		}
		if ctx.Version != "" {
			if out.Version != "" && out.Version != ctx.Version {
				return pipelineDependencyContext{}, fmt.Errorf("dependency versions conflict: %q vs %q", out.Version, ctx.Version)
			}
			out.Version = ctx.Version
			out.VersionRaw = ctx.VersionRaw
		}
		if ctx.SourceRefResolved != "" {
			if out.SourceRefResolved != "" && out.SourceRefResolved != ctx.SourceRefResolved {
				return pipelineDependencyContext{}, fmt.Errorf("dependency source refs conflict: %q vs %q", out.SourceRefResolved, ctx.SourceRefResolved)
			}
			out.SourceRefResolved = ctx.SourceRefResolved
		}
	}
	if report != nil {
		if out.Version != "" {
			report("dependencies", "ok", fmt.Sprintf("dependencies satisfied; inherited version=%s", out.Version))
		} else {
			report("dependencies", "ok", "dependencies satisfied")
		}
	}
	return out, nil
}

func (s *stateStore) checkPipelineDependencies(p store.PersistedPipeline) (pipelineDependencyContext, error) {
	return s.checkPipelineDependenciesWithReporter(p, nil)
}

func verifyDependencyRun(jobs []protocol.Job, projectName, pipelineID string) (pipelineDependencyContext, error) {
	type runState struct {
		lastCreated time.Time
		statuses    []string
		metadata    map[string]string
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
		if st.metadata == nil {
			st.metadata = map[string]string{}
		}
		for k, v := range j.Metadata {
			if _, exists := st.metadata[k]; !exists && strings.TrimSpace(v) != "" {
				st.metadata[k] = v
			}
		}
		byRun[runID] = st
	}
	if len(byRun) == 0 {
		return pipelineDependencyContext{}, fmt.Errorf("no previous run found")
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
			return pipelineDependencyContext{}, fmt.Errorf("latest run is still in progress")
		}
		if st == "failed" {
			return pipelineDependencyContext{}, fmt.Errorf("latest run failed")
		}
	}
	meta := byRun[latestRunID].metadata
	return pipelineDependencyContext{
		VersionRaw:        strings.TrimSpace(meta["pipeline_version_raw"]),
		Version:           strings.TrimSpace(meta["pipeline_version"]),
		SourceRefResolved: strings.TrimSpace(meta["pipeline_source_ref_resolved"]),
	}, nil
}

func (s *stateStore) streamVersionResolve(w http.ResponseWriter, p store.PersistedPipeline) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(step, status, message string, extra map[string]any) {
		payload := map[string]any{
			"step":    step,
			"status":  status,
			"message": message,
		}
		for k, v := range extra {
			payload[k] = v
		}
		b, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
		flusher.Flush()
	}

	send("start", "running", fmt.Sprintf("resolving version for pipeline %q", p.PipelineID), nil)
	depCtx, depErr := s.checkPipelineDependenciesWithReporter(p, func(step, status, message string) {
		send(step, status, message, nil)
	})
	if depErr != nil {
		send("done", "error", depErr.Error(), nil)
		return
	}
	runCtx, runErr := resolvePipelineRunContextWithReporter(p, depCtx, func(step, status, message string) {
		send(step, status, message, nil)
	})
	if runErr != nil {
		send("done", "error", runErr.Error(), nil)
		return
	}
	send("done", "ok", "version resolution completed", map[string]any{
		"pipeline_version":     strings.TrimSpace(runCtx.Version),
		"pipeline_version_raw": strings.TrimSpace(runCtx.VersionRaw),
		"source_ref_resolved":  strings.TrimSpace(runCtx.SourceRefResolved),
		"version_file":         strings.TrimSpace(runCtx.VersionFile),
		"tag_prefix":           strings.TrimSpace(runCtx.TagPrefix),
		"auto_bump":            strings.TrimSpace(runCtx.AutoBump),
	})
}
