package presentation

import (
	"context"
	"errors"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

type serverSourceStub struct{}

func (serverSourceStub) GetServerInfo(context.Context) (domain.ServerInfo, error) {
	return domain.ServerInfo{Name: "ciwi", Version: "v0.2.0"}, nil
}

type failingProjectSourceStub struct{}

func (failingProjectSourceStub) ListProjects(context.Context) ([]domain.Project, error) {
	return nil, errors.New("project read failed")
}

type projectSourceStub struct{}

func (projectSourceStub) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{{ID: 1, Name: "ciwi"}}, nil
}

type executionSourceStub struct{}

func (executionSourceStub) ListFrontPageExecutionCards(context.Context) ([]domain.ExecutionCard, []domain.ExecutionCard, error) {
	return []domain.ExecutionCard{{Key: "queued"}}, []domain.ExecutionCard{{Key: "history"}}, nil
}

func TestFrontPageQueriesComposeOneRendererView(t *testing.T) {
	view, err := NewFrontPageQueries(serverSourceStub{}, projectSourceStub{}, executionSourceStub{}).GetFrontPageView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.Server.Name != "ciwi" || len(view.Projects) != 1 || len(view.QueuedExecutions) != 1 || len(view.HistoryExecutions) != 1 {
		t.Fatalf("view = %+v", view)
	}
}

func TestFrontPageQueriesReportsPhaseTimingAndFailure(t *testing.T) {
	var success FrontPageTiming
	queries := NewFrontPageQueriesWithObserver(serverSourceStub{}, projectSourceStub{}, executionSourceStub{}, func(_ context.Context, timing FrontPageTiming) {
		success = timing
	})
	if _, err := queries.GetFrontPageView(t.Context()); err != nil {
		t.Fatal(err)
	}
	if success.ProjectCount != 1 || success.QueuedCardCount != 1 || success.HistoryCardCount != 1 || success.FailedPhase != "" || success.Err != nil {
		t.Fatalf("success timing = %+v", success)
	}

	var failure FrontPageTiming
	queries = NewFrontPageQueriesWithObserver(serverSourceStub{}, failingProjectSourceStub{}, executionSourceStub{}, func(_ context.Context, timing FrontPageTiming) {
		failure = timing
	})
	if _, err := queries.GetFrontPageView(t.Context()); err == nil {
		t.Fatal("expected project failure")
	}
	if failure.FailedPhase != "projects" || failure.Err == nil || failure.Executions != 0 {
		t.Fatalf("failure timing = %+v", failure)
	}
}
