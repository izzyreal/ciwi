package server

import (
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation"
)

type agentView struct {
	AgentID              string                          `json:"agent_id"`
	Hostname             string                          `json:"hostname"`
	OS                   string                          `json:"os"`
	Arch                 string                          `json:"arch"`
	Authorized           bool                            `json:"authorized"`
	Deactivated          bool                            `json:"deactivated,omitempty"`
	JobInProgress        bool                            `json:"job_in_progress,omitempty"`
	Version              string                          `json:"version,omitempty"`
	Capabilities         map[string]string               `json:"capabilities"`
	CanRunScript         bool                            `json:"can_run_script"`
	ScriptShells         []presentation.AgentScriptShell `json:"script_shells"`
	LastSeenUTC          time.Time                       `json:"last_seen_utc"`
	RecentLog            []string                        `json:"recent_log,omitempty"`
	NeedsUpdate          bool                            `json:"needs_update,omitempty"`
	UpdateTarget         string                          `json:"update_target,omitempty"`
	UpdateRequested      bool                            `json:"update_requested,omitempty"`
	UpdateAttempts       int                             `json:"update_attempts,omitempty"`
	UpdateInProgress     bool                            `json:"update_in_progress,omitempty"`
	UpdateLastRequestUTC *time.Time                      `json:"update_last_request_utc,omitempty"`
	UpdateNextRetryUTC   *time.Time                      `json:"update_next_retry_utc,omitempty"`
	UpdateLastError      string                          `json:"update_last_error,omitempty"`
	UpdateLastErrorUTC   *time.Time                      `json:"update_last_error_utc,omitempty"`
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
	Queued         bool                         `json:"queued"`
	AgentID        string                       `json:"agent_id"`
	JobExecutionID string                       `json:"job_execution_id"`
	Shell          string                       `json:"shell"`
	TimeoutSeconds int                          `json:"timeout_seconds"`
	Notice         presentation.TransientNotice `json:"notice"`
}

func agentViewFromState(agentID string, state agentState, pendingTarget, serverVersion string, jobInProgress bool) agentView {
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

	scriptShells := presentation.AgentScriptShells(state.Capabilities)
	return agentView{
		AgentID:              agentID,
		Hostname:             state.Hostname,
		OS:                   state.OS,
		Arch:                 state.Arch,
		Authorized:           state.Authorized,
		Deactivated:          state.Deactivated,
		JobInProgress:        jobInProgress,
		Version:              state.Version,
		Capabilities:         cloneMap(state.Capabilities),
		CanRunScript:         len(scriptShells) > 0,
		ScriptShells:         scriptShells,
		LastSeenUTC:          state.LastSeenUTC,
		RecentLog:            append([]string(nil), state.RecentLog...),
		NeedsUpdate:          needsUpdate,
		UpdateTarget:         updateTarget,
		UpdateRequested:      updateRequested,
		UpdateAttempts:       state.UpdateAttempts,
		UpdateInProgress:     state.UpdateInProgress,
		UpdateLastRequestUTC: optionalTime(state.UpdateLastRequestUTC),
		UpdateNextRetryUTC:   optionalTime(state.UpdateNextRetryUTC),
		UpdateLastError:      state.UpdateLastError,
		UpdateLastErrorUTC:   optionalTime(state.UpdateLastErrorUTC),
	}
}

func optionalTime(ts time.Time) *time.Time {
	if ts.IsZero() {
		return nil
	}
	out := ts
	return &out
}
