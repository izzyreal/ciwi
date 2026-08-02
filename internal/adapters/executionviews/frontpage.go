package executionviews

import (
	"context"
	"fmt"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobhistory"
)

type Store interface {
	ListJobExecutions() ([]protocol.JobExecution, error)
	GetJobExecution(string) (protocol.JobExecution, error)
	ListJobExecutionTimelineEvents(string) ([]protocol.JobExecutionEvent, error)
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
		details.Timeline = append(details.Timeline, domain.JobTimelineItem{
			ID: item.ID, Kind: item.Kind, Name: item.Name, Description: item.Description,
			Index: item.Index, Total: item.Total, Status: status, DurationMS: state.durationMS,
			ExitCode: copyInt(state.exitCode), Error: state.error,
		})
	}
	return details
}

type timelineState struct {
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
		switch event.Type {
		case protocol.JobExecutionEventTypePhaseStarted, protocol.JobExecutionEventTypeStepStarted:
			state.status = "in progress"
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
		})
	}
	return out
}
