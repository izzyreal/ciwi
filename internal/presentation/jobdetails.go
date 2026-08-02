package presentation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

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
	CanCancel   bool
	CanRerun    bool
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

type JobOutputView struct {
	JobExecutionID string
	Lines          []JobOutputLineView
	NextEventID    int64
	HasMore        bool
	Terminal       bool
}

type JobOutputLineView struct {
	EventID int64
	Text    string
}

type JobDetailsQueries struct {
	executions interface {
		GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error)
		GetJobOutput(context.Context, string, int64) (domain.JobOutputBatch, error)
	}
}

func NewJobDetailsQueries(executions interface {
	GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error)
	GetJobOutput(context.Context, string, int64) (domain.JobOutputBatch, error)
}) *JobDetailsQueries {
	return &JobDetailsQueries{executions: executions}
}

func (q *JobDetailsQueries) GetJobOutputView(ctx context.Context, jobID string, afterEventID int64) (JobOutputView, error) {
	batch, err := q.executions.GetJobOutput(ctx, jobID, afterEventID)
	if err != nil {
		return JobOutputView{}, err
	}
	view := JobOutputView{
		JobExecutionID: batch.JobExecutionID, NextEventID: batch.NextEventID,
		HasMore: batch.HasMore, Terminal: batch.Terminal,
		Lines: make([]JobOutputLineView, 0, len(batch.Events)),
	}
	for _, event := range batch.Events {
		text := renderOutputEvent(event)
		if text != "" {
			view.Lines = append(view.Lines, JobOutputLineView{EventID: event.ID, Text: text})
		}
	}
	return view, nil
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
		CanCancel: canCancelJob(details), CanRerun: canRerunJob(details),
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

func canCancelJob(details domain.JobExecutionDetails) bool {
	switch strings.ToLower(strings.TrimSpace(details.Status)) {
	case "queued", "leased", "running", "in progress":
		return true
	default:
		return false
	}
}

func canRerunJob(details domain.JobExecutionDetails) bool {
	if !details.StartedUTC.IsZero() {
		return true
	}
	if strings.ToLower(strings.TrimSpace(details.Status)) != "failed" {
		return false
	}
	reason := strings.ToLower(strings.TrimSpace(details.Error))
	return (strings.HasPrefix(reason, "cancelled: required job ") || strings.HasPrefix(reason, "cancelled: upstream pipeline ")) && strings.HasSuffix(reason, " failed")
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

func renderOutputEvent(event domain.JobOutputEvent) string {
	text := ""
	switch event.Type {
	case domain.JobOutputEventSystemMessage:
		text = event.Message
	case domain.JobOutputEventOutput:
		text = event.Output
	case domain.JobOutputEventFinished:
		if strings.TrimSpace(event.Error) == "" && (event.ExitCode == nil || *event.ExitCode == 0) {
			return ""
		}
		kind := event.ItemKind
		if kind == "" {
			kind = "execution item"
		}
		title := outputItemTitle(event)
		text = fmt.Sprintf("[%s] failed: %s", kind, title)
		if errText := strings.TrimSpace(event.Error); errText != "" {
			text += " (" + errText + ")"
		} else if event.ExitCode != nil {
			text += fmt.Sprintf(" (exit=%d)", *event.ExitCode)
		}
	}
	text = cleanOutputText(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

func outputItemTitle(event domain.JobOutputEvent) string {
	name := strings.Join(strings.Fields(event.ItemName), " ")
	if event.ItemIndex > 0 && event.ItemTotal > 0 {
		if name == "" {
			return fmt.Sprintf("%d/%d", event.ItemIndex, event.ItemTotal)
		}
		return fmt.Sprintf("%d/%d: %s", event.ItemIndex, event.ItemTotal, name)
	}
	if name != "" {
		return name
	}
	return "unknown"
}

var outputANSIEscapeRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

func cleanOutputText(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = outputANSIEscapeRE.ReplaceAllString(text, "")
	var clean strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if r == '\n' || r == '\t' || r >= 0x20 {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}
