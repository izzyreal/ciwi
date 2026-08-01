package application

import (
	"context"

	"github.com/izzyreal/ciwi/internal/domain"
)

type ProjectRepository interface {
	ListProjects(context.Context) ([]domain.Project, error)
}

type ProjectQueries struct {
	repository ProjectRepository
}

func NewProjectQueries(repository ProjectRepository) *ProjectQueries {
	return &ProjectQueries{repository: repository}
}

func (q *ProjectQueries) ListProjects(ctx context.Context) ([]domain.Project, error) {
	if q == nil || q.repository == nil {
		return nil, NewError(ErrorUnavailable, "project repository unavailable", nil)
	}
	projects, err := q.repository.ListProjects(ctx)
	if err != nil {
		return nil, WrapInternal("list projects", err)
	}
	if projects == nil {
		projects = []domain.Project{}
	}
	return projects, nil
}
