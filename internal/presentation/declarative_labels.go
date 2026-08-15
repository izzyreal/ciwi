package presentation

import (
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
)

// ExecutionCardDisplay contains renderer-independent labels used by the shared
// declarative screens. Transports carry these values; clients should not
// independently reimplement the presentation rules.
type ExecutionCardDisplay struct {
	Status             string
	SummaryTone        string
	SummaryLabel       string
	JobExecutionIDsCSV string
}

type ExecutionCardJobDisplay struct {
	CreatedLabel  string
	DurationLabel string
	StatusLabel   string
}

func PresentExecutionCard(card domain.ExecutionCard, queued bool) ExecutionCardDisplay {
	status, tone := "succeeded", "success"
	if card.Summary.Failed > 0 {
		status, tone = "failed", "danger"
	} else if queued && card.Summary.InProgress > 0 {
		status, tone = "running", "warning"
	} else if queued {
		status, tone = "waiting", "muted"
	}
	return ExecutionCardDisplay{
		Status: status, SummaryTone: tone,
		SummaryLabel:       ExecutionSummaryLabel(card.Summary),
		JobExecutionIDsCSV: strings.Join(card.JobExecutionIDs, ","),
	}
}

func ExecutionSummaryLabel(summary domain.ExecutionSummary) string {
	parts := []string{fmt.Sprintf("%d/%d successful", max(0, summary.Succeeded), max(0, summary.TotalJobs))}
	if summary.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", summary.Failed))
	}
	if summary.InProgress > 0 {
		parts = append(parts, fmt.Sprintf("%d in progress", summary.InProgress))
	}
	if summary.Waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", summary.Waiting))
	}
	return strings.Join(parts, ", ")
}

func PresentExecutionCardJob(job domain.ExecutionCardJob, now time.Time) ExecutionCardJobDisplay {
	end := job.FinishedUTC
	if end.IsZero() && strings.EqualFold(strings.TrimSpace(job.Status), "running") {
		end = now
	}
	duration := ""
	if !job.StartedUTC.IsZero() && !end.IsZero() && !end.Before(job.StartedUTC) {
		duration = declarativeDuration(end.Sub(job.StartedUTC))
	}
	return ExecutionCardJobDisplay{
		CreatedLabel:  DeclarativeTimestamp(job.CreatedUTC),
		DurationLabel: duration,
		StatusLabel:   statusWithTestCounts(job.Status, job.TestSummary),
	}
}

func statusWithTestCounts(status string, summary *domain.JobTestSummary) string {
	if summary == nil || summary.Total <= 0 {
		return status
	}
	return fmt.Sprintf("%s (%d/%d passed)", status, summary.Passed, summary.Total)
}

func DeclarativeTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("Mon 02 Jan, 15:04:05")
}

func declarativeDuration(value time.Duration) string {
	totalSeconds := max(0, int(value.Seconds()))
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02dh %02dm %02ds", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02dm %02ds", minutes, seconds)
}

func PipelineCountLabel(count int) string {
	return countLabel(count, "pipeline", "pipelines")
}

func ProjectSourceMetadata(repoRef, configFile string) string {
	parts := make([]string, 0, 2)
	if repoRef = strings.TrimSpace(repoRef); repoRef != "" {
		parts = append(parts, "branch: "+repoRef)
	}
	if configFile = strings.TrimSpace(configFile); configFile != "" {
		parts = append(parts, configFile)
	}
	return strings.Join(parts, " · ")
}

func PipelineSummaryLabel(jobsCount int, dependencies string) string {
	dependencies = defaultValue(strings.TrimSpace(dependencies), "none")
	return fmt.Sprintf("%s · depends on: %s", countLabel(jobsCount, "job", "jobs"), dependencies)
}

func PipelineGraphSummaryLabel(jobsCount, dependencyCount int) string {
	return fmt.Sprintf("%s · %s", countLabel(jobsCount, "job", "jobs"), countLabel(dependencyCount, "dependency", "dependencies"))
}

func ProjectJobSummaryLabel(stepsCount int, runsOn string) string {
	return fmt.Sprintf("%s · runs on: %s", countLabel(stepsCount, "step", "steps"), DeclarativeDefaultLabel(runsOn, "unspecified"))
}

func DeclarativeDefaultLabel(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func ProjectJobTimeoutLabel(timeoutSeconds int) string {
	return fmt.Sprintf("Timeout: %ds", max(0, timeoutSeconds))
}

func ProjectJobMatrixLabel(matrixCount int) string {
	if matrixCount <= 0 {
		return "Matrix: none"
	}
	return "Matrix: " + countLabel(matrixCount, "execution", "executions")
}

func ProjectStepEnvironmentLabel(environment []string) string {
	parts := make([]string, 0, len(environment))
	for _, item := range environment {
		if item = strings.TrimSpace(item); item != "" {
			parts = append(parts, item)
		}
	}
	return strings.Join(parts, " · ")
}

func ProjectStepCommand(command string) string {
	if strings.TrimSpace(command) == "" {
		return "(no command)"
	}
	return command
}

func countLabel(count int, singular, plural string) string {
	noun := plural
	if count == 1 {
		noun = singular
	}
	return fmt.Sprintf("%d %s", max(0, count), noun)
}
