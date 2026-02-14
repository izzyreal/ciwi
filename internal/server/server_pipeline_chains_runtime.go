package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) onJobExecutionUpdated(job protocol.JobExecution) {
	chainRunID := strings.TrimSpace(job.Metadata["chain_run_id"])
	if chainRunID == "" {
		return
	}
	if strings.TrimSpace(job.Metadata["chain_cancelled"]) == "1" {
		return
	}
	pos, err := strconv.Atoi(strings.TrimSpace(job.Metadata["pipeline_chain_position"]))
	if err != nil || pos <= 0 {
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
	if status == protocol.JobExecutionStatusSucceeded {
		currentPosJobs := make([]protocol.JobExecution, 0)
		for _, candidate := range all {
			if strings.TrimSpace(candidate.Metadata["chain_run_id"]) != chainRunID {
				continue
			}
			cpos, err := strconv.Atoi(strings.TrimSpace(candidate.Metadata["pipeline_chain_position"]))
			if err != nil || cpos != pos {
				continue
			}
			currentPosJobs = append(currentPosJobs, candidate)
		}
		if len(currentPosJobs) == 0 {
			return
		}
		for _, current := range currentPosJobs {
			cstatus := protocol.NormalizeJobExecutionStatus(current.Status)
			if cstatus != protocol.JobExecutionStatusSucceeded {
				// Wait until every job in this chain position has succeeded.
				return
			}
		}
		nextPos := strconv.Itoa(pos + 1)
		for _, candidate := range all {
			if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusQueued {
				continue
			}
			if strings.TrimSpace(candidate.Metadata["chain_run_id"]) != chainRunID {
				continue
			}
			if strings.TrimSpace(candidate.Metadata["pipeline_chain_position"]) != nextPos {
				continue
			}
			if strings.TrimSpace(candidate.Metadata["chain_blocked"]) != "1" {
				continue
			}
			if err := s.bindQueuedChainJobDependencyArtifacts(candidate, all); err != nil {
				reason := "cancelled: " + err.Error()
				_, _ = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{
					"chain_cancelled": "1",
					"chain_blocked":   "",
				})
				_, _ = s.pipelineStore().UpdateJobExecutionStatus(candidate.ID, protocol.JobExecutionStatusUpdateRequest{
					AgentID:      "server-chain",
					Status:       protocol.JobExecutionStatusFailed,
					Error:        reason,
					Output:       "[chain] " + reason,
					TimestampUTC: time.Now().UTC(),
				})
				continue
			}
			_, _ = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{
				"chain_blocked": "",
			})
		}
		return
	}

	pipelineID := strings.TrimSpace(job.Metadata["pipeline_id"])
	reason := "cancelled: upstream pipeline failed"
	if pipelineID != "" {
		reason = "cancelled: upstream pipeline " + pipelineID + " failed"
	}
	for _, candidate := range all {
		if protocol.NormalizeJobExecutionStatus(candidate.Status) != protocol.JobExecutionStatusQueued {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["chain_run_id"]) != chainRunID {
			continue
		}
		cpos, err := strconv.Atoi(strings.TrimSpace(candidate.Metadata["pipeline_chain_position"]))
		if err != nil || cpos <= pos {
			continue
		}
		_, _ = s.pipelineStore().MergeJobExecutionMetadata(candidate.ID, map[string]string{
			"chain_cancelled": "1",
		})
		_, _ = s.pipelineStore().UpdateJobExecutionStatus(candidate.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      "server-chain",
			Status:       protocol.JobExecutionStatusFailed,
			Error:        reason,
			Output:       "[chain] " + reason,
			TimestampUTC: time.Now().UTC(),
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
