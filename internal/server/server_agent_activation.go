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
		if _, err := s.agentJobExecutionStore().UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobExecutionStatusFailed,
			Error:        "cancelled by user",
			TimestampUTC: time.Now().UTC(),
		}); err != nil {
			return cancelled, err
		}
		if err := s.jobExecutionStore().AppendJobExecutionEvents(job.ID, []protocol.JobExecutionEvent{{
			Type:         protocol.JobExecutionEventTypeSystemMessage,
			TimestampUTC: time.Now().UTC(),
			Message:      "[control] job cancelled by user",
		}}); err != nil {
			return cancelled, err
		}
		cancelled++
	}
	return cancelled, nil
}
