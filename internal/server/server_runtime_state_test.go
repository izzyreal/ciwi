package server

import (
	"net/http"
	"testing"
	"time"
)

func TestRuntimeStateHandlerAndComputation(t *testing.T) {
	ts, s := newTestHTTPServerWithState(t)
	defer ts.Close()

	origLookPath := execLookPath
	execLookPath = func(file string) (string, error) { return "/usr/bin/git", nil }
	t.Cleanup(func() { execLookPath = origLookPath })

	resp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/runtime-state", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("runtime-state status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var payload struct {
		Mode         string `json:"mode"`
		OnlineAgents int    `json:"online_agents"`
	}
	decodeJSONBody(t, resp, &payload)
	if payload.Mode != "normal" {
		t.Fatalf("expected mode=normal with no registered agents, got %q", payload.Mode)
	}
	if payload.OnlineAgents != 0 {
		t.Fatalf("expected online_agents=0, got %d", payload.OnlineAgents)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.agents["agent-old"] = agentState{
		OS:          "linux",
		Arch:        "amd64",
		LastSeenUTC: now.Add(-2 * time.Minute),
	}
	s.mu.Unlock()
	resp = mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/runtime-state", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("runtime-state status=%d body=%s", resp.StatusCode, readBody(t, resp))
	}
	var degraded struct {
		Mode         string   `json:"mode"`
		Reasons      []string `json:"reasons"`
		OnlineAgents int      `json:"online_agents"`
		Offline      int      `json:"offline_agents"`
	}
	decodeJSONBody(t, resp, &degraded)
	if degraded.Mode != "degraded_offline" {
		t.Fatalf("expected degraded_offline mode, got %q", degraded.Mode)
	}
	if degraded.OnlineAgents != 0 || degraded.Offline != 1 {
		t.Fatalf("unexpected counters: %+v", degraded)
	}
}
