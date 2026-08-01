package executionviews

import (
	"context"

	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobhistory"
)

type Store interface {
	ListJobExecutions() ([]protocol.JobExecution, error)
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
