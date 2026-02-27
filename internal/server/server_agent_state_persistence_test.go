package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/izzyreal/ciwi/internal/store"
)

func TestHydrateAgentStateFromAppStateRestoresSnapshot(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "ciwi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s1 := &stateStore{
		agents:            make(map[string]agentState),
		agentUpdates:      make(map[string]string),
		agentToolRefresh:  make(map[string]bool),
		agentRestarts:     make(map[string]bool),
		agentCacheWipes:   make(map[string]bool),
		agentHistoryWipes: make(map[string]bool),
		agentDeactivated:  make(map[string]bool),
		db:                db,
	}

	hbBody, _ := json.Marshal(map[string]any{
		"agent_id":      "agent-persisted",
		"hostname":      "persist-host",
		"os":            "linux",
		"arch":          "amd64",
		"version":       "v1.0.0",
		"capabilities":  map[string]string{"executor": "script", "shells": "posix"},
		"timestamp_utc": "2026-02-27T00:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", bytes.NewReader(hbBody))
	rec := httptest.NewRecorder()
	s1.heartbeatHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := db.SetAppState(agentDeactivatedStateKey("agent-persisted"), "1"); err != nil {
		t.Fatalf("set deactivated app state: %v", err)
	}

	appState, err := db.ListAppState()
	if err != nil {
		t.Fatalf("list app state: %v", err)
	}

	s2 := &stateStore{
		agents:           make(map[string]agentState),
		agentDeactivated: make(map[string]bool),
		db:               db,
	}
	s2.hydrateAgentStateFromAppState(appState)

	got, ok := s2.agents["agent-persisted"]
	if !ok {
		t.Fatalf("expected persisted agent snapshot to be hydrated")
	}
	if got.Hostname != "persist-host" {
		t.Fatalf("unexpected hostname: %q", got.Hostname)
	}
	if got.OS != "linux" || got.Arch != "amd64" {
		t.Fatalf("unexpected platform: %s/%s", got.OS, got.Arch)
	}
	if !got.Deactivated {
		t.Fatalf("expected hydrated agent to be deactivated from app_state override")
	}
}
