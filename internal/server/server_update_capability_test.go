package server

import "testing"

func TestDetectServerUpdateCapability(t *testing.T) {
	t.Setenv("CIWI_UPDATE_TEST_VERSION", "")

	old := currentVersion
	_ = old
	cap := detectServerUpdateCapability()
	if cap.Mode == "" {
		t.Fatalf("expected capability mode to be populated")
	}
	if !cap.Supported && cap.Reason == "" {
		t.Fatalf("expected unsupported capability to include reason")
	}
}

func TestListAgentsBlockedOnNonServiceSelfUpdate(t *testing.T) {
	s := &stateStore{agents: map[string]agentState{
		"a": {UpdateLastError: agentNonServiceUpdateErrorMarker + ": install via service"},
		"b": {UpdateLastError: "other failure"},
		"c": {UpdateLastError: "AGENT IS NOT RUNNING AS A SERVICE; SELF-UPDATE DISABLED"},
	}}
	got := s.listAgentsBlockedOnNonServiceSelfUpdate()
	if len(got) != 2 {
		t.Fatalf("expected 2 blocked agents, got %d (%v)", len(got), got)
	}
}
