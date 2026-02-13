package server

import (
	"net/http"
	"testing"

	"github.com/izzyreal/ciwi/internal/version"
)

func TestHeartbeatDoesNotRequestAgentUpdate(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()

	hbResp := mustJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	var hbPayload struct {
		Accepted        bool   `json:"accepted"`
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
	}
	decodeJSONBody(t, hbResp, &hbPayload)
	if !hbPayload.Accepted {
		t.Fatalf("expected accepted=true")
	}
	if hbPayload.UpdateRequested {
		t.Fatalf("expected update_requested=false")
	}
	if hbPayload.UpdateTarget != "" {
		t.Fatalf("unexpected update_target: %q", hbPayload.UpdateTarget)
	}

	agentsResp := mustJSONRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if agentsResp.StatusCode != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", agentsResp.StatusCode, readBody(t, agentsResp))
	}
	var agentsPayload struct {
		Agents []struct {
			AgentID string `json:"agent_id"`
			Version string `json:"version"`
		} `json:"agents"`
	}
	decodeJSONBody(t, agentsResp, &agentsPayload)
	if len(agentsPayload.Agents) != 1 {
		t.Fatalf("expected exactly one agent, got %d", len(agentsPayload.Agents))
	}
	if agentsPayload.Agents[0].AgentID != "agent-a" || agentsPayload.Agents[0].Version != "v1.1.0" {
		t.Fatalf("unexpected agent payload: %+v", agentsPayload.Agents[0])
	}
}

func TestGetAgentByIDEndpoint(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-by-id",
		"hostname":      "host-z",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	getResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/agent-by-id", nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get agent status=%d body=%s", getResp.StatusCode, readBody(t, getResp))
	}
	var payload struct {
		Agent struct {
			AgentID     string `json:"agent_id"`
			Hostname    string `json:"hostname"`
			Version     string `json:"version"`
			NeedsUpdate bool   `json:"needs_update"`
		} `json:"agent"`
	}
	decodeJSONBody(t, getResp, &payload)
	if payload.Agent.AgentID != "agent-by-id" {
		t.Fatalf("unexpected agent id: %q", payload.Agent.AgentID)
	}
	if payload.Agent.Hostname != "host-z" {
		t.Fatalf("unexpected hostname: %q", payload.Agent.Hostname)
	}
	if payload.Agent.Version != "v1.1.0" {
		t.Fatalf("unexpected version: %q", payload.Agent.Version)
	}
	if !payload.Agent.NeedsUpdate {
		t.Fatalf("expected needs_update=true")
	}

	missingResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/does-not-exist", nil)
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing agent status=%d body=%s", missingResp.StatusCode, readBody(t, missingResp))
	}
	_ = readBody(t, missingResp)
}

func TestAgentListAndDetailUseConsistentViewFields(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	hbResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-consistency",
		"hostname":      "host-c",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbResp.StatusCode, readBody(t, hbResp))
	}
	_ = readBody(t, hbResp)

	updateResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-consistency/update", map[string]any{})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("manual update status=%d body=%s", updateResp.StatusCode, readBody(t, updateResp))
	}
	_ = readBody(t, updateResp)

	type agentCore struct {
		AgentID         string `json:"agent_id"`
		Hostname        string `json:"hostname"`
		Version         string `json:"version"`
		NeedsUpdate     bool   `json:"needs_update"`
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
		UpdateAttempts  int    `json:"update_attempts"`
	}

	listResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("agents list status=%d body=%s", listResp.StatusCode, readBody(t, listResp))
	}
	var listPayload struct {
		Agents []agentCore `json:"agents"`
	}
	decodeJSONBody(t, listResp, &listPayload)
	if len(listPayload.Agents) != 1 {
		t.Fatalf("expected 1 agent in list, got %d", len(listPayload.Agents))
	}

	detailResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents/agent-consistency", nil)
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("agent detail status=%d body=%s", detailResp.StatusCode, readBody(t, detailResp))
	}
	var detailPayload struct {
		Agent agentCore `json:"agent"`
	}
	decodeJSONBody(t, detailResp, &detailPayload)

	listAgent := listPayload.Agents[0]
	detailAgent := detailPayload.Agent
	if listAgent.AgentID != detailAgent.AgentID {
		t.Fatalf("agent_id mismatch list=%q detail=%q", listAgent.AgentID, detailAgent.AgentID)
	}
	if listAgent.Hostname != detailAgent.Hostname {
		t.Fatalf("hostname mismatch list=%q detail=%q", listAgent.Hostname, detailAgent.Hostname)
	}
	if listAgent.Version != detailAgent.Version {
		t.Fatalf("version mismatch list=%q detail=%q", listAgent.Version, detailAgent.Version)
	}
	if listAgent.NeedsUpdate != detailAgent.NeedsUpdate {
		t.Fatalf("needs_update mismatch list=%v detail=%v", listAgent.NeedsUpdate, detailAgent.NeedsUpdate)
	}
	if listAgent.UpdateRequested != detailAgent.UpdateRequested {
		t.Fatalf("update_requested mismatch list=%v detail=%v", listAgent.UpdateRequested, detailAgent.UpdateRequested)
	}
	if listAgent.UpdateTarget != detailAgent.UpdateTarget {
		t.Fatalf("update_target mismatch list=%q detail=%q", listAgent.UpdateTarget, detailAgent.UpdateTarget)
	}
	if listAgent.UpdateAttempts != detailAgent.UpdateAttempts {
		t.Fatalf("update_attempts mismatch list=%d detail=%d", listAgent.UpdateAttempts, detailAgent.UpdateAttempts)
	}
}

