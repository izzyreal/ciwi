package presentation

import (
	"fmt"
	"strings"
	"time"
)

const (
	TransientNoticeCapacity = 4
	TransientNoticeDuration = 8 * time.Second
)

// TransientNotice is renderer-neutral feedback for a completed operation.
// Route and Section describe an optional navigation action.
type TransientNotice struct {
	Message     string `json:"message"`
	ActionLabel string `json:"action_label,omitempty"`
	Route       string `json:"route,omitempty"`
	Section     string `json:"section,omitempty"`
}

func QueuedPipelineNotice(projectName, pipelineID, pipelineJobID string, enqueued int, dryRun bool, jobExecutionIDs []string) TransientNotice {
	target := strings.TrimSpace(strings.Join(nonEmptyStrings(projectName, pipelineID), " / "))
	if job := strings.TrimSpace(pipelineJobID); job != "" {
		target = strings.TrimSpace(strings.Join(nonEmptyStrings(target, job), " / "))
	}
	kind := executionLabel(enqueued, dryRun)
	notice := TransientNotice{Message: fmt.Sprintf("Queued %d %s for %s", enqueued, kind, fallback(target, "pipeline"))}
	if len(jobExecutionIDs) == 1 && strings.TrimSpace(pipelineJobID) != "" {
		notice.ActionLabel = "Show job execution"
		notice.Route = "/jobs/" + strings.TrimSpace(jobExecutionIDs[0])
		return notice
	}
	notice.ActionLabel = "Show queued jobs"
	notice.Route = "/"
	notice.Section = "queued-executions"
	return notice
}

func QueuedChainNotice(projectName, chainName string, enqueued int, dryRun bool) TransientNotice {
	target := strings.TrimSpace(strings.Join(nonEmptyStrings(projectName, chainName), " / "))
	kind := executionLabel(enqueued, dryRun)
	return TransientNotice{
		Message:     fmt.Sprintf("Queued %d %s for chain %s", enqueued, kind, fallback(target, "chain")),
		ActionLabel: "Show queued jobs", Route: "/", Section: "queued-executions",
	}
}

func QueuedJobNotice(message, jobExecutionID string) TransientNotice {
	jobExecutionID = strings.TrimSpace(jobExecutionID)
	notice := TransientNotice{Message: strings.TrimSpace(message)}
	if jobExecutionID != "" {
		notice.ActionLabel = "Show job execution"
		notice.Route = "/jobs/" + jobExecutionID
	}
	return notice
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func executionLabel(count int, dryRun bool) string {
	label := "execution"
	if count != 1 {
		label = "executions"
	}
	if dryRun {
		label = "dry-run " + label
	}
	return label
}
