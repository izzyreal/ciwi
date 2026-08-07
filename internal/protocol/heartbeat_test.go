package protocol

import (
	"testing"
	"time"
)

func TestAgentHeartbeatFadeDurationIsTwentyPercentFaster(t *testing.T) {
	if AgentHeartbeatInterval != 10*time.Second {
		t.Fatalf("heartbeat interval = %v, want 10s", AgentHeartbeatInterval)
	}
	if AgentHeartbeatFadeDuration != 6400*time.Millisecond {
		t.Fatalf("heartbeat fade duration = %v, want 6.4s", AgentHeartbeatFadeDuration)
	}
}
