package server

import (
	"hash/fnv"
	"strings"
	"time"
)

const (
	agentUpdateInitialWarmupBase   = 12 * time.Second
	agentUpdateInitialWarmupJitter = 8 * time.Second
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

func agentUpdateInitialWarmup(agentID, target string) time.Duration {
	base := agentUpdateInitialWarmupBase
	if base < 0 {
		base = 0
	}
	jitterWindow := agentUpdateInitialWarmupJitter
	if jitterWindow <= 0 {
		return base
	}
	key := strings.TrimSpace(agentID) + "|" + strings.TrimSpace(target)
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))
	jitterSteps := uint32(jitterWindow/time.Millisecond) + 1
	jitter := time.Duration(hasher.Sum32()%jitterSteps) * time.Millisecond
	return base + jitter
}
