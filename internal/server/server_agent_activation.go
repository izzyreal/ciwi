package server

import (
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

func (s *stateStore) cancelActiveJobsForAgent(agentID string) (int, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return 0, nil
	}
	jobs, err := s.jobExecutionStore().ListJobExecutions()
	if err != nil {
		return 0, err
	}
	cancelled := 0
	for _, job := range jobs {
		if strings.TrimSpace(job.LeasedByAgentID) != agentID {
			continue
		}
		if !protocol.IsActiveJobExecutionStatus(job.Status) {
			continue
		}
		fullJob, err := s.jobExecutionStore().GetJobExecution(job.ID)
		if err != nil {
			return cancelled, err
		}
		outputAppend := "[control] job cancelled by user"
		if strings.TrimSpace(fullJob.Output) != "" {
			outputAppend = "\n" + outputAppend
		}
		if _, err := s.agentJobExecutionStore().UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:           agentID,
			Status:            protocol.JobExecutionStatusFailed,
			Error:             "cancelled by user",
			OutputAppend:      outputAppend,
			OutputOffsetBytes: len(fullJob.Output),
			TimestampUTC:      time.Now().UTC(),
		}); err != nil {
			return cancelled, err
		}
		cancelled++
	}
	return cancelled, nil
}
