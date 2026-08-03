package protocol

import "strings"

const (
	JobExecutionStatusQueued    = "queued"
	JobExecutionStatusLeased    = "leased"
	JobExecutionStatusRunning   = "running"
	JobExecutionStatusSucceeded = "succeeded"
	JobExecutionStatusFailed    = "failed"

	JobSchedulingBlockedMetadataKey       = "scheduling_blocked"
	JobSchedulingBlockedReasonMetadataKey = "scheduling_blocked_reason"
	JobSchedulingRetryUTCMetadataKey      = "scheduling_retry_utc"
)

func JobSchedulingBlockedReason(job JobExecution) string {
	if strings.TrimSpace(job.Metadata[JobSchedulingBlockedMetadataKey]) != "1" {
		return ""
	}
	return strings.TrimSpace(job.Metadata[JobSchedulingBlockedReasonMetadataKey])
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
