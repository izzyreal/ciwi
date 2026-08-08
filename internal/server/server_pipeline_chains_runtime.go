package server

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) onJobExecutionUpdated(job protocol.JobExecution) {
	s.app().changes.Publish(application.ChangeQueue, application.ChangeHistory)
	status := protocol.NormalizeJobExecutionStatus(job.Status)
	if !protocol.IsTerminalJobExecutionStatus(status) {
		return
	}
	if job.Metadata.Value(domain.ExecutionMetadataPipelineRunID) == "" && job.Metadata.Value(domain.ExecutionMetadataChainRunID) == "" {
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
		if candidate.Metadata.Flag(domain.ExecutionMetadataChainBlocked) {
			changed, waiting, err := s.reconcileChainBlockedJob(candidate, all)
			if err != nil || changed {
				return changed, err
			}
			if waiting {
				continue
			}
		}
		if candidate.Metadata.Flag(domain.ExecutionMetadataNeedsBlocked) {
			changed, err := s.reconcileNeedsBlockedJob(candidate, all)
			if err != nil || changed {
				return changed, err
			}
		}
	}
	return false, nil
}

func (s *stateStore) reconcileChainBlockedJob(candidate protocol.JobExecution, all []protocol.JobExecution) (changed, waiting bool, err error) {
	deps := candidate.Metadata.CSV(domain.ExecutionMetadataChainDependsOnPipelines)
	if len(deps) == 0 {
		_, err = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{domain.ExecutionMetadataChainBlocked: ""})
		return err == nil, false, err
	}
	chainRunID := candidate.Metadata.Value(domain.ExecutionMetadataChainRunID)
	for _, depID := range deps {
		terminated, succeeded, exists := pipelineChainStatus(all, chainRunID, depID)
		if !exists || !terminated {
			return false, true, nil
		}
		if !succeeded {
			reason := "cancelled: upstream pipeline " + depID + " failed"
			return true, false, s.failBlockedJob(candidate, "server-chain", "chain", reason, map[string]string{
				domain.ExecutionMetadataChainCancelled: "1",
				domain.ExecutionMetadataChainBlocked:   "",
			})
		}
	}
	if err := s.bindQueuedChainJobDependencyArtifacts(candidate, all); err != nil {
		reason := "cancelled: " + err.Error()
		return true, false, s.failBlockedJob(candidate, "server-chain", "chain", reason, map[string]string{
			domain.ExecutionMetadataChainCancelled: "1",
			domain.ExecutionMetadataChainBlocked:   "",
		})
	}
	_, err = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{domain.ExecutionMetadataChainBlocked: ""})
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
	all = protocol.LatestJobExecutionAttempts(all)
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return false, false, false
	}
	terminated = true
	succeeded = true
	for _, j := range all {
		if j.Metadata.Value(domain.ExecutionMetadataChainRunID) != chainRunID {
			continue
		}
		if j.Metadata.Value(domain.ExecutionMetadataPipelineID) != pipelineID {
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
	all = protocol.LatestJobExecutionAttempts(all)
	needs := candidate.Metadata.CSV(domain.ExecutionMetadataNeedsJobIDs)
	if len(needs) == 0 {
		_, err := s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{domain.ExecutionMetadataNeedsBlocked: ""})
		return err == nil, err
	}
	runID := candidate.Metadata.Value(domain.ExecutionMetadataPipelineRunID)
	projectID := candidate.Metadata.Value(domain.ExecutionMetadataProjectID)
	pipelineID := candidate.Metadata.Value(domain.ExecutionMetadataPipelineID)
	for _, need := range needs {
		found := false
		allTerminal := true
		allSucceeded := true
		for _, possible := range all {
			if possible.Metadata.Value(domain.ExecutionMetadataPipelineRunID) != runID ||
				possible.Metadata.Value(domain.ExecutionMetadataProjectID) != projectID ||
				possible.Metadata.Value(domain.ExecutionMetadataPipelineID) != pipelineID ||
				possible.Metadata.Value(domain.ExecutionMetadataPipelineJobID) != need {
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
			return true, s.failBlockedJob(candidate, "server-needs", "needs", reason, map[string]string{domain.ExecutionMetadataNeedsBlocked: ""})
		}
	}
	_, err := s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{domain.ExecutionMetadataNeedsBlocked: ""})
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
	dependsOn, depCtx, err := s.resolveChainJobDependencyContext(job, all)
	if err != nil {
		return err
	}
	if len(dependsOn) == 0 {
		return nil
	}

	dependencyArtifactJobIDs, err := dependencyArtifactJobIDsForJob(job, depCtx)
	if err != nil {
		return err
	}
	if len(dependencyArtifactJobIDs) == 0 {
		return nil
	}
	if _, err := s.pipelineStore().SetJobExecutionDependencyArtifactJobIDs(job.ID, dependencyArtifactJobIDs); err != nil {
		return fmt.Errorf("persist dependency artifact job ids: %w", err)
	}
	return nil
}

func (s *stateStore) resolveChainJobDependencyContext(job protocol.JobExecution, all []protocol.JobExecution) ([]string, pipelineDependencyContext, error) {
	chainRunID := job.Metadata.Value(domain.ExecutionMetadataChainRunID)
	pipelineID := job.Metadata.Value(domain.ExecutionMetadataPipelineID)
	if chainRunID == "" || job.Metadata.Value(domain.ExecutionMetadataProjectID) == "" || pipelineID == "" {
		return nil, pipelineDependencyContext{}, nil
	}
	p, err := s.getPipelineForJobExecution(job)
	if err != nil {
		return nil, pipelineDependencyContext{}, fmt.Errorf("load pipeline %q: %w", pipelineID, err)
	}
	if len(p.DependsOn) == 0 {
		return nil, pipelineDependencyContext{}, nil
	}

	depCtx := pipelineDependencyContext{}
	for _, depID := range p.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" {
			continue
		}
		ctx, foundInChain, err := verifyDependencyRunInChain(all, chainRunID, p.ProjectID, depID)
		if err != nil {
			return nil, pipelineDependencyContext{}, fmt.Errorf("dependency %q not satisfied in chain run: %w", depID, err)
		}
		if !foundInChain {
			ctx, err = verifyDependencyRun(all, p.ProjectID, depID)
			if err != nil {
				return nil, pipelineDependencyContext{}, fmt.Errorf("dependency %q not satisfied: %w", depID, err)
			}
		}
		if err := mergePipelineDependencyContext(&depCtx, depID, ctx); err != nil {
			return nil, pipelineDependencyContext{}, err
		}
	}
	return append([]string(nil), p.DependsOn...), depCtx, nil
}

func dependencyArtifactJobIDsForJob(job protocol.JobExecution, depCtx pipelineDependencyContext) ([]string, error) {
	sources, err := decodeArtifactSources(job.Metadata)
	if err != nil {
		return nil, err
	}
	depJobIDs, err := resolveDependencyArtifactJobIDs(sources, depCtx)
	if err != nil {
		return nil, err
	}
	if len(depJobIDs) == 0 {
		return nil, nil
	}
	return depJobIDs, nil
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
