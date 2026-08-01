package application

import (
	"context"

	"github.com/izzyreal/ciwi/internal/domain"
)

type ExecutionCardRepository interface {
	ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error)
}

type ExecutionQueries struct {
	repository ExecutionCardRepository
}

func NewExecutionQueries(repository ExecutionCardRepository) *ExecutionQueries {
	return &ExecutionQueries{repository: repository}
}

func (q *ExecutionQueries) ListFrontPageExecutionCards(ctx context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error) {
	if q == nil || q.repository == nil {
		return nil, nil, NewError(ErrorUnavailable, "execution repository unavailable", nil)
	}
	queued, history, err := q.repository.ListFrontPageExecutionCards(ctx)
	if err != nil {
		return nil, nil, WrapInternal("list front-page execution cards", err)
	}
	return queued, history, nil
}
