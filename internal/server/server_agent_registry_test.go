package server

import (
	"testing"
	"time"
)

func TestAgentRegistrySnapshotsAreDefensive(t *testing.T) {
	registry := newAgentRegistry()
	registry.agents["agent-1"] = agentState{
		Capabilities: map[string]string{"os": "linux"},
		RecentLog:    []string{"registered"},
	}
	registry.agentUpdates["agent-1"] = " v1.2.3 "

	snapshots := registry.snapshots()
	if len(snapshots) != 1 || snapshots[0].PendingUpdate != "v1.2.3" {
		t.Fatalf("unexpected snapshots: %+v", snapshots)
	}
	snapshots[0].State.Capabilities["os"] = "windows"
	snapshots[0].State.RecentLog[0] = "changed"

	stored, ok := registry.snapshot("agent-1")
	if !ok {
		t.Fatal("agent disappeared")
	}
	if stored.State.Capabilities["os"] != "linux" || stored.State.RecentLog[0] != "registered" {
		t.Fatalf("snapshot leaked mutable registry state: %+v", stored.State)
	}
}

func TestAgentRegistryMarkSeen(t *testing.T) {
	registry := newAgentRegistry()
	registry.agents["agent-1"] = agentState{}
	want := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	registry.markSeen("agent-1", want)

	got, ok := registry.snapshot("agent-1")
	if !ok || !got.State.LastSeenUTC.Equal(want) {
		t.Fatalf("last seen = %v, found=%t", got.State.LastSeenUTC, ok)
	}
}
