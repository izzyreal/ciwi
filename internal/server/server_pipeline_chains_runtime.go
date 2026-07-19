package server

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) onJobExecutionUpdated(job protocol.JobExecution) {
	status := protocol.NormalizeJobExecutionStatus(job.Status)
	if !protocol.IsTerminalJobExecutionStatus(status) {
		return
	}
	if strings.TrimSpace(job.Metadata["pipeline_run_id"]) == "" && strings.TrimSpace(job.Metadata["chain_run_id"]) == "" {
		return
	}
	if err := s.reconcileBlockedJobExecutions(); err != nil {
		slog.Error("reconcile blocked job executions after terminal update", "job_id", job.ID, "error", err)
	}
}

// reconcileBlockedJobExecutions is deliberately independent of the triggering job.
// This lets startup and server-generated failures repair and advance persisted runs.
func (s *stateStore) reconcileBlockedJobExecutions() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.dependencyMu.Lock()
	defer s.dependencyMu.Unlock()

	initial, err := s.pipelineStore().ListJobExecutions()
	if err != nil {
		return err
	}
	maxTransitions := len(initial)*3 + 1
	all := initial
	for transition := 0; transition < maxTransitions; transition++ {
		changed, err := s.reconcileOneBlockedJobExecution(all)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		all, err = s.pipelineStore().ListJobExecutions()
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("blocked job reconciliation did not converge after %d transitions", maxTransitions)
}

func (s *stateStore) reconcileOneBlockedJobExecution(all []protocol.JobExecution) (bool, error) {
	for _, candidate := range all {
		if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusQueued {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["chain_blocked"]) == "1" {
			changed, waiting, err := s.reconcileChainBlockedJob(candidate, all)
			if err != nil || changed {
				return changed, err
			}
			if waiting {
				continue
			}
		}
		if strings.TrimSpace(candidate.Metadata["needs_blocked"]) == "1" {
			changed, err := s.reconcileNeedsBlockedJob(candidate, all)
			if err != nil || changed {
				return changed, err
			}
		}
	}
	return false, nil
}

func (s *stateStore) reconcileChainBlockedJob(candidate protocol.JobExecution, all []protocol.JobExecution) (changed, waiting bool, err error) {
	deps := parseChainDependsOnPipelines(candidate.Metadata["chain_depends_on_pipelines"])
	if len(deps) == 0 {
		_, err = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{"chain_blocked": ""})
		return err == nil, false, err
	}
	chainRunID := strings.TrimSpace(candidate.Metadata["chain_run_id"])
	for _, depID := range deps {
		terminated, succeeded, exists := pipelineChainStatus(all, chainRunID, depID)
		if !exists || !terminated {
			return false, true, nil
		}
		if !succeeded {
			reason := "cancelled: upstream pipeline " + depID + " failed"
			return true, false, s.failBlockedJob(candidate, "server-chain", "chain", reason, map[string]string{
				"chain_cancelled": "1",
				"chain_blocked":   "",
			})
		}
	}
	if err := s.bindQueuedChainJobDependencyArtifacts(candidate, all); err != nil {
		reason := "cancelled: " + err.Error()
		return true, false, s.failBlockedJob(candidate, "server-chain", "chain", reason, map[string]string{
			"chain_cancelled": "1",
			"chain_blocked":   "",
		})
	}
	_, err = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{"chain_blocked": ""})
	return err == nil, false, err
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

func (s *stateStore) reconcileNeedsBlockedJob(candidate protocol.JobExecution, all []protocol.JobExecution) (bool, error) {
	needs := parseNeedsJobIDs(candidate.Metadata["needs_job_ids"])
	if len(needs) == 0 {
		_, err := s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{"needs_blocked": ""})
		return err == nil, err
	}
	runID := strings.TrimSpace(candidate.Metadata["pipeline_run_id"])
	projectName := strings.TrimSpace(candidate.Metadata["project"])
	pipelineID := strings.TrimSpace(candidate.Metadata["pipeline_id"])
	for _, need := range needs {
		found := false
		allTerminal := true
		allSucceeded := true
		for _, possible := range all {
			if strings.TrimSpace(possible.Metadata["pipeline_run_id"]) != runID ||
				strings.TrimSpace(possible.Metadata["project"]) != projectName ||
				strings.TrimSpace(possible.Metadata["pipeline_id"]) != pipelineID ||
				strings.TrimSpace(possible.Metadata["pipeline_job_id"]) != need {
				continue
			}
			found = true
			status := protocol.NormalizeJobExecutionStatus(possible.Status)
			if !protocol.IsTerminalJobExecutionStatus(status) {
				allTerminal = false
				allSucceeded = false
				continue
			}
			if status != protocol.JobExecutionStatusSucceeded {
				allSucceeded = false
			}
		}
		if !found || !allTerminal {
			return false, nil
		}
		if !allSucceeded {
			reason := "cancelled: required job " + need + " failed"
			return true, s.failBlockedJob(candidate, "server-needs", "needs", reason, map[string]string{"needs_blocked": ""})
		}
	}
	_, err := s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{"needs_blocked": ""})
	return err == nil, err
}

func (s *stateStore) failBlockedJob(job protocol.JobExecution, agentID, marker, reason string, metadataPatch map[string]string) error {
	if _, err := s.pipelineStore().UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
		AgentID:      agentID,
		Status:       protocol.JobExecutionStatusFailed,
		Error:        reason,
		TimestampUTC: time.Now().UTC(),
	}); err != nil {
		return err
	}
	if err := s.pipelineStore().AppendJobExecutionEvents(job.ID, []protocol.JobExecutionEvent{{
		Type:         protocol.JobExecutionEventTypeSystemMessage,
		TimestampUTC: time.Now().UTC(),
		Message:      "[" + marker + "] " + reason,
	}}); err != nil {
		return err
	}
	_, err := s.pipelineStore().MergeJobExecutionMetadata(job.ID, metadataPatch)
	return err
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
