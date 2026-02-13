package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/store"
	"github.com/izzyreal/ciwi/internal/version"
)

func newAgentUpdateTestStateStore(t *testing.T) *stateStore {
	t.Helper()
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	artifactsDir := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("create artifacts dir: %v", err)
	}
	return &stateStore{
		agents:           make(map[string]agentState),
		agentUpdates:     make(map[string]string),
		agentToolRefresh: make(map[string]bool),
		db:               db,
		artifactsDir:     artifactsDir,
	}
}

func heartbeatForTest(t *testing.T, s *stateStore, req protocol.HeartbeatRequest) protocol.HeartbeatResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal heartbeat request: %v", err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.heartbeatHandler(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp protocol.HeartbeatResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode heartbeat response: %v", err)
	}
	return resp
}

func TestAgentUpdateInitialWarmupDeterministicAndBounded(t *testing.T) {
	a := agentUpdateInitialWarmup("agent-a", "v1.2.3")
	b := agentUpdateInitialWarmup("agent-a", "v1.2.3")
	if a != b {
		t.Fatalf("expected deterministic warmup delay, got %s and %s", a, b)
	}
	min := agentUpdateInitialWarmupBase
	max := agentUpdateInitialWarmupBase + agentUpdateInitialWarmupJitter
	if a < min || a > max {
		t.Fatalf("warmup delay out of range: got=%s min=%s max=%s", a, min, max)
	}
}

func TestHeartbeatAutomaticUpdateUsesWarmupBeforeFirstRequest(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	s := newAgentUpdateTestStateStore(t)
	if err := s.setAgentUpdateTarget("v1.2.0"); err != nil {
		t.Fatalf("set agent update target: %v", err)
	}

	start := time.Now().UTC()
	first := heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:      "agent-auto",
		Hostname:     "host-a",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v1.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		TimestampUTC: start,
	})
	if first.UpdateRequested {
		t.Fatalf("expected first automatic heartbeat update request to be delayed by warmup")
	}

	delay := agentUpdateInitialWarmup("agent-auto", "v1.2.0")
	s.mu.Lock()
	state := s.agents["agent-auto"]
	s.mu.Unlock()
	if state.UpdateAttempts != 0 {
		t.Fatalf("expected no attempts during warmup, got %d", state.UpdateAttempts)
	}
	if !state.UpdateLastRequestUTC.IsZero() {
		t.Fatalf("expected no last request timestamp during warmup")
	}
	if state.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected warmup next retry timestamp to be set")
	}
	minNext := start.Add(delay - 2*time.Second)
	maxNext := time.Now().UTC().Add(delay + 2*time.Second)
	if state.UpdateNextRetryUTC.Before(minNext) || state.UpdateNextRetryUTC.After(maxNext) {
		t.Fatalf("unexpected warmup next retry timestamp: got=%s expected around now+%s", state.UpdateNextRetryUTC, delay)
	}

	s.mu.Lock()
	state.UpdateNextRetryUTC = time.Now().UTC().Add(-time.Second)
	s.agents["agent-auto"] = state
	s.mu.Unlock()

	second := heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:      "agent-auto",
		Hostname:     "host-a",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v1.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		TimestampUTC: start.Add(10 * time.Second),
	})
	if !second.UpdateRequested {
		t.Fatalf("expected update request after warmup expires")
	}
	if second.UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected update target: %q", second.UpdateTarget)
	}

	s.mu.Lock()
	state = s.agents["agent-auto"]
	s.mu.Unlock()
	if state.UpdateAttempts != 1 {
		t.Fatalf("expected attempts=1 after first request, got %d", state.UpdateAttempts)
	}
	if state.UpdateLastRequestUTC.IsZero() {
		t.Fatalf("expected last request timestamp to be set")
	}
}

func TestHeartbeatManualUpdateBypassesWarmupDelay(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	s := newAgentUpdateTestStateStore(t)
	s.mu.Lock()
	s.agentUpdates["agent-manual"] = "v1.2.0"
	s.mu.Unlock()

	resp := heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:      "agent-manual",
		Hostname:     "host-m",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v1.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		TimestampUTC: time.Now().UTC(),
	})
	if !resp.UpdateRequested {
		t.Fatalf("expected manual update to be requested immediately")
	}
	if resp.UpdateTarget != "v1.2.0" {
		t.Fatalf("unexpected manual update target: %q", resp.UpdateTarget)
	}

	s.mu.Lock()
	state := s.agents["agent-manual"]
	s.mu.Unlock()
	if state.UpdateAttempts != 1 {
		t.Fatalf("expected attempts=1 for immediate manual update, got %d", state.UpdateAttempts)
	}
	if state.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected retry timestamp after manual request")
	}
}
