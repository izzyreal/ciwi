package protocol

import "strings"

const (
	JobExecutionStatusQueued    = "queued"
	JobExecutionStatusLeased    = "leased"
	JobExecutionStatusRunning   = "running"
	JobExecutionStatusSucceeded = "succeeded"
	JobExecutionStatusFailed    = "failed"
)

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
