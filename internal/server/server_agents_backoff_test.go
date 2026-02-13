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
		agentRollout: agentUpdateRolloutState{
			Slots: make(map[string]int),
		},
		db:           db,
		artifactsDir: artifactsDir,
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

func TestAgentUpdateFirstAttemptDelayBySlot(t *testing.T) {
	if got, want := agentUpdateFirstAttemptDelay(0), 10*time.Second; got != want {
		t.Fatalf("slot 0 delay=%s want=%s", got, want)
	}
	if got, want := agentUpdateFirstAttemptDelay(1), 12*time.Second; got != want {
		t.Fatalf("slot 1 delay=%s want=%s", got, want)
	}
	if got, want := agentUpdateFirstAttemptDelay(2), 14*time.Second; got != want {
		t.Fatalf("slot 2 delay=%s want=%s", got, want)
	}
	if got, want := agentUpdateFirstAttemptDelay(-5), 10*time.Second; got != want {
		t.Fatalf("negative slot delay=%s want=%s", got, want)
	}
}

func TestHeartbeatAutomaticUpdateSchedulesThenDispatchesInProgressAttempt(t *testing.T) {
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
		t.Fatalf("expected first automatic heartbeat update request to be delayed")
	}

	delay := agentUpdateFirstAttemptDelay(0)
	s.mu.Lock()
	state := s.agents["agent-auto"]
	s.mu.Unlock()
	if state.UpdateAttempts != 0 {
		t.Fatalf("expected no attempts before first scheduled request, got %d", state.UpdateAttempts)
	}
	if state.UpdateInProgress {
		t.Fatalf("expected update_in_progress=false before first request")
	}
	if !state.UpdateLastRequestUTC.IsZero() {
		t.Fatalf("expected no last request timestamp before first scheduled request")
	}
	if state.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected first-attempt schedule timestamp to be set")
	}
	minNext := start.Add(delay - 2*time.Second)
	maxNext := time.Now().UTC().Add(delay + 2*time.Second)
	if state.UpdateNextRetryUTC.Before(minNext) || state.UpdateNextRetryUTC.After(maxNext) {
		t.Fatalf("unexpected first-attempt schedule timestamp: got=%s expected around now+%s", state.UpdateNextRetryUTC, delay)
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
		t.Fatalf("expected update request when first-attempt schedule expires")
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
	if !state.UpdateInProgress {
		t.Fatalf("expected update_in_progress=true after request dispatch")
	}
	if state.UpdateLastRequestUTC.IsZero() {
		t.Fatalf("expected last request timestamp to be set")
	}
	if !state.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected no retry schedule while update is in progress, got %s", state.UpdateNextRetryUTC)
	}
}

func TestHeartbeatAutomaticUpdateMarksFailedAttemptAndSchedulesBackoff(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	s := newAgentUpdateTestStateStore(t)
	if err := s.setAgentUpdateTarget("v1.2.0"); err != nil {
		t.Fatalf("set agent update target: %v", err)
	}

	s.mu.Lock()
	s.agents["agent-auto"] = agentState{
		Hostname:             "host-a",
		OS:                   "linux",
		Arch:                 "amd64",
		Version:              "v1.1.0",
		Capabilities:         map[string]string{"executor": "script", "shells": "posix"},
		UpdateTarget:         "v1.2.0",
		UpdateAttempts:       1,
		UpdateInProgress:     true,
		UpdateLastRequestUTC: time.Now().UTC().Add(-agentUpdateInProgressGrace - time.Second),
	}
	s.mu.Unlock()

	resp := heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:      "agent-auto",
		Hostname:     "host-a",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v1.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		TimestampUTC: time.Now().UTC(),
	})
	if resp.UpdateRequested {
		t.Fatalf("expected failed in-progress attempt to enter backoff instead of immediate re-request")
	}

	s.mu.Lock()
	state := s.agents["agent-auto"]
	s.mu.Unlock()
	if state.UpdateAttempts != 1 {
		t.Fatalf("expected attempts to remain 1 after first failure, got %d", state.UpdateAttempts)
	}
	if state.UpdateInProgress {
		t.Fatalf("expected update_in_progress=false after failed attempt")
	}
	if state.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected retry schedule to be set after failed attempt")
	}
	delta := state.UpdateNextRetryUTC.Sub(time.Now().UTC())
	if delta < 25*time.Second || delta > 35*time.Second {
		t.Fatalf("expected first failure backoff around 30s, got %s", delta)
	}
}

