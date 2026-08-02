package presentation

import (
	"context"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
)

type presentationAgentRepository struct {
	agents []domain.Agent
}

func (s presentationAgentRepository) ListAgents(_ context.Context) ([]domain.Agent, error) {
	return s.agents, nil
}

func TestAgentsViewDecoratesAndSortsAgentState(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	queries := NewAgentsQueries(application.NewAgentQueries(presentationAgentRepository{agents: []domain.Agent{
		{ID: "z-offline", LastSeenUTC: now.Add(-2 * time.Minute)},
		{ID: "a-online", Hostname: "builder", OS: "darwin", Arch: "arm64", Version: "v0.2.0", Authorized: true, LastSeenUTC: now.Add(-5 * time.Second), Capabilities: map[string]string{"run_mode": "service", "shell": "zsh"}},
	}}))
	queries.now = func() time.Time { return now }
	view, err := queries.GetAgentsView(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view.Summary != "1/2 online" || len(view.Agents) != 2 || view.Agents[0].ID != "a-online" || view.Agents[0].Status != "online" || view.Agents[0].RunMode != "Service" || view.Agents[1].Status != "offline" {
		t.Fatalf("view = %+v", view)
	}
}
