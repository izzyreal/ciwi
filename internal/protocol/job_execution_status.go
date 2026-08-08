package protocol

import (
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
)

const (
	JobExecutionStatusQueued    = "queued"
	JobExecutionStatusLeased    = "leased"
	JobExecutionStatusRunning   = "running"
	JobExecutionStatusSucceeded = "succeeded"
	JobExecutionStatusFailed    = "failed"

	JobSchedulingBlockedMetadataKey       = domain.ExecutionMetadataSchedulingBlocked
	JobSchedulingBlockedReasonMetadataKey = domain.ExecutionMetadataSchedulingBlockedReason
	JobSchedulingRetryUTCMetadataKey      = domain.ExecutionMetadataSchedulingRetryUTC
)

func JobSchedulingBlockedReason(job JobExecution) string {
	if !job.Metadata.Flag(JobSchedulingBlockedMetadataKey) {
		return ""
	}
	return job.Metadata.Value(JobSchedulingBlockedReasonMetadataKey)
}

func NormalizeJobExecutionStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func IsQueuedJobExecutionStatus(status string) bool {
	return NormalizeJobExecutionStatus(status) == JobExecutionStatusQueued
}

func IsPendingJobExecutionStatus(status string) bool {
	switch NormalizeJobExecutionStatus(status) {
	case JobExecutionStatusQueued, JobExecutionStatusLeased:
		return true
	default:
		return false
	}
}

func IsActiveJobExecutionStatus(status string) bool {
	switch NormalizeJobExecutionStatus(status) {
	case JobExecutionStatusQueued, JobExecutionStatusLeased, JobExecutionStatusRunning:
		return true
	default:
		return false
	}
}

func IsTerminalJobExecutionStatus(status string) bool {
	switch NormalizeJobExecutionStatus(status) {
	case JobExecutionStatusSucceeded, JobExecutionStatusFailed:
		return true
	default:
		return false
	}
}

func IsValidJobExecutionUpdateStatus(status string) bool {
	switch NormalizeJobExecutionStatus(status) {
	case JobExecutionStatusRunning, JobExecutionStatusSucceeded, JobExecutionStatusFailed:
		return true
	default:
		return false
	}
}
