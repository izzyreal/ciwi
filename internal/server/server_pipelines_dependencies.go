package server

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
)

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
	jobs, err := s.pipelineStore().ListJobExecutions()
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
		ctx, err := verifyDependencyRun(jobs, p.ProjectID, depID)
		if err != nil {
			if report != nil {
				report("dependencies", "error", fmt.Sprintf("dependency %q not satisfied: %v", depID, err))
			}
			return pipelineDependencyContext{}, fmt.Errorf("pipeline %q dependency %q not satisfied: %w", p.PipelineID, depID, err)
		}
		if err := mergePipelineDependencyContext(&out, depID, ctx); err != nil {
			return pipelineDependencyContext{}, err
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

func (s *stateStore) inspectPipelineDependenciesWithReporter(p store.PersistedPipeline, report resolveStepReporter) (pipelineDependencyContext, []string, bool, error) {
	if len(p.DependsOn) == 0 {
		if report != nil {
			report("dependencies", "ok", "no dependencies declared")
		}
		return pipelineDependencyContext{}, nil, false, nil
	}
	if report != nil {
		report("dependencies", "running", fmt.Sprintf("checking %d dependency pipeline(s)", len(p.DependsOn)))
	}
	jobs, err := s.pipelineStore().ListJobExecutions()
	if err != nil {
		if report != nil {
			report("dependencies", "error", "failed to read job history: "+err.Error())
		}
		return pipelineDependencyContext{}, nil, false, fmt.Errorf("check dependencies: %w", err)
	}
	out := pipelineDependencyContext{}
	warnings := make([]string, 0)
	blocked := false
	for _, depID := range p.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		if report != nil {
			report("dependencies", "running", fmt.Sprintf("checking latest run for dependency %q", depID))
		}
		ctx, err := verifyDependencyRun(jobs, p.ProjectID, depID)
		if err != nil {
			blocked = true
			msg := fmt.Sprintf("dependency %q unresolved for preview: %v", depID, err)
			warnings = append(warnings, msg)
			if report != nil {
				report("dependencies", "warning", msg)
			}
			continue
		}
		if err := mergePipelineDependencyContext(&out, depID, ctx); err != nil {
			if report != nil {
				report("dependencies", "error", err.Error())
			}
			return pipelineDependencyContext{}, nil, false, err
		}
	}
	if report != nil {
		switch {
		case blocked && out.Version != "":
			report("dependencies", "warning", fmt.Sprintf("dependency history incomplete; preview remains blocked, inherited version=%s where available", out.Version))
		case blocked:
			report("dependencies", "warning", "dependency history incomplete; preview remains blocked")
		case out.Version != "":
			report("dependencies", "ok", fmt.Sprintf("dependencies satisfied; inherited version=%s", out.Version))
		default:
			report("dependencies", "ok", "dependencies satisfied")
		}
	}
	return out, warnings, blocked, nil
}

func mergePipelineDependencyContext(out *pipelineDependencyContext, depID string, ctx pipelineDependencyContext) error {
	if out == nil {
		return nil
	}
	if ctx.Version != "" {
		if out.Version != "" && out.Version != ctx.Version {
			return fmt.Errorf("dependency versions conflict: %q vs %q", out.Version, ctx.Version)
		}
		out.Version = ctx.Version
		out.VersionRaw = ctx.VersionRaw
	}
	if strings.TrimSpace(ctx.SourceRepo) != "" && ctx.SourceRefResolved != "" {
		if out.SourceRefResolved == "" {
			out.SourceRepo = strings.TrimSpace(ctx.SourceRepo)
			out.SourceRefResolved = ctx.SourceRefResolved
		} else if sameSourceRepo(out.SourceRepo, ctx.SourceRepo) {
			if out.SourceRefResolved != ctx.SourceRefResolved {
				return fmt.Errorf("dependency source refs conflict: %q vs %q", out.SourceRefResolved, ctx.SourceRefResolved)
			}
		} else {
			// Dependencies from different repos cannot provide one shared pinned source ref.
			out.SourceRepo = ""
			out.SourceRefResolved = ""
		}
	}
	if len(ctx.ArtifactExecutions) > 0 {
		if out.ArtifactExecutions == nil {
			out.ArtifactExecutions = map[string][]dependencyArtifactExecution{}
		}
		for ctxDepID, executions := range ctx.ArtifactExecutions {
			targetDepID := strings.TrimSpace(ctxDepID)
			if targetDepID == "" {
				targetDepID = depID
			}
			existing := out.ArtifactExecutions[targetDepID]
			seen := make(map[string]struct{}, len(existing))
			for _, execution := range existing {
				seen[strings.TrimSpace(execution.ID)] = struct{}{}
			}
			for _, execution := range executions {
				execution.ID = strings.TrimSpace(execution.ID)
				if execution.ID == "" {
					continue
				}
				if _, ok := seen[execution.ID]; ok {
					continue
				}
				existing = append(existing, execution)
				seen[execution.ID] = struct{}{}
			}
			out.ArtifactExecutions[targetDepID] = existing
		}
	}
	return nil
}

