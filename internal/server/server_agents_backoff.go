package server

import (
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
)

const (
	agentUpdateFirstAttemptBaseDelay = 10 * time.Second
	agentUpdateFirstAttemptStepDelay = 2 * time.Second
	agentUpdateRestartAllowance      = 70 * time.Second
	agentUpdateInProgressGrace       = (2 * protocol.AgentHeartbeatInterval) + agentUpdateRestartAllowance
)

func agentUpdateBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return 30 * time.Second
	}
	d := 30 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= 15*time.Minute {
			return 15 * time.Minute
		}
	}
	if d < 30*time.Second {
		return 30 * time.Second
	}
	if d > 15*time.Minute {
		return 15 * time.Minute
	}
	return d
}

func agentUpdateFirstAttemptDelay(slot int) time.Duration {
	if slot < 0 {
		slot = 0
	}
	return agentUpdateFirstAttemptBaseDelay + (time.Duration(slot) * agentUpdateFirstAttemptStepDelay)
}
