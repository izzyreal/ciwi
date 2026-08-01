package presentation

import (
	"context"

	"github.com/izzyreal/ciwi/internal/domain"
)

type FrontPageView struct {
	Server            domain.ServerInfo
	Projects          []domain.Project
	QueuedExecutions  []domain.ExecutionCard
	HistoryExecutions []domain.ExecutionCard
}

type FrontPageQueries struct {
	server interface {
		GetServerInfo(context.Context) (domain.ServerInfo, error)
	}
	projects interface {
		ListProjects(context.Context) ([]domain.Project, error)
	}
	executions interface {
		ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error)
	}
}

func NewFrontPageQueries(
	server interface {
		GetServerInfo(context.Context) (domain.ServerInfo, error)
	},
	projects interface {
		ListProjects(context.Context) ([]domain.Project, error)
	},
	executions interface {
		ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error)
	},
) *FrontPageQueries {
	return &FrontPageQueries{server: server, projects: projects, executions: executions}
}

func (q *FrontPageQueries) GetFrontPageView(ctx context.Context) (FrontPageView, error) {
	server, err := q.server.GetServerInfo(ctx)
	if err != nil {
		return FrontPageView{}, err
	}
	projects, err := q.projects.ListProjects(ctx)
	if err != nil {
		return FrontPageView{}, err
	}
	queued, history, err := q.executions.ListFrontPageExecutionCards(ctx)
	if err != nil {
		return FrontPageView{}, err
	}
	return FrontPageView{Server: server, Projects: projects, QueuedExecutions: queued, HistoryExecutions: history}, nil
}
