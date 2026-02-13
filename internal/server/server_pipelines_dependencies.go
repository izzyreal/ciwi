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
		st.statuses = append(st.statuses, protocol.NormalizeJobStatus(j.Status))
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
		if protocol.IsActiveJobStatus(st) {
			return pipelineDependencyContext{}, fmt.Errorf("latest run is still in progress")
		}
		if st == protocol.JobStatusFailed {
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