func TestHeartbeatAutomaticUpdateUsesReportedFailureReasonAndSchedulesBackoff(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	s := newAgentUpdateTestStateStore(t)
	if err := s.setAgentUpdateTarget("v1.2.0"); err != nil {
		t.Fatalf("set agent update target: %v", err)
	}

	now := time.Now().UTC()
	reason := "download update asset: status=502 body=bad gateway"
	s.mu.Lock()
	s.agents["agent-auto"] = agentState{
		Hostname:             "host-a",
		OS:                   "linux",
		Arch:                 "amd64",
		Version:              "v1.1.0",
		Capabilities:         map[string]string{"executor": "script", "shells": "posix"},
		UpdateTarget:         "v1.2.0",
		UpdateAttempts:       1,
		UpdateInProgress:     true,
		UpdateLastRequestUTC: now.Add(-time.Second),
	}
	s.mu.Unlock()

	resp := heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:       "agent-auto",
		Hostname:      "host-a",
		OS:            "linux",
		Arch:          "amd64",
		Version:       "v1.1.0",
		Capabilities:  map[string]string{"executor": "script", "shells": "posix"},
		UpdateFailure: reason,
		TimestampUTC:  now,
	})
	if resp.UpdateRequested {
		t.Fatalf("expected reported failure to enter backoff before retry")
	}

	s.mu.Lock()
	state := s.agents["agent-auto"]
	s.mu.Unlock()
	if state.UpdateInProgress {
		t.Fatalf("expected update_in_progress=false after reported failure")
	}
	if state.UpdateAttempts != 1 {
		t.Fatalf("expected attempts to remain 1 after reported failure, got %d", state.UpdateAttempts)
	}
	if state.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected retry schedule to be set after reported failure")
	}
	if state.UpdateLastError != reason {
		t.Fatalf("unexpected update_last_error=%q want=%q", state.UpdateLastError, reason)
	}
	if state.UpdateLastErrorUTC.IsZero() {
		t.Fatalf("expected update_last_error_utc to be set")
	}
	delta := state.UpdateNextRetryUTC.Sub(time.Now().UTC())
	if delta < 25*time.Second || delta > 35*time.Second {
		t.Fatalf("expected first failure backoff around 30s, got %s", delta)
	}
}

func TestHeartbeatClearsReportedUpdateFailureAfterAgentReachesTarget(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	s := newAgentUpdateTestStateStore(t)
	if err := s.setAgentUpdateTarget("v1.2.0"); err != nil {
		t.Fatalf("set agent update target: %v", err)
	}

	s.mu.Lock()
	s.agents["agent-auto"] = agentState{
		Hostname:           "host-a",
		OS:                 "linux",
		Arch:               "amd64",
		Version:            "v1.1.0",
		Capabilities:       map[string]string{"executor": "script", "shells": "posix"},
		UpdateTarget:       "v1.2.0",
		UpdateAttempts:     1,
		UpdateInProgress:   false,
		UpdateNextRetryUTC: time.Now().UTC().Add(30 * time.Second),
		UpdateLastError:    "download update asset: status=502 body=bad gateway",
		UpdateLastErrorUTC: time.Now().UTC().Add(-time.Second),
	}
	s.mu.Unlock()

	_ = heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:      "agent-auto",
		Hostname:     "host-a",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v1.2.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		TimestampUTC: time.Now().UTC(),
	})

	s.mu.Lock()
	state := s.agents["agent-auto"]
	s.mu.Unlock()
	if state.UpdateLastError != "" {
		t.Fatalf("expected update_last_error to be cleared after reaching target, got %q", state.UpdateLastError)
	}
	if !state.UpdateLastErrorUTC.IsZero() {
		t.Fatalf("expected update_last_error_utc to be cleared after reaching target, got %s", state.UpdateLastErrorUTC)
	}
}

func TestHeartbeatAutomaticUpdatePhasesAgentsByTwoSecondSlots(t *testing.T) {
	oldVersion := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = oldVersion })

	s := newAgentUpdateTestStateStore(t)
	if err := s.setAgentUpdateTarget("v1.2.0"); err != nil {
		t.Fatalf("set agent update target: %v", err)
	}

	first := heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:      "agent-1",
		Hostname:     "host-1",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v1.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		TimestampUTC: time.Now().UTC(),
	})
	if first.UpdateRequested {
		t.Fatalf("expected first request for agent-1 to be scheduled, not immediate")
	}

	second := heartbeatForTest(t, s, protocol.HeartbeatRequest{
		AgentID:      "agent-2",
		Hostname:     "host-2",
		OS:           "linux",
		Arch:         "amd64",
		Version:      "v1.1.0",
		Capabilities: map[string]string{"executor": "script", "shells": "posix"},
		TimestampUTC: time.Now().UTC(),
	})
	if second.UpdateRequested {
		t.Fatalf("expected first request for agent-2 to be scheduled, not immediate")
	}

	s.mu.Lock()
	a1 := s.agents["agent-1"]
	a2 := s.agents["agent-2"]
	s.mu.Unlock()
	if a1.UpdateNextRetryUTC.IsZero() || a2.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected both agents to have first-attempt schedule timestamps")
	}
	diff := a2.UpdateNextRetryUTC.Sub(a1.UpdateNextRetryUTC)
	if diff < 1500*time.Millisecond || diff > 2500*time.Millisecond {
		t.Fatalf("expected phase difference near 2s, got %s", diff)
	}
}

func TestHeartbeatManualUpdateBypassesFirstAttemptSchedule(t *testing.T) {
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
	if !state.UpdateInProgress {
		t.Fatalf("expected update_in_progress=true after manual request dispatch")
	}
	if !state.UpdateNextRetryUTC.IsZero() {
		t.Fatalf("expected no retry schedule while manual update is in progress")
	}
}
