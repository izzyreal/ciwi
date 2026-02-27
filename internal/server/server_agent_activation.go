package server

import (
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const agentDeactivatedStatePrefix = "agent_deactivated:"

func agentDeactivatedStateKey(agentID string) string {
	return agentDeactivatedStatePrefix + strings.TrimSpace(agentID)
}

func parseBooleanStateValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

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
		output := strings.TrimSpace(job.Output)
		if output != "" {
			output += "\n"
		}
		output += "[control] job cancelled by user"
		if _, err := s.agentJobExecutionStore().UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID:      agentID,
			Status:       protocol.JobExecutionStatusFailed,
			Error:        "cancelled by user",
			Output:       output,
			TimestampUTC: time.Now().UTC(),
		}); err != nil {
			return cancelled, err
		}
		cancelled++
	}
	return cancelled, nil
}
