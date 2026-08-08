package server

import (
	"fmt"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) prepareJobExecutionRerun(original protocol.JobExecution, req *protocol.CreateJobExecutionRequest) error {
	if req == nil {
		return fmt.Errorf("rerun request is required")
	}
	projectID := original.Metadata.Value(domain.ExecutionMetadataProjectID)
	pipelineID := original.Metadata.Value(domain.ExecutionMetadataPipelineID)
	if projectID == "" || pipelineID == "" {
		return nil
	}

	all, err := s.pipelineStore().ListJobExecutions()
	if err != nil {
		return fmt.Errorf("load job attempts: %w", err)
	}
	effective := protocol.LatestJobExecutionAttempts(all)
	if err := validateRerunNeeds(original, effective); err != nil {
		return err
	}
	pipeline, err := s.getPipelineForJobExecution(original)
	if err != nil {
		// The stored execution remains rerunnable even if a later project
		// definition removed or renamed its pipeline.
		return nil
	}

	dependsOn := append([]string(nil), pipeline.DependsOn...)
	var depCtx pipelineDependencyContext
	if len(dependsOn) > 0 && original.Metadata.Value(domain.ExecutionMetadataChainRunID) != "" {
		dependsOn, depCtx, err = s.resolveChainJobDependencyContext(original, effective)
	} else if len(dependsOn) > 0 {
		depCtx, err = s.checkPipelineDependencies(pipeline)
	}
	if err != nil {
		return fmt.Errorf("rerun dependencies are not satisfied: %w", err)
	}
	req.DependencyArtifactJobIDs = nil
	for _, key := range []string{domain.ExecutionMetadataChainCancelled, domain.ExecutionMetadataDependencyBlocked, domain.ExecutionMetadataNeedsBlocked} {
		delete(req.Metadata, key)
	}
	dependencyArtifactJobIDs, err := dependencyArtifactJobIDsForJob(original, depCtx)
	if err != nil {
		return fmt.Errorf("resolve rerun artifact sources: %w", err)
	}
	req.DependencyArtifactJobIDs = dependencyArtifactJobIDs
	return nil
}

func validateRerunNeeds(job protocol.JobExecution, jobs []protocol.JobExecution) error {
	needs := job.Metadata.CSV(domain.ExecutionMetadataNeedsJobIDs)
	if len(needs) == 0 {
		return nil
	}
	runID := job.Metadata.Value(domain.ExecutionMetadataPipelineRunID)
	projectID := job.Metadata.Value(domain.ExecutionMetadataProjectID)
	pipelineID := job.Metadata.Value(domain.ExecutionMetadataPipelineID)
	for _, need := range needs {
		found := false
		for _, candidate := range jobs {
			if candidate.Metadata.Value(domain.ExecutionMetadataPipelineRunID) != runID ||
				candidate.Metadata.Value(domain.ExecutionMetadataProjectID) != projectID ||
				candidate.Metadata.Value(domain.ExecutionMetadataPipelineID) != pipelineID ||
				candidate.Metadata.Value(domain.ExecutionMetadataPipelineJobID) != need {
				continue
			}
			found = true
			if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusSucceeded {
				return fmt.Errorf("required job %q latest attempt has status %s", need, protocol.NormalizeJobExecutionStatus(candidate.Status))
			}
		}
		if !found {
			return fmt.Errorf("required job %q was not found in pipeline run", need)
		}
	}
	return nil
}
