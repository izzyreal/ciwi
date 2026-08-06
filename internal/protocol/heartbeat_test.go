package protocol

import (
	"testing"
	"time"
)

func TestAgentHeartbeatFadeDurationIsTwentyPercentFasterThanHeartbeat(t *testing.T) {
	if AgentHeartbeatInterval != 10*time.Second {
		t.Fatalf("heartbeat interval = %v, want 10s", AgentHeartbeatInterval)
	}
	if AgentHeartbeatFadeDuration != 8*time.Second {
		t.Fatalf("heartbeat fade duration = %v, want 8s", AgentHeartbeatFadeDuration)
	}
}
