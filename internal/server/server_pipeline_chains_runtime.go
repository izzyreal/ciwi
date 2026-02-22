package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) onJobExecutionUpdated(job protocol.JobExecution) {
	s.onJobExecutionUpdatedChain(job)
	s.onJobExecutionUpdatedNeeds(job)
}

func (s *stateStore) onJobExecutionUpdatedChain(job protocol.JobExecution) {
	chainRunID := strings.TrimSpace(job.Metadata["chain_run_id"])
	if chainRunID == "" {
		return
	}
	if strings.TrimSpace(job.Metadata["chain_cancelled"]) == "1" {
		return
	}
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	if pipelineID == "" {
		return
	}
	status := protocol.NormalizeJobExecutionStatus(job.Status)
	if !protocol.IsTerminalJobExecutionStatus(status) {
		return
	}
	all, err := s.pipelineStore().ListJobExecutions()
	if err != nil {
		return
	}
	currentTerminated, currentSucceeded, _ := pipelineChainStatus(all, chainRunID, pipelineID)
	if !currentTerminated {
		return
	}

	if !currentSucceeded {
		cancelBlockedChainDependents(s, all, chainRunID, pipelineID)
		return
	}

	for _, candidate := range all {
		if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusQueued {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["chain_run_id"]) != chainRunID {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["chain_blocked"]) != "1" {
			continue
		}
		deps := parseChainDependsOnPipelines(candidate.Metadata["chain_depends_on_pipelines"])
		if len(deps) == 0 {
			_, _ = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{
				"chain_blocked": "",
			})
			continue
		}
		ready := true
		failedDep := ""
		for _, depID := range deps {
			depTerminated, depSucceeded, depExists := pipelineChainStatus(all, chainRunID, depID)
			if !depExists || !depTerminated {
				ready = false
				break
			}
			if !depSucceeded {
				failedDep = depID
				break
			}
		}
		if failedDep != "" {
			cancelChainJob(s, candidate, "cancelled: upstream pipeline "+failedDep+" failed")
			continue
		}
		if !ready {
			continue
		}
		if err := s.bindQueuedChainJobDependencyArtifacts(candidate, all); err != nil {
			cancelChainJob(s, candidate, "cancelled: "+err.Error())
			continue
		}
		_, _ = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{
			"chain_blocked": "",
		})
	}
}

func parseChainDependsOnPipelines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func pipelineChainStatus(all []protocol.JobExecution, chainRunID, pipelineID string) (terminated bool, succeeded bool, exists bool) {
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return false, false, false
	}
	terminated = true
	succeeded = true
	for _, j := range all {
		if strings.TrimSpace(j.Metadata["chain_run_id"]) != chainRunID {
			continue
		}
		if strings.TrimSpace(j.Metadata["pipeline_id"]) != pipelineID {
			continue
		}
		exists = true
		status := protocol.NormalizeJobExecutionStatus(j.Status)
		if !protocol.IsTerminalJobExecutionStatus(status) {
			terminated = false
			succeeded = false
			continue
		}
		if status != protocol.JobExecutionStatusSucceeded {
			succeeded = false
		}
	}
	if !exists {
		return false, false, false
	}
	return terminated, succeeded, true
}

func cancelChainJob(s *stateStore, job protocol.JobExecution, reason string) {
	_, _ = s.pipelineStore().MergeJobExecutionMetadata(job.ID, map[string]string{
		"chain_cancelled": "1",
		"chain_blocked":   "",
	})
	_, _ = s.pipelineStore().UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      "server-chain",
		Status:       protocol.JobExecutionStatusFailed,
		Error:        reason,
		Output:       "[chain] " + reason,
		TimestampUTC: time.Now().UTC(),
	})
}

func cancelBlockedChainDependents(s *stateStore, all []protocol.JobExecution, chainRunID, failedPipelineID string) {
	failedPipelineID = strings.TrimSpace(failedPipelineID)
	if failedPipelineID == "" {
		return
	}
	for _, candidate := range all {
		if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusQueued {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["chain_run_id"]) != chainRunID {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["chain_blocked"]) != "1" {
			continue
		}
		deps := parseChainDependsOnPipelines(candidate.Metadata["chain_depends_on_pipelines"])
		if len(deps) == 0 {
			continue
		}
		if !needsContains(deps, failedPipelineID) {
			continue
		}
		cancelChainJob(s, candidate, "cancelled: upstream pipeline "+failedPipelineID+" failed")
	}
}

