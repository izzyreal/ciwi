package presentation

import (
	"context"
	"time"

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
	observer FrontPageObserver
}

type FrontPageTiming struct {
	ServerInfo       time.Duration
	Projects         time.Duration
	Executions       time.Duration
	Total            time.Duration
	ProjectCount     int
	QueuedCardCount  int
	HistoryCardCount int
	FailedPhase      string
	Err              error
}

type FrontPageObserver func(context.Context, FrontPageTiming)

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
	return NewFrontPageQueriesWithObserver(server, projects, executions, nil)
}

func NewFrontPageQueriesWithObserver(
	server interface {
		GetServerInfo(context.Context) (domain.ServerInfo, error)
	},
	projects interface {
		ListProjects(context.Context) ([]domain.Project, error)
	},
	executions interface {
		ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error)
	},
	observer FrontPageObserver,
) *FrontPageQueries {
	return &FrontPageQueries{server: server, projects: projects, executions: executions, observer: observer}
}

func (q *FrontPageQueries) GetFrontPageView(ctx context.Context) (FrontPageView, error) {
	started := time.Now()
	timing := FrontPageTiming{}
	defer func() {
		timing.Total = time.Since(started)
		if q.observer != nil {
			q.observer(ctx, timing)
		}
	}()
	phaseStarted := time.Now()
	server, err := q.server.GetServerInfo(ctx)
	timing.ServerInfo = time.Since(phaseStarted)
	if err != nil {
		timing.FailedPhase, timing.Err = "server_info", err
		return FrontPageView{}, err
	}
	phaseStarted = time.Now()
	projects, err := q.projects.ListProjects(ctx)
	timing.Projects = time.Since(phaseStarted)
	timing.ProjectCount = len(projects)
	if err != nil {
		timing.FailedPhase, timing.Err = "projects", err
		return FrontPageView{}, err
	}
	phaseStarted = time.Now()
	queued, history, err := q.executions.ListFrontPageExecutionCards(ctx)
	timing.Executions = time.Since(phaseStarted)
	timing.QueuedCardCount, timing.HistoryCardCount = len(queued), len(history)
	if err != nil {
		timing.FailedPhase, timing.Err = "executions", err
		return FrontPageView{}, err
	}
	now := time.Now().UTC()
	presentFrontPageProgress(queued, now)
	presentFrontPageProgress(history, now)
	return FrontPageView{Server: server, Projects: projects, QueuedExecutions: queued, HistoryExecutions: history}, nil
}
