package executionviews

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobhistory"
)

type Store interface {
	ListJobExecutions() ([]protocol.JobExecution, error)
	GetJobExecution(string) (protocol.JobExecution, error)
	ListJobExecutionTimelineEvents(string) ([]protocol.JobExecutionEvent, error)
	ListJobExecutionEventsPageAfter(string, int64, int) ([]protocol.JobExecutionEvent, error)
}

const (
	outputPageSize  = 128
	outputPageBytes = 512 * 1024
)

func (r *Repository) ListJobOutputAfter(ctx context.Context, jobID string, afterEventID int64) (domain.JobOutputBatch, error) {
	if err := ctx.Err(); err != nil {
		return domain.JobOutputBatch{}, err
	}
	job, err := r.store.GetJobExecution(jobID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return domain.JobOutputBatch{}, domain.ErrJobExecutionNotFound
		}
		return domain.JobOutputBatch{}, err
	}
	events, err := r.store.ListJobExecutionEventsPageAfter(jobID, afterEventID, outputPageSize)
	if err != nil {
		return domain.JobOutputBatch{}, err
	}
	batch := domain.JobOutputBatch{
		JobExecutionID: jobID, NextEventID: afterEventID,
		Terminal: protocol.IsTerminalJobExecutionStatus(protocol.NormalizeJobExecutionStatus(job.Status)),
		Events:   make([]domain.JobOutputEvent, 0, len(events)),
	}
	pageBytes := 0
	for eventIndex, event := range events {
		eventBytes := len(event.Output) + len(event.Message) + len(event.Error)
		if len(batch.Events) > 0 && pageBytes+eventBytes > outputPageBytes {
			batch.HasMore = true
			break
		}
		itemKind, itemName, itemIndex, itemTotal := "", "", 0, 0
		itemID := ""
		if event.Phase != nil {
			itemKind, itemName, itemIndex, itemTotal = "phase", event.Phase.Name, event.Phase.Index, event.Phase.Total
			itemID = strings.TrimSpace(event.Phase.ID)
		} else if event.Step != nil {
			itemKind, itemName, itemIndex, itemTotal = "step", event.Step.Name, event.Step.Index, event.Step.Total
			if event.Step.Index > 0 {
				itemID = fmt.Sprintf("step:%d", event.Step.Index)
			}
		}
		batch.Events = append(batch.Events, domain.JobOutputEvent{
			ID: event.ID, Type: outputEventType(event.Type), Message: event.Message, Output: event.Output,
			Error: event.Error, ExitCode: copyInt(event.ExitCode), ItemID: itemID, ItemKind: itemKind,
			ItemName: itemName, ItemIndex: itemIndex, ItemTotal: itemTotal,
		})
		if event.ID > batch.NextEventID {
			batch.NextEventID = event.ID
		}
		pageBytes += eventBytes
		if eventIndex == len(events)-1 && len(events) == outputPageSize {
			batch.HasMore = true
		}
	}
	return batch, nil
}

func outputEventType(eventType string) string {
	switch eventType {
	case protocol.JobExecutionEventTypeSystemMessage:
		return domain.JobOutputEventSystemMessage
	case protocol.JobExecutionEventTypeStepOutput, protocol.JobExecutionEventTypePhaseOutput:
		return domain.JobOutputEventOutput
	case protocol.JobExecutionEventTypeStepFinished, protocol.JobExecutionEventTypePhaseFinished:
		return domain.JobOutputEventFinished
	default:
		return ""
	}
}

func (r *Repository) GetJobExecutionDetails(ctx context.Context, jobID string) (domain.JobExecutionDetails, error) {
	if err := ctx.Err(); err != nil {
		return domain.JobExecutionDetails{}, err
	}
	job, err := r.store.GetJobExecution(jobID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return domain.JobExecutionDetails{}, domain.ErrJobExecutionNotFound
		}
		return domain.JobExecutionDetails{}, err
	}
	events, err := r.store.ListJobExecutionTimelineEvents(jobID)
	if err != nil {
		return domain.JobExecutionDetails{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.JobExecutionDetails{}, err
	}
	return mapJobExecutionDetails(job, events), nil
}