func (s *stateStore) onJobExecutionUpdatedNeeds(job protocol.JobExecution) {
	runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
	projectName := strings.TrimSpace(job.Metadata["project"])
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	pipelineJobID := strings.TrimSpace(job.Metadata["pipeline_job_id"])
	if runID == "" || projectName == "" || pipelineID == "" || pipelineJobID == "" {
		return
	}
	status := protocol.NormalizeJobExecutionStatus(job.Status)
	if !protocol.IsTerminalJobExecutionStatus(status) {
		return
	}
	all, err := s.pipelineStore().ListJobExecutions()
	if err != nil {
		return
	}
	inRun := make([]protocol.JobExecution, 0)
	for _, candidate := range all {
		if strings.TrimSpace(candidate.Metadata["pipeline_run_id"]) != runID {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["project"]) != projectName {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["pipeline_id"]) != pipelineID {
			continue
		}
		inRun = append(inRun, candidate)
	}
	if len(inRun) == 0 {
		return
	}

	upstreamGroup := make([]protocol.JobExecution, 0)
	for _, candidate := range inRun {
		if strings.TrimSpace(candidate.Metadata["pipeline_job_id"]) != pipelineJobID {
			continue
		}
		upstreamGroup = append(upstreamGroup, candidate)
	}
	if len(upstreamGroup) == 0 {
		return
	}
	for _, current := range upstreamGroup {
		if !protocol.IsTerminalJobExecutionStatus(protocol.NormalizeJobExecutionStatus(current.Status)) {
			return
		}
	}

	upstreamSucceeded := true
	for _, current := range upstreamGroup {
		if protocol.NormalizeJobExecutionStatus(current.Status) != protocol.JobExecutionStatusSucceeded {
			upstreamSucceeded = false
			break
		}
	}
	if !upstreamSucceeded {
		reason := "cancelled: required job " + pipelineJobID + " failed"
		for _, candidate := range inRun {
			if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusQueued {
				continue
			}
			if strings.TrimSpace(candidate.Metadata["needs_blocked"]) != "1" {
				continue
			}
			needs := parseNeedsJobIDs(candidate.Metadata["needs_job_ids"])
			if !needsContains(needs, pipelineJobID) {
				continue
			}
			_, _ = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{
				"needs_blocked": "",
			})
			_, _ = s.pipelineStore().UpdateJobExecutionStatus(candidate.ID, protocol.JobExecutionStatusUpdateRequest{
				AgentID:      "server-needs",
				Status:       protocol.JobExecutionStatusFailed,
				Error:        reason,
				Output:       "[needs] " + reason,
				TimestampUTC: time.Now().UTC(),
			})
		}
		return
	}

	// Unblock queued jobs only when all their required pipeline jobs are fully successful.
	for _, candidate := range inRun {
		if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusQueued {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["needs_blocked"]) != "1" {
			continue
		}
		needs := parseNeedsJobIDs(candidate.Metadata["needs_job_ids"])
		if len(needs) == 0 || !needsContains(needs, pipelineJobID) {
			continue
		}
		ready := true
		for _, need := range needs {
			needGroup := make([]protocol.JobExecution, 0)
			for _, possible := range inRun {
				if strings.TrimSpace(possible.Metadata["pipeline_job_id"]) != need {
					continue
				}
				needGroup = append(needGroup, possible)
			}
			if len(needGroup) == 0 {
				ready = false
				break
			}
			for _, dep := range needGroup {
				if protocol.NormalizeJobExecutionStatus(dep.Status) != protocol.JobExecutionStatusSucceeded {
					ready = false
					break
				}
			}
			if !ready {
				break
			}
		}
		if !ready {
			continue
		}
		_, _ = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{
			"needs_blocked": "",
		})
	}
}

func (s *stateStore) bindQueuedChainJobDependencyArtifacts(job protocol.JobExecution, all []protocol.JobExecution) error {
	chainRunID := strings.TrimSpace(job.Metadata["chain_run_id"])
	projectName := strings.TrimSpace(job.Metadata["project"])
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	if chainRunID == "" || projectName == "" || pipelineID == "" {
		return nil
	}
	p, err := s.pipelineStore().GetPipelineByProjectAndID(projectName, pipelineID)
	if err != nil {
		return fmt.Errorf("load pipeline %q: %w", pipelineID, err)
	}
	if len(p.DependsOn) == 0 {
		return nil
	}

	depCtx := pipelineDependencyContext{}
	for _, depID := range p.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		ctx, foundInChain, err := verifyDependencyRunInChain(all, chainRunID, projectName, depID)
		if err != nil {
			return fmt.Errorf("dependency %q not satisfied in chain run: %w", depID, err)
		}
		if !foundInChain {
			ctx, err = verifyDependencyRun(all, projectName, depID)
			if err != nil {
				return fmt.Errorf("dependency %q not satisfied: %w", depID, err)
			}
		}
		if len(ctx.ArtifactJobIDs) > 0 {
			if depCtx.ArtifactJobIDs == nil {
				depCtx.ArtifactJobIDs = map[string]string{}
			}
			for k, v := range ctx.ArtifactJobIDs {
				key := depID + ":" + strings.TrimSpace(k)
				if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
					continue
				}
				depCtx.ArtifactJobIDs[key] = strings.TrimSpace(v)
			}
		}
		if len(ctx.ArtifactJobIDsAll) > 0 {
			if depCtx.ArtifactJobIDsAll == nil {
				depCtx.ArtifactJobIDsAll = map[string][]string{}
			}
			for ctxDepID, ids := range ctx.ArtifactJobIDsAll {
				targetDepID := strings.TrimSpace(ctxDepID)
				if targetDepID == "" {
					targetDepID = depID
				}
				existing := depCtx.ArtifactJobIDsAll[targetDepID]
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
				depCtx.ArtifactJobIDsAll[targetDepID] = existing
			}
		}
	}

	vars := map[string]string{
		"name":         strings.TrimSpace(job.Metadata["matrix_name"]),
		"build_target": strings.TrimSpace(job.Metadata["build_target"]),
	}
	depJobID := resolveDependencyArtifactJobID(p.DependsOn, depCtx.ArtifactJobIDs, strings.TrimSpace(job.Metadata["pipeline_job_id"]), vars)
	depJobIDs := resolveDependencyArtifactJobIDs(p.DependsOn, depCtx.ArtifactJobIDsAll, depJobID)
	if depJobID == "" && len(depJobIDs) == 0 {
		return nil
	}

	envPatch := map[string]string{
		"CIWI_DEP_ARTIFACT_JOB_ID":  depJobID,
		"CIWI_DEP_ARTIFACT_JOB_IDS": strings.Join(depJobIDs, ","),
	}
	if _, err := s.pipelineStore().MergeJobExecutionEnv(job.ID, envPatch); err != nil {
		return fmt.Errorf("persist dependency artifact env: %w", err)
	}
	return nil
}

func parseNeedsJobIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func needsContains(needs []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, need := range needs {
		if strings.TrimSpace(need) == target {
			return true
		}
	}
	return false
}
