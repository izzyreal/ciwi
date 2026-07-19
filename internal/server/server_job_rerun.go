package server

import (
	"fmt"
	"strings"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) prepareJobExecutionRerun(original protocol.JobExecution, req *protocol.CreateJobExecutionRequest) error {
	if req == nil {
		return fmt.Errorf("rerun request is required")
	}
	projectName := strings.TrimSpace(original.Metadata["project"])
	pipelineID := strings.TrimSpace(original.Metadata["pipeline_id"])
	if projectName == "" || pipelineID == "" {
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
	pipeline, err := s.pipelineStore().GetPipelineByProjectAndID(projectName, pipelineID)
	if err != nil {
		// The stored execution remains rerunnable even if a later project
		// definition removed or renamed its pipeline.
		return nil
	}

	dependsOn := append([]string(nil), pipeline.DependsOn...)
	var depCtx pipelineDependencyContext
	if len(dependsOn) > 0 && strings.TrimSpace(original.Metadata["chain_run_id"]) != "" {
		dependsOn, depCtx, err = s.resolveChainJobDependencyContext(original, effective)
	} else if len(dependsOn) > 0 {
		depCtx, err = s.checkPipelineDependencies(pipeline)
	}
	if err != nil {
		return fmt.Errorf("rerun dependencies are not satisfied: %w", err)
	}
	if req.Env == nil {
		req.Env = map[string]string{}
	}
	delete(req.Env, "CIWI_DEP_ARTIFACT_JOB_ID")
	delete(req.Env, "CIWI_DEP_ARTIFACT_JOB_IDS")
	for _, key := range []string{"chain_cancelled", "dependency_blocked", "needs_blocked"} {
		delete(req.Metadata, key)
	}
	for key, value := range dependencyArtifactEnv(original, dependsOn, depCtx) {
		req.Env[key] = value
	}
	return nil
}

func validateRerunNeeds(job protocol.JobExecution, jobs []protocol.JobExecution) error {
	needs := parseNeedsJobIDs(job.Metadata["needs_job_ids"])
	if len(needs) == 0 {
		return nil
	}
	runID := strings.TrimSpace(job.Metadata["pipeline_run_id"])
	projectName := strings.TrimSpace(job.Metadata["project"])
	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	for _, need := range needs {
		found := false
		for _, candidate := range jobs {
			if strings.TrimSpace(candidate.Metadata["pipeline_run_id"]) != runID ||
				strings.TrimSpace(candidate.Metadata["project"]) != projectName ||
				strings.TrimSpace(candidate.Metadata["pipeline_id"]) != pipelineID ||
				strings.TrimSpace(candidate.Metadata["pipeline_job_id"]) != need {
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
