package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation"
)

func TestAgentsViewUsesSharedPresentationContract(t *testing.T) {
	server, state := newTestHTTPServerWithState(t)
	defer server.Close()
	state.mu.Lock()
	state.agents["agent-1"] = agentState{
		Hostname: "builder", OS: "darwin", Arch: "arm64", Version: "v0.2.0", Authorized: true,
		Capabilities: map[string]string{"run_mode": "service"}, LastSeenUTC: time.Now().UTC(),
	}
	state.mu.Unlock()
	response, err := http.Get(server.URL + "/api/v1/views/agents")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var view presentation.AgentsView
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || view.Summary != "1/1 online" || len(view.Agents) != 1 || view.Agents[0].RunMode != "Service" {
		t.Fatalf("status = %d, view = %+v", response.StatusCode, view)
	}
}
