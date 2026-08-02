package application

import (
	"context"
	"errors"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

type projectRepositoryStub struct {
	details domain.ProjectDetails
	err     error
}

func (s projectRepositoryStub) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{}, nil
}

func (s projectRepositoryStub) GetProjectDetails(context.Context, int64) (domain.ProjectDetails, error) {
	return s.details, s.err
}

func TestGetProjectDetailsValidatesAndMapsNotFound(t *testing.T) {
	queries := NewProjectQueries(projectRepositoryStub{})
	if _, err := queries.GetProjectDetails(t.Context(), 0); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("invalid id error = %v", err)
	}
	queries = NewProjectQueries(projectRepositoryStub{err: domain.ErrProjectNotFound})
	if _, err := queries.GetProjectDetails(t.Context(), 7); ErrorKindOf(err) != ErrorNotFound || !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("not found error = %v", err)
	}
}
