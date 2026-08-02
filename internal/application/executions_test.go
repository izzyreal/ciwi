package application

import (
	"context"
	"errors"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

type executionRepositoryStub struct {
	details domain.JobExecutionDetails
	err     error
}

func (s executionRepositoryStub) ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error) {
	return nil, nil, nil
}

func (s executionRepositoryStub) GetJobExecutionDetails(context.Context, string) (domain.JobExecutionDetails, error) {
	return s.details, s.err
}

func (s executionRepositoryStub) ListJobOutputAfter(context.Context, string, int64) (domain.JobOutputBatch, error) {
	return domain.JobOutputBatch{}, s.err
}

func TestGetJobExecutionDetailsValidatesAndMapsNotFound(t *testing.T) {
	queries := NewExecutionQueries(executionRepositoryStub{})
	if _, err := queries.GetJobExecutionDetails(t.Context(), " "); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("invalid id error = %v", err)
	}
	queries = NewExecutionQueries(executionRepositoryStub{err: domain.ErrJobExecutionNotFound})
	if _, err := queries.GetJobExecutionDetails(t.Context(), "missing"); ErrorKindOf(err) != ErrorNotFound || !errors.Is(err, domain.ErrJobExecutionNotFound) {
		t.Fatalf("not found error = %v", err)
	}
}

func TestGetJobOutputValidatesCursor(t *testing.T) {
	queries := NewExecutionQueries(executionRepositoryStub{})
	if _, err := queries.GetJobOutput(t.Context(), "job-1", -1); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("invalid cursor error = %v", err)
	}
}