func mapJobExecutionDetails(job protocol.JobExecution, events []protocol.JobExecutionEvent) domain.JobExecutionDetails {
	details := domain.JobExecutionDetails{
		ID: job.ID, ProjectName: protocol.JobMetadataValue(job, protocol.JobMetadataProject),
		PipelineID: protocol.JobMetadataValue(job, protocol.JobMetadataPipelineID), PipelineJobID: protocol.JobMetadataValue(job, protocol.JobMetadataPipelineJobID),
		MatrixName: protocol.JobMetadataValue(job, protocol.JobMetadataMatrixName), Status: protocol.NormalizeJobExecutionStatus(job.Status),
		CurrentStep: strings.TrimSpace(job.CurrentStep), AgentID: strings.TrimSpace(job.LeasedByAgentID),
		DryRun: protocol.JobMetadataValue(job, protocol.JobMetadataDryRun) == "1", CreatedUTC: job.CreatedUTC,
		StartedUTC: job.StartedUTC, FinishedUTC: job.FinishedUTC, ExitCode: copyInt(job.ExitCode), Error: strings.TrimSpace(job.Error),
	}
	states := timelineStates(events)
	stepsByIndex := make(map[int]protocol.JobStepPlanItem, len(job.StepPlan))
	for _, step := range job.StepPlan {
		if step.Index > 0 {
			stepsByIndex[step.Index] = step
		}
	}
	terminal := protocol.IsTerminalJobExecutionStatus(details.Status)
	for _, item := range protocol.BuildJobExecutionTimeline(job) {
		state := states[item.ID]
		status := state.status
		if status == "" {
			if terminal {
				status = "not reached"
			} else {
				status = "pending"
			}
		}
		timelineItem := domain.JobTimelineItem{
			ID: item.ID, Kind: item.Kind, Name: item.Name, Description: item.Description,
			Index: item.Index, Total: item.Total, Reached: state.reached, Status: status, StartedUTC: state.startedUTC, DurationMS: state.durationMS,
			ExitCode: copyInt(state.exitCode), Error: state.error,
		}
		if item.Kind == "step" {
			if step, ok := stepsByIndex[item.StepIndex]; ok {
				timelineItem.YAMLLiteral = step.YAMLLiteral
				timelineItem.Command = step.Script
			}
		}
		details.Timeline = append(details.Timeline, timelineItem)
	}
	return details
}

type timelineState struct {
	reached    bool
	startedUTC time.Time
	status     string
	durationMS int64
	exitCode   *int
	error      string
}

func timelineStates(events []protocol.JobExecutionEvent) map[string]timelineState {
	states := make(map[string]timelineState)
	for _, event := range events {
		id := ""
		if event.Phase != nil {
			id = strings.TrimSpace(event.Phase.ID)
		} else if event.Step != nil && event.Step.Index > 0 {
			id = fmt.Sprintf("step:%d", event.Step.Index)
		}
		if id == "" {
			continue
		}
		state := states[id]
		state.reached = true
		switch event.Type {
		case protocol.JobExecutionEventTypePhaseStarted, protocol.JobExecutionEventTypeStepStarted:
			state.status = "in progress"
			state.startedUTC = event.TimestampUTC
		case protocol.JobExecutionEventTypePhaseFinished, protocol.JobExecutionEventTypeStepFinished:
			state.status = "succeeded"
			if strings.TrimSpace(event.Error) != "" || (event.ExitCode != nil && *event.ExitCode != 0) {
				state.status = "failed"
			}
			state.durationMS = event.DurationMS
			state.exitCode = copyInt(event.ExitCode)
			state.error = strings.TrimSpace(event.Error)
		}
		states[id] = state
	}
	return states
}

func copyInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type Repository struct {
	store Store
	limit int
}

func NewRepository(store Store, limit int) *Repository {
	if limit <= 0 {
		limit = 40
	}
	return &Repository{store: store, limit: limit}
}

func (r *Repository) ListFrontPageExecutionCards(ctx context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	jobs, err := r.store.ListJobExecutions()
	if err != nil {
		return nil, nil, err
	}
	queued := mapCards(jobhistory.SummaryCards(jobs, true, r.limit))
	history := mapCards(jobhistory.SummaryCards(jobs, false, r.limit))
	return queued, history, nil
}

func mapCards(cards []jobhistory.CardView) []domain.ExecutionCard {
	out := make([]domain.ExecutionCard, 0, len(cards))
	for _, card := range cards {
		out = append(out, domain.ExecutionCard{
			Key: card.Key, Kind: card.Kind, Title: card.Title,
			JobExecutionIDs: append([]string(nil), card.JobExecutionIDs...),
			Summary: domain.ExecutionSummary{
				TotalJobs: card.Summary.TotalJobs, Succeeded: card.Summary.Succeeded,
				Failed: card.Summary.Failed, InProgress: card.Summary.InProgress, Waiting: card.Summary.Waiting,
			},
			Sections: mapCardSections(card.Sections),
		})
	}
	return out
}

func mapCardSections(sections []jobhistory.SectionView) []domain.ExecutionCardSection {
	out := make([]domain.ExecutionCardSection, 0, len(sections))
	for _, section := range sections {
		label := strings.TrimSpace(section.Label)
		if label == "" {
			label = "Execution"
		}
		mapped := domain.ExecutionCardSection{Key: section.Key, Label: label}
		for _, item := range section.Items {
			mapped.Jobs = append(mapped.Jobs, mapCardItemJobs(item)...)
		}
		out = append(out, mapped)
	}
	return out
}

func mapCardItemJobs(item jobhistory.ItemView) []domain.ExecutionCardJob {
	if item.Job != nil {
		label := strings.TrimSpace(item.MatrixLabel)
		if label == "" {
			label = strings.TrimSpace(item.Job.Metadata["pipeline_job_id"])
		}
		if label == "" {
			label = strings.TrimSpace(item.Job.ID)
		}
		return []domain.ExecutionCardJob{{
			ID: item.Job.ID, Label: label, Status: protocol.NormalizeJobExecutionStatus(item.Job.Status),
			CurrentStep: strings.TrimSpace(item.Job.CurrentStep),
		}}
	}
	out := make([]domain.ExecutionCardJob, 0, len(item.Items))
	for _, child := range item.Items {
		out = append(out, mapCardItemJobs(child)...)
	}
	return out
}
