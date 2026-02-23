package server

import (
	"fmt"
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
		if strings.TrimSpace(ctx.SourceRepo) != "" && ctx.SourceRefResolved != "" {
			if out.SourceRefResolved == "" {
				out.SourceRepo = strings.TrimSpace(ctx.SourceRepo)
				out.SourceRefResolved = ctx.SourceRefResolved
			} else if sameSourceRepo(out.SourceRepo, ctx.SourceRepo) {
				if out.SourceRefResolved != ctx.SourceRefResolved {
					return pipelineDependencyContext{}, fmt.Errorf("dependency source refs conflict: %q vs %q", out.SourceRefResolved, ctx.SourceRefResolved)
				}
			} else {
				// Dependencies from different repos cannot provide one shared pinned source ref.
				out.SourceRepo = ""
				out.SourceRefResolved = ""
			}
		}
		if len(ctx.ArtifactJobIDs) > 0 {
			if out.ArtifactJobIDs == nil {
				out.ArtifactJobIDs = map[string]string{}
			}
			for k, v := range ctx.ArtifactJobIDs {
				key := depID + ":" + strings.TrimSpace(k)
				if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
					continue
				}
				out.ArtifactJobIDs[key] = strings.TrimSpace(v)
			}
		}
		if len(ctx.ArtifactJobIDsAll) > 0 {
			if out.ArtifactJobIDsAll == nil {
				out.ArtifactJobIDsAll = map[string][]string{}
			}
			for ctxDepID, ids := range ctx.ArtifactJobIDsAll {
				targetDepID := strings.TrimSpace(ctxDepID)
				if targetDepID == "" {
					targetDepID = depID
				}
				existing := out.ArtifactJobIDsAll[targetDepID]
				seen := map[string]struct{}{}
				for _, v := range existing {
					if strings.TrimSpace(v) == "" {
						continue
					}
					seen[strings.TrimSpace(v)] = struct{}{}
				}
				for _, v := range ids {
					v = strings.TrimSpace(v)
					if v == "" {
						continue
					}
					if _, ok := seen[v]; ok {
						continue
					}
					existing = append(existing, v)
					seen[v] = struct{}{}
				}
				out.ArtifactJobIDsAll[targetDepID] = existing
			}
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

func verifyDependencyRun(jobs []protocol.JobExecution, projectName, pipelineID string) (pipelineDependencyContext, error) {
	type runState struct {
		lastCreated time.Time
		statuses    []string
		metadata    map[string]string
		jobs        []protocol.JobExecution
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
	artifactJobIDs := map[string]string{}
	artifactJobIDsAll := make([]string, 0)
	artifactJobSeen := map[string]struct{}{}
	for _, j := range byRun[selectedRunID].jobs {
		jobID := strings.TrimSpace(j.ID)
		if jobID == "" {
			continue
		}
		if len(j.ArtifactGlobs) > 0 {
			if _, exists := artifactJobSeen[jobID]; !exists {
				artifactJobIDsAll = append(artifactJobIDsAll, jobID)
				artifactJobSeen[jobID] = struct{}{}
			}
		}
		for _, key := range []string{
			strings.TrimSpace(j.Metadata["build_target"]),
			strings.TrimSpace(j.Metadata["matrix_name"]),
			strings.TrimSpace(j.Metadata["pipeline_job_id"]),
		} {
			if key == "" {
				continue
			}
			if _, exists := artifactJobIDs[key]; !exists {
				artifactJobIDs[key] = jobID
			}
		}
	}
	return pipelineDependencyContext{
		VersionRaw:        strings.TrimSpace(meta["pipeline_version_raw"]),
		Version:           strings.TrimSpace(meta["pipeline_version"]),
		SourceRepo:        strings.TrimSpace(meta["pipeline_source_repo"]),
		SourceRefRaw:      strings.TrimSpace(meta["pipeline_source_ref_raw"]),
		SourceRefResolved: strings.TrimSpace(meta["pipeline_source_ref_resolved"]),
		ArtifactJobIDs:    artifactJobIDs,
		ArtifactJobIDsAll: map[string][]string{pipelineID: artifactJobIDsAll},
	}, nil
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

func verifyDependencyRunInChain(jobs []protocol.JobExecution, chainRunID, projectName, pipelineID string) (pipelineDependencyContext, bool, error) {
	chainRunID = strings.TrimSpace(chainRunID)
	if chainRunID == "" {
		return pipelineDependencyContext{}, false, fmt.Errorf("chain run id is required")
	}
	filtered := make([]protocol.JobExecution, 0)
	for _, j := range jobs {
		if strings.TrimSpace(j.Metadata["project"]) != projectName {
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
	ctx, err := verifyDependencyRun(filtered, projectName, pipelineID)
	return ctx, true, err
}
