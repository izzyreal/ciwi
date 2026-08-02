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
	ID           string
	Title        string
	Context      string
	Status       string
	StatusLabel  string
	CurrentStep  string
	Agent        string
	Mode         string
	Created      string
	Started      string
	Finished     string
	Duration     string
	ExitCode     string
	Error        string
	CanCancel    bool
	CanRerun     bool
	Timeline     []JobTimelineView
	OutputGroups []JobOutputGroupView
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

type JobOutputGroupView struct {
	ID              string
	StateKey        string
	Kind            string
	Title           string
	CommandSummary  string
	Status          string
	StatusLabel     string
	Reached         bool
	Started         string
	Duration        string
	ExitCode        string
	Error           string
	Details         string
	YAMLLiteral     string
	ExpandedCommand string
}

type JobOutputView struct {
	JobExecutionID string
	Events         []JobOutputEventView
	NextEventID    int64
	HasMore        bool
	Terminal       bool
}

type JobOutputEventView struct {
	EventID  int64
	Type     string
	ItemID   string
	Text     string
	Error    string
	ExitCode string
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
		Events: make([]JobOutputEventView, 0, len(batch.Events)),
	}
	for _, event := range batch.Events {
		text := outputEventText(event)
		if event.Type != "" {
			view.Events = append(view.Events, JobOutputEventView{
				EventID: event.ID, Type: event.Type, ItemID: event.ItemID, Text: text,
				Error: strings.TrimSpace(event.Error), ExitCode: formatExitCode(event.ExitCode),
			})
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
	phaseTotal, stepTotal := 0, 0
	for _, item := range details.Timeline {
		if item.Kind == "phase" {
			phaseTotal++
		} else {
			stepTotal++
		}
	}
	phaseIndex, stepIndex := 0, 0
	view.Timeline = make([]JobTimelineView, 0, len(details.Timeline))
	view.OutputGroups = make([]JobOutputGroupView, 0, len(details.Timeline))
	for _, item := range details.Timeline {
		prefix := "Job step"
		categoryIndex, categoryTotal := stepIndex+1, stepTotal
		if item.Kind == "phase" {
			prefix = "Ciwi phase"
			phaseIndex++
			categoryIndex, categoryTotal = phaseIndex, phaseTotal
		} else {
			stepIndex++
			categoryIndex = stepIndex
		}
		title := fmt.Sprintf("%s %d/%d", prefix, categoryIndex, categoryTotal)
		if name := strings.TrimSpace(item.Name); name != "" {
			title += ": " + name
		}
		reached := item.Reached || (item.Status != "" && item.Status != "pending" && item.Status != "not reached")
		view.Timeline = append(view.Timeline, JobTimelineView{
			ID: item.ID, Kind: item.Kind, Title: title, Description: item.Description,
			Status: item.Status, StatusLabel: humanStatus(item.Status), Duration: formatDurationMS(item.DurationMS),
			ExitCode: formatExitCode(item.ExitCode), Error: item.Error,
		})
		view.OutputGroups = append(view.OutputGroups, JobOutputGroupView{
			ID: item.ID, StateKey: "job-output:" + details.ID + ":" + item.ID, Kind: item.Kind, Title: title,
			CommandSummary: strings.Join(strings.Fields(item.Command), " "), Status: item.Status,
			StatusLabel: humanStatus(item.Status), Reached: reached, Started: formatTimestamp(item.StartedUTC),
			Duration: formatDurationMS(item.DurationMS), ExitCode: formatExitCode(item.ExitCode), Error: item.Error,
			Details: item.Description, YAMLLiteral: item.YAMLLiteral, ExpandedCommand: item.Command,
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

func outputEventText(event domain.JobOutputEvent) string {
	text := ""
	switch event.Type {
	case domain.JobOutputEventSystemMessage:
		text = event.Message
	case domain.JobOutputEventOutput:
		text = event.Output
	case domain.JobOutputEventFinished:
		return ""
	}
	text = cleanOutputText(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
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
