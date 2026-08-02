package server

import "testing"

func TestAgentEligibilityChangedIgnoresHeartbeatOnlyState(t *testing.T) {
	previous := agentState{OS: "darwin", Arch: "arm64", Authorized: true, Capabilities: map[string]string{"shells": "posix"}}
	current := previous
	current.Hostname = "renamed"
	current.Version = "v0.2.1"
	if agentEligibilityChanged(previous, current) {
		t.Fatal("non-scheduling agent metadata changed eligibility")
	}
	current.Capabilities = map[string]string{"shells": "posix", "tool.go": "1.25"}
	if !agentEligibilityChanged(previous, current) {
		t.Fatal("capability change did not invalidate eligibility")
	}
}
