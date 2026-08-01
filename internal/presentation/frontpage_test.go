package presentation

import (
	"context"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

type serverSourceStub struct{}

func (serverSourceStub) GetServerInfo(context.Context) (domain.ServerInfo, error) {
	return domain.ServerInfo{Name: "ciwi", Version: "v0.2.0"}, nil
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