func TestManualAgentUpdateRequestTriggersHeartbeatUpdate(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "darwin",
		"arch":          "arm64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	manualResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-a/update", map[string]any{})
	if manualResp.StatusCode != http.StatusOK {
		t.Fatalf("manual update status=%d body=%s", manualResp.StatusCode, readBody(t, manualResp))
	}
	_ = readBody(t, manualResp)

	agentsResp := mustJSONRequest(t, client, http.MethodGet, ts.URL+"/api/v1/agents", nil)
	if agentsResp.StatusCode != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", agentsResp.StatusCode, readBody(t, agentsResp))
	}
	var agentsPayload struct {
		Agents []struct {
			AgentID         string `json:"agent_id"`
			UpdateRequested bool   `json:"update_requested"`
			UpdateTarget    string `json:"update_target"`
		} `json:"agents"`
	}
	decodeJSONBody(t, agentsResp, &agentsPayload)
	if len(agentsPayload.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agentsPayload.Agents))
	}
	if !agentsPayload.Agents[0].UpdateRequested {
		t.Fatalf("expected update_requested=true on agents list")
	}
	if agentsPayload.Agents[0].UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected update_target in agents list: %q", agentsPayload.Agents[0].UpdateTarget)
	}

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-a",
		"hostname":      "host-a",
		"os":            "darwin",
		"arch":          "arm64",
		"version":       "v1.1.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var hbPayload struct {
		UpdateRequested bool   `json:"update_requested"`
		UpdateTarget    string `json:"update_target"`
	}
	decodeJSONBody(t, secondHB, &hbPayload)
	if !hbPayload.UpdateRequested {
		t.Fatalf("expected update_requested=true")
	}
	if hbPayload.UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected update_target: %q", hbPayload.UpdateTarget)
	}
}

func TestManualRefreshToolsRequest(t *testing.T) {
	ts := newTestHTTPServer(t)
	defer ts.Close()
	client := ts.Client()

	firstHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-refresh",
		"hostname":      "host-r",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:00Z",
	})
	if firstHB.StatusCode != http.StatusOK {
		t.Fatalf("first heartbeat status=%d body=%s", firstHB.StatusCode, readBody(t, firstHB))
	}
	_ = readBody(t, firstHB)

	refreshResp := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/agents/agent-refresh/refresh-tools", map[string]any{})
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("refresh-tools status=%d body=%s", refreshResp.StatusCode, readBody(t, refreshResp))
	}
	_ = readBody(t, refreshResp)

	secondHB := mustJSONRequest(t, client, http.MethodPost, ts.URL+"/api/v1/heartbeat", map[string]any{
		"agent_id":      "agent-refresh",
		"hostname":      "host-r",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-11T00:00:10Z",
	})
	if secondHB.StatusCode != http.StatusOK {
		t.Fatalf("second heartbeat status=%d body=%s", secondHB.StatusCode, readBody(t, secondHB))
	}
	var hbPayload struct {
		RefreshToolsRequested bool `json:"refresh_tools_requested"`
	}
	decodeJSONBody(t, secondHB, &hbPayload)
	if !hbPayload.RefreshToolsRequested {
		t.Fatalf("expected refresh_tools_requested=true")
	}
}