func verifyDependencyRun(jobs []protocol.JobExecution, projectID int64, pipelineID string) (pipelineDependencyContext, error) {
	jobs = protocol.LatestJobExecutionAttempts(jobs)
	type runState struct {
		lastCreated time.Time
		statuses    []string
		metadata    map[string]string
		jobs        []protocol.JobExecution
	}
	byRun := map[string]runState{}
	for _, j := range jobs {
		if !jobExecutionMatchesProject(j, projectID) {
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
		st.statuses = append(st.statuses, protocol.NormalizeJobExecutionStatus(j.Status))
		st.jobs = append(st.jobs, j)
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
	latestRun := byRun[latestRunID]
	statuses := latestRun.statuses
	for _, st := range statuses {
		if protocol.IsActiveJobExecutionStatus(st) {
			return pipelineDependencyContext{}, fmt.Errorf("latest run is still in progress")
		}
	}

	targetVersionRaw := strings.TrimSpace(latestRun.metadata["pipeline_version_raw"])
	targetVersion := strings.TrimSpace(latestRun.metadata["pipeline_version"])

	selectedRunID := ""
	selectedCreated := time.Time{}
	for runID, st := range byRun {
		if !dependencyRunIsSuccessful(st.statuses) {
			continue
		}
		if !dependencyRunVersionMatches(st.metadata, targetVersionRaw, targetVersion) {
			continue
		}
		if selectedRunID == "" || st.lastCreated.After(selectedCreated) {
			selectedRunID = runID
			selectedCreated = st.lastCreated
		}
	}
	if selectedRunID == "" {
		return pipelineDependencyContext{}, fmt.Errorf("no successful run found for latest dependency version")
	}

	meta := byRun[selectedRunID].metadata
	artifactExecutions := make([]dependencyArtifactExecution, 0)
	for _, j := range byRun[selectedRunID].jobs {
		jobID := strings.TrimSpace(j.ID)
		pipelineJobID := strings.TrimSpace(j.Metadata["pipeline_job_id"])
		if jobID == "" || pipelineJobID == "" || len(j.ArtifactGlobs) == 0 {
			continue
		}
		matrix := map[string]string{}
		if name := strings.TrimSpace(j.Metadata["matrix_name"]); name != "" {
			matrix["name"] = name
		}
		for key, value := range j.Metadata {
			const prefix = "matrix_var."
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			matrixKey := strings.TrimSpace(strings.TrimPrefix(key, prefix))
			if matrixKey != "" {
				matrix[matrixKey] = value
			}
		}
		artifactExecutions = append(artifactExecutions, dependencyArtifactExecution{
			ID:          jobID,
			Pipeline:    pipelineID,
			Job:         pipelineJobID,
			MatrixIndex: dependencyArtifactMatrixIndex(j.Metadata),
			Matrix:      matrix,
		})
	}
	sort.SliceStable(artifactExecutions, func(i, j int) bool {
		if artifactExecutions[i].Job != artifactExecutions[j].Job {
			return artifactExecutions[i].Job < artifactExecutions[j].Job
		}
		return artifactExecutions[i].MatrixIndex < artifactExecutions[j].MatrixIndex
	})
	return pipelineDependencyContext{
		VersionRaw:         strings.TrimSpace(meta["pipeline_version_raw"]),
		Version:            strings.TrimSpace(meta["pipeline_version"]),
		SourceRepo:         strings.TrimSpace(meta["pipeline_source_repo"]),
		SourceRefRaw:       strings.TrimSpace(meta["pipeline_source_ref_raw"]),
		SourceRefResolved:  strings.TrimSpace(meta["pipeline_source_ref_resolved"]),
		ArtifactExecutions: map[string][]dependencyArtifactExecution{pipelineID: artifactExecutions},
	}, nil
}

func dependencyArtifactMatrixIndex(metadata map[string]string) int {
	for _, key := range []string{"matrix_index", "pipeline_job_index"} {
		if value, err := strconv.Atoi(strings.TrimSpace(metadata[key])); err == nil {
			return value
		}
	}
	return 0
}

func sameSourceRepo(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	return a != "" && b != "" && a == b
}

func dependencyRunIsSuccessful(statuses []string) bool {
	if len(statuses) == 0 {
		return false
	}
	for _, st := range statuses {
		if protocol.NormalizeJobExecutionStatus(st) != protocol.JobExecutionStatusSucceeded {
			return false
		}
	}
	return true
}

func dependencyRunVersionMatches(meta map[string]string, targetVersionRaw, targetVersion string) bool {
	runRaw := strings.TrimSpace(meta["pipeline_version_raw"])
	runTagged := strings.TrimSpace(meta["pipeline_version"])
	targetVersionRaw = strings.TrimSpace(targetVersionRaw)
	targetVersion = strings.TrimSpace(targetVersion)

	if targetVersionRaw != "" {
		return runRaw == targetVersionRaw
	}
	if targetVersion != "" {
		return runTagged == targetVersion
	}
	return runRaw == "" && runTagged == ""
}

func verifyDependencyRunInChain(jobs []protocol.JobExecution, chainRunID string, projectID int64, pipelineID string) (pipelineDependencyContext, bool, error) {
	chainRunID = strings.TrimSpace(chainRunID)
	if chainRunID == "" {
		return pipelineDependencyContext{}, false, fmt.Errorf("chain run id is required")
	}
	filtered := make([]protocol.JobExecution, 0)
	for _, j := range jobs {
		if !jobExecutionMatchesProject(j, projectID) {
			continue
		}
		if strings.TrimSpace(j.Metadata["pipeline_id"]) != pipelineID {
			continue
		}
		if strings.TrimSpace(j.Metadata["chain_run_id"]) != chainRunID {
			continue
		}
		filtered = append(filtered, j)
	}
	if len(filtered) == 0 {
		return pipelineDependencyContext{}, false, nil
	}
	ctx, err := verifyDependencyRun(filtered, projectID, pipelineID)
	return ctx, true, err
}
