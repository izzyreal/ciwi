package application

import (
	"context"
	"errors"

	"github.com/izzyreal/ciwi/internal/domain"
)

type ProjectRepository interface {
	ListProjects(context.Context) ([]domain.Project, error)
	GetProjectDetails(context.Context, int64) (domain.ProjectDetails, error)
}

func (q *ProjectQueries) GetProjectDetails(ctx context.Context, projectID int64) (domain.ProjectDetails, error) {
	if projectID <= 0 {
		return domain.ProjectDetails{}, NewError(ErrorInvalidArgument, "project id must be positive", nil)
	}
	if q == nil || q.repository == nil {
		return domain.ProjectDetails{}, NewError(ErrorUnavailable, "project repository unavailable", nil)
	}
	details, err := q.repository.GetProjectDetails(ctx, projectID)
	if errors.Is(err, domain.ErrProjectNotFound) {
		return domain.ProjectDetails{}, NewError(ErrorNotFound, "project not found", err)
	}
	if err != nil {
		return domain.ProjectDetails{}, WrapInternal("get project details", err)
	}
	return details, nil
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
