package server

import (
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
