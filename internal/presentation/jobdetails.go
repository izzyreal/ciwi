package presentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
)

type JobDetailsView struct {
	ID          string
	Title       string
	Context     string
	Status      string
	StatusLabel string
	CurrentStep string
	Agent       string
	Mode        string
	Created     string
	Started     string
	Finished    string
	Duration    string
	ExitCode    string
	Error       string
	Timeline    []JobTimelineView
}

type JobTimelineView struct {
	ID          string
	Kind        string
	Title       string
	Description string
	Status      string
	StatusLabel string
	Duration    string
	ExitCode    string
	Error       string
}

type JobDetailsQueries struct {
	executions interface {
		GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error)
	}
}

func NewJobDetailsQueries(executions interface {
	GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error)
}) *JobDetailsQueries {
	return &JobDetailsQueries{executions: executions}
}

func (q *JobDetailsQueries) GetJobDetailsView(ctx context.Context, jobID string) (JobDetailsView, error) {
	details, err := q.executions.GetJobExecutionDetails(ctx, jobID)
	if err != nil {
		return JobDetailsView{}, err
	}
	return presentJobDetails(details), nil
}

func presentJobDetails(details domain.JobExecutionDetails) JobDetailsView {
	titleTarget := firstNonEmpty(details.PipelineJobID, details.PipelineID, details.ID)
	view := JobDetailsView{
		ID: details.ID, Title: "Job: " + titleTarget, Context: jobContext(details),
		Status: details.Status, StatusLabel: humanStatus(details.Status), CurrentStep: details.CurrentStep,
		Agent: details.AgentID, Created: formatTimestamp(details.CreatedUTC), Started: formatTimestamp(details.StartedUTC),
		Finished: formatTimestamp(details.FinishedUTC), ExitCode: formatExitCode(details.ExitCode), Error: details.Error,
	}
	if details.DryRun {
		view.Mode = "Dry run"
	} else {
		view.Mode = "Run"
	}
	if !details.StartedUTC.IsZero() && !details.FinishedUTC.IsZero() && !details.FinishedUTC.Before(details.StartedUTC) {
		view.Duration = formatDuration(details.FinishedUTC.Sub(details.StartedUTC))
	}
	view.Timeline = make([]JobTimelineView, 0, len(details.Timeline))
	for _, item := range details.Timeline {
		prefix := "Job step"
		if item.Kind == "phase" {
			prefix = "Ciwi phase"
		}
		title := fmt.Sprintf("%s %d/%d", prefix, item.Index, item.Total)
		if name := strings.TrimSpace(item.Name); name != "" {
			title += ": " + name
		}
		view.Timeline = append(view.Timeline, JobTimelineView{
			ID: item.ID, Kind: item.Kind, Title: title, Description: item.Description,
			Status: item.Status, StatusLabel: humanStatus(item.Status), Duration: formatDurationMS(item.DurationMS),
			ExitCode: formatExitCode(item.ExitCode), Error: item.Error,
		})
	}
	return view
}

func jobContext(details domain.JobExecutionDetails) string {
	parts := make([]string, 0, 4)
	if details.ProjectName != "" {
		parts = append(parts, details.ProjectName)
	}
	if details.PipelineID != "" {
		parts = append(parts, "pipeline "+details.PipelineID)
	}
	if details.MatrixName != "" {
		parts = append(parts, details.MatrixName)
	}
	parts = append(parts, "execution "+details.ID)
	return strings.Join(parts, " · ")
}

func formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		return ""
	}
	value = value.Round(time.Millisecond)
	return value.String()
}

func formatDurationMS(value int64) string {
	if value <= 0 {
		return ""
	}
	return formatDuration(time.Duration(value) * time.Millisecond)
}

func formatExitCode(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func humanStatus(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "_", " "), "-", " "))
	if value == "" {
		return "Unknown"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "execution"
}
