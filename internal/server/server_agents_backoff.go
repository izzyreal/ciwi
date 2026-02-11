package server

import "time"

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
