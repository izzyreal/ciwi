package protocol

import "time"

type HeartbeatRequest struct {
	AgentID      string            `json:"agent_id"`
	Hostname     string            `json:"hostname"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Capabilities map[string]string `json:"capabilities"`
	TimestampUTC time.Time         `json:"timestamp_utc"`
}

type HeartbeatResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
}
