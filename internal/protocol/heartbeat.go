package protocol

import "time"

type HeartbeatRequest struct {
	AgentID      string            `json:"agent_id"`
	Hostname     string            `json:"hostname"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Version      string            `json:"version,omitempty"`
	Capabilities map[string]string `json:"capabilities"`
	TimestampUTC time.Time         `json:"timestamp_utc"`
}

type HeartbeatResponse struct {
	Accepted         bool   `json:"accepted"`
	Message          string `json:"message,omitempty"`
	UpdateRequested  bool   `json:"update_requested,omitempty"`
	UpdateTarget     string `json:"update_target,omitempty"`
	UpdateRepository string `json:"update_repository,omitempty"`
	UpdateAPIBase    string `json:"update_api_base,omitempty"`
}
