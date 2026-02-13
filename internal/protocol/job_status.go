package protocol

import "strings"

const (
	JobStatusQueued    = "queued"
	JobStatusLeased    = "leased"
	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
)

func NormalizeJobStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func IsQueuedJobStatus(status string) bool {
	return NormalizeJobStatus(status) == JobStatusQueued
}

func IsPendingJobStatus(status string) bool {
	switch NormalizeJobStatus(status) {
	case JobStatusQueued, JobStatusLeased:
		return true
	default:
		return false
	}
}

func IsActiveJobStatus(status string) bool {
	switch NormalizeJobStatus(status) {
	case JobStatusQueued, JobStatusLeased, JobStatusRunning:
		return true
	default:
		return false
	}
}

func IsTerminalJobStatus(status string) bool {
	switch NormalizeJobStatus(status) {
	case JobStatusSucceeded, JobStatusFailed:
		return true
	default:
		return false
	}
}

func IsValidJobUpdateStatus(status string) bool {
	switch NormalizeJobStatus(status) {
	case JobStatusRunning, JobStatusSucceeded, JobStatusFailed:
		return true
	default:
		return false
	}
}
