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
	ListJobOutputAfter(context.Context, string, int64) (domain.JobOutputBatch, error)
}

type executionJobLogRepository interface {
	GetJobLogDescriptor(context.Context, string) (domain.JobLogDescriptor, error)
	GetJobLogPage(context.Context, string, string, domain.JobLogPageMode, int64) (domain.JobLogPage, error)
	SearchJobLog(context.Context, string, string, int64) (domain.JobLogSearchResult, error)
}

func (q *ExecutionQueries) GetJobLogDescriptor(ctx context.Context, jobID string) (domain.JobLogDescriptor, error) {
	if q == nil || q.repository == nil {
		return domain.JobLogDescriptor{}, NewError(ErrorUnavailable, "execution repository unavailable", nil)
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return domain.JobLogDescriptor{}, NewError(ErrorInvalidArgument, "job execution id is required", nil)
	}
	repository, ok := q.repository.(executionJobLogRepository)
	if !ok {
		return domain.JobLogDescriptor{}, NewError(ErrorUnavailable, "job log repository unavailable", nil)
	}
	descriptor, err := repository.GetJobLogDescriptor(ctx, jobID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return domain.JobLogDescriptor{}, NewError(ErrorNotFound, "job execution not found", err)
		}
		return domain.JobLogDescriptor{}, WrapInternal("get job log descriptor", err)
	}
	return descriptor, nil
}

func (q *ExecutionQueries) GetJobLogPage(ctx context.Context, jobID, itemID string, mode domain.JobLogPageMode, cursor int64) (domain.JobLogPage, error) {
	if q == nil || q.repository == nil {
		return domain.JobLogPage{}, NewError(ErrorUnavailable, "execution repository unavailable", nil)
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || cursor < 0 {
		return domain.JobLogPage{}, NewError(ErrorInvalidArgument, "valid job execution id and cursor are required", nil)
	}
	repository, ok := q.repository.(executionJobLogRepository)
	if !ok {
		return domain.JobLogPage{}, NewError(ErrorUnavailable, "job log repository unavailable", nil)
	}
	page, err := repository.GetJobLogPage(ctx, jobID, itemID, mode, cursor)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "not found") {
			return domain.JobLogPage{}, NewError(ErrorNotFound, "job execution not found", err)
		}
		if strings.Contains(message, "cursor") || strings.Contains(message, "mode") || strings.Contains(message, "legacy") {
			return domain.JobLogPage{}, NewError(ErrorInvalidArgument, err.Error(), err)
		}
		return domain.JobLogPage{}, WrapInternal("get job log page", err)
	}
	return page, nil
}

func (q *ExecutionQueries) SearchJobLog(ctx context.Context, jobID, query string, selectedIndex int64) (domain.JobLogSearchResult, error) {
	if q == nil || q.repository == nil {
		return domain.JobLogSearchResult{}, NewError(ErrorUnavailable, "execution repository unavailable", nil)
	}
	repository, ok := q.repository.(executionJobLogRepository)
	if !ok {
		return domain.JobLogSearchResult{}, NewError(ErrorUnavailable, "job log repository unavailable", nil)
	}
	result, err := repository.SearchJobLog(ctx, strings.TrimSpace(jobID), query, selectedIndex)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "not found") {
			return domain.JobLogSearchResult{}, NewError(ErrorNotFound, "job execution not found", err)
		}
		if strings.Contains(message, "query") || strings.Contains(message, "index") || strings.Contains(message, "legacy") {
			return domain.JobLogSearchResult{}, NewError(ErrorInvalidArgument, err.Error(), err)
		}
		return domain.JobLogSearchResult{}, WrapInternal("search job log", err)
	}
	return result, nil
}

func (q *ExecutionQueries) GetJobOutput(ctx context.Context, jobID string, afterEventID int64) (domain.JobOutputBatch, error) {
	if q == nil || q.repository == nil {
		return domain.JobOutputBatch{}, NewError(ErrorUnavailable, "execution repository unavailable", nil)
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return domain.JobOutputBatch{}, NewError(ErrorInvalidArgument, "job execution id is required", nil)
	}
	if afterEventID < 0 {
		return domain.JobOutputBatch{}, NewError(ErrorInvalidArgument, "after event id must be non-negative", nil)
	}
	batch, err := q.repository.ListJobOutputAfter(ctx, jobID, afterEventID)
	if errors.Is(err, domain.ErrJobExecutionNotFound) {
		return domain.JobOutputBatch{}, NewError(ErrorNotFound, "job execution not found", err)
	}
	if err != nil {
		return domain.JobOutputBatch{}, WrapInternal("get job output", err)
	}
	return batch, nil
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
