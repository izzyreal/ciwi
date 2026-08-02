package application

import (
	"context"
	"errors"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
)

type ExecutionCardRepository interface {
	ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error)
	GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error)
}

func (q *ExecutionQueries) GetJobExecutionDetails(ctx context.Context, jobID string) (domain.JobExecutionDetails, error) {
	if q == nil || q.repository == nil {
		return domain.JobExecutionDetails{}, NewError(ErrorUnavailable, "execution repository unavailable", nil)
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return domain.JobExecutionDetails{}, NewError(ErrorInvalidArgument, "job execution id is required", nil)
	}
	details, err := q.repository.GetJobExecutionDetails(ctx, jobID)
	if errors.Is(err, domain.ErrJobExecutionNotFound) {
		return domain.JobExecutionDetails{}, NewError(ErrorNotFound, "job execution not found", err)
	}
	if err != nil {
		return domain.JobExecutionDetails{}, WrapInternal("get job execution details", err)
	}
	return details, nil
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
