package server

import (
	"strings"
	"time"
)

type agentView struct {
	AgentID              string            `json:"agent_id"`
	Hostname             string            `json:"hostname"`
	OS                   string            `json:"os"`
	Arch                 string            `json:"arch"`
	Version              string            `json:"version,omitempty"`
	Capabilities         map[string]string `json:"capabilities"`
	LastSeenUTC          time.Time         `json:"last_seen_utc"`
	RecentLog            []string          `json:"recent_log,omitempty"`
	NeedsUpdate          bool              `json:"needs_update,omitempty"`
	UpdateTarget         string            `json:"update_target,omitempty"`
	UpdateRequested      bool              `json:"update_requested,omitempty"`
	UpdateAttempts       int               `json:"update_attempts,omitempty"`
	UpdateLastRequestUTC *time.Time        `json:"update_last_request_utc,omitempty"`
	UpdateNextRetryUTC   *time.Time        `json:"update_next_retry_utc,omitempty"`
}

type agentViewResponse struct {
	Agent agentView `json:"agent"`
}

type agentsViewResponse struct {
	Agents []agentView `json:"agents"`
}

type agentActionResponse struct {
	Requested bool   `json:"requested"`
	AgentID   string `json:"agent_id,omitempty"`
	Message   string `json:"message,omitempty"`
	Target    string `json:"target,omitempty"`
}

type agentRunScriptResponse struct {
	Queued         bool   `json:"queued"`
	AgentID        string `json:"agent_id"`
	JobExecutionID string `json:"job_execution_id"`
	Shell          string `json:"shell"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func agentViewFromState(agentID string, state agentState, pendingTarget, serverVersion string) agentView {
	version := strings.TrimSpace(state.Version)
	trimmedPendingTarget := strings.TrimSpace(pendingTarget)
	trimmedStateTarget := strings.TrimSpace(state.UpdateTarget)
	needsUpdate := serverVersion != "" && isVersionNewer(serverVersion, version)
	updateTarget := serverVersion
	if trimmedPendingTarget != "" {
		updateTarget = trimmedPendingTarget
	} else if trimmedStateTarget != "" {
		updateTarget = trimmedStateTarget
	}
	updateRequested := trimmedPendingTarget != "" || (trimmedStateTarget != "" && isVersionDifferent(trimmedStateTarget, version))

	return agentView{
		AgentID:              agentID,
		Hostname:             state.Hostname,
		OS:                   state.OS,
		Arch:                 state.Arch,
		Version:              state.Version,
		Capabilities:         cloneMap(state.Capabilities),
		LastSeenUTC:          state.LastSeenUTC,
		RecentLog:            append([]string(nil), state.RecentLog...),
		NeedsUpdate:          needsUpdate,
		UpdateTarget:         updateTarget,
		UpdateRequested:      updateRequested,
		UpdateAttempts:       state.UpdateAttempts,
		UpdateLastRequestUTC: optionalTime(state.UpdateLastRequestUTC),
		UpdateNextRetryUTC:   optionalTime(state.UpdateNextRetryUTC),
	}
}

func optionalTime(ts time.Time) *time.Time {
	if ts.IsZero() {
		return nil
	}
	out := ts
	return &out
}
