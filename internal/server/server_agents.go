package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/server/jobexecution"
)

const maxReportedUpdateFailureLength = 240
const (
	updateSourceAutomatic = "automatic"
	updateSourceManual    = "manual"
)

func (s *stateStore) scheduleAutomaticUpdateFirstAttemptLocked(agentID, target string, now time.Time) (time.Time, int) {
	agentID = strings.TrimSpace(agentID)
	target = strings.TrimSpace(target)
	if agentID == "" || target == "" {
		return time.Time{}, -1
	}
	if s.agentRollout.Slots == nil {
		s.agentRollout.Slots = make(map[string]int)
	}
	if s.agentRollout.Target != target {
		s.agentRollout.Target = target
		s.agentRollout.StartedUTC = now
		s.agentRollout.NextSlot = 0
		s.agentRollout.Slots = make(map[string]int)
	}
	slot, ok := s.agentRollout.Slots[agentID]
	if !ok {
		slot = s.agentRollout.NextSlot
		s.agentRollout.Slots[agentID] = slot
		s.agentRollout.NextSlot++
	}
	firstAttemptAt := s.agentRollout.StartedUTC.Add(agentUpdateFirstAttemptDelay(slot))
	return firstAttemptAt, slot
}

func (s *stateStore) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var hb protocol.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if hb.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	if hb.TimestampUTC.IsZero() {
		hb.TimestampUTC = time.Now().UTC()
	}

	now := time.Now().UTC()
	hasActiveJob := false
	if active, err := s.agentJobExecutionStore().AgentHasActiveJobExecution(hb.AgentID); err == nil {
		hasActiveJob = active
	}
	s.mu.Lock()
	prev := s.agents[hb.AgentID]
	refreshTools := s.agentToolRefresh[hb.AgentID]
	if refreshTools {
		delete(s.agentToolRefresh, hb.AgentID)
	}
	restartRequested := s.agentRestarts[hb.AgentID]
	if restartRequested {
		delete(s.agentRestarts, hb.AgentID)
	}
	target := resolveEffectiveAgentUpdateTarget(s.getAgentUpdateTarget(), currentVersion())
	manualTarget := strings.TrimSpace(s.agentUpdates[hb.AgentID])
	updateSource := updateSourceAutomatic
	if manualTarget != "" {
		target = manualTarget
		updateSource = updateSourceManual
	}

	updateRequested := false
	updateAttemptFailed := false
	updateAttemptFailureReason := ""
	firstAttemptAt := time.Time{}
	firstAttemptSlot := -1
	reportedUpdateFailure := summarizeUpdateFailure(hb.UpdateFailure)
	reportedRestartStatus := summarizeRestartStatus(hb.RestartStatus)
	needsUpdate := target != "" && isVersionDifferent(target, strings.TrimSpace(hb.Version))
	if needsUpdate {
		prevTarget := strings.TrimSpace(prev.UpdateTarget)
		prevSource := strings.TrimSpace(prev.UpdateSource)
		if prevSource == "" {
			prevSource = updateSourceAutomatic
			if manualTarget != "" && prevTarget == target {
				prevSource = updateSourceManual
			}
		}
		overrideScheduled := false
		if prevTarget != target {
			overrideScheduled = true
		} else if prevSource != updateSource {
			// Keep rollback/manual intent stable; allow manual requests to replace
			// automatic schedules, but not vice versa.
			overrideScheduled = updateSource == updateSourceManual
		}
		if overrideScheduled {
			prev.UpdateTarget = target
			prev.UpdateSource = updateSource
			prev.UpdateAttempts = 0
			prev.UpdateInProgress = false
			prev.UpdateLastRequestUTC = time.Time{}
			prev.UpdateNextRetryUTC = time.Time{}
			prev.UpdateLastError = ""
			prev.UpdateLastErrorUTC = time.Time{}
			if updateSource == updateSourceAutomatic {
				firstAttemptAt, firstAttemptSlot = s.scheduleAutomaticUpdateFirstAttemptLocked(hb.AgentID, target, now)
				if !firstAttemptAt.IsZero() {
					prev.UpdateNextRetryUTC = firstAttemptAt
				}
			}
		} else if prev.UpdateSource == "" {
			prev.UpdateSource = updateSource
		}
		if reportedUpdateFailure != "" {
			prev.UpdateLastError = reportedUpdateFailure
			prev.UpdateLastErrorUTC = now
		}

		if prev.UpdateInProgress {
			if reportedUpdateFailure != "" {
				prev.UpdateInProgress = false
				prev.UpdateNextRetryUTC = now.Add(agentUpdateBackoff(prev.UpdateAttempts))
				updateAttemptFailed = true
				updateAttemptFailureReason = reportedUpdateFailure
			} else if prev.UpdateLastRequestUTC.IsZero() || !now.Before(prev.UpdateLastRequestUTC.Add(agentUpdateInProgressGrace)) {
				prev.UpdateInProgress = false
				// If the agent is still busy with a job, treat stale in-progress as deferred
				// instead of failed so we don't enter unnecessary backoff loops.
				if hasActiveJob {
					prev.UpdateNextRetryUTC = time.Time{}
				} else {
					prev.UpdateNextRetryUTC = now.Add(agentUpdateBackoff(prev.UpdateAttempts))
					updateAttemptFailed = true
				}
			}
		}

		if !hasActiveJob && !prev.UpdateInProgress && (prev.UpdateNextRetryUTC.IsZero() || !now.Before(prev.UpdateNextRetryUTC)) {
			updateRequested = true
			prev.UpdateAttempts++
			prev.UpdateInProgress = true
			prev.UpdateLastRequestUTC = now
			prev.UpdateNextRetryUTC = time.Time{}
		}
	} else {
		if manualTarget != "" {
			delete(s.agentUpdates, hb.AgentID)
		}
		prev.UpdateTarget = ""
		prev.UpdateSource = ""
		prev.UpdateAttempts = 0
		prev.UpdateInProgress = false
		prev.UpdateLastRequestUTC = time.Time{}
		prev.UpdateNextRetryUTC = time.Time{}
		prev.UpdateLastError = ""
		prev.UpdateLastErrorUTC = time.Time{}
	}

	state := agentState{
		Hostname:             hb.Hostname,
		OS:                   hb.OS,
		Arch:                 hb.Arch,
		Version:              hb.Version,
		Capabilities:         hb.Capabilities,
		LastSeenUTC:          hb.TimestampUTC,
		RecentLog:            append([]string(nil), prev.RecentLog...),
		UpdateTarget:         prev.UpdateTarget,
		UpdateSource:         prev.UpdateSource,
		UpdateAttempts:       prev.UpdateAttempts,
		UpdateInProgress:     prev.UpdateInProgress,
		UpdateLastRequestUTC: prev.UpdateLastRequestUTC,
		UpdateNextRetryUTC:   prev.UpdateNextRetryUTC,
		UpdateLastError:      prev.UpdateLastError,
		UpdateLastErrorUTC:   prev.UpdateLastErrorUTC,
	}
	state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("heartbeat version=%s platform=%s/%s", strings.TrimSpace(hb.Version), strings.TrimSpace(hb.OS), strings.TrimSpace(hb.Arch)))
	if refreshTools {
		state.RecentLog = appendAgentLog(state.RecentLog, "server requested tools refresh")
	}
	if restartRequested {
		state.RecentLog = appendAgentLog(state.RecentLog, "server requested restart")
	}
	if !firstAttemptAt.IsZero() {
		delay := firstAttemptAt.Sub(now)
		if delay < 0 {
			delay = 0
		}
		state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("scheduled automatic update to %s at %s (in=%s, slot=%d)", target, firstAttemptAt.Local().Format("15:04:05"), delay.Round(time.Second), firstAttemptSlot))
	}
	if updateAttemptFailed {
		if updateAttemptFailureReason != "" {
			state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("update attempt to %s failed: %s; retry at %s (attempt=%d)", target, updateAttemptFailureReason, state.UpdateNextRetryUTC.Local().Format("15:04:05"), state.UpdateAttempts))
		} else {
			state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("update attempt to %s did not complete; retry at %s (attempt=%d)", target, state.UpdateNextRetryUTC.Local().Format("15:04:05"), state.UpdateAttempts))
		}
	} else if reportedUpdateFailure != "" {
		state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("agent reported update failure: %s", reportedUpdateFailure))
	}
	if reportedRestartStatus != "" {
		state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("agent restart status: %s", reportedRestartStatus))
	}
	if updateRequested {
		state.RecentLog = appendAgentLog(state.RecentLog, fmt.Sprintf("server requested update to %s (attempt=%d)", target, state.UpdateAttempts))
	}
	s.agents[hb.AgentID] = state
	s.mu.Unlock()

	resp := protocol.HeartbeatResponse{
		Accepted: true,
	}
	if refreshTools {
		resp.RefreshToolsRequested = true
	}
	if restartRequested {
		resp.RestartRequested = true
	}
	if updateRequested {
		resp.UpdateRequested = true
		resp.UpdateTarget = target
		resp.UpdateRepository = strings.TrimSpace(envOrDefault("CIWI_UPDATE_REPO", "izzyreal/ciwi"))
		resp.UpdateAPIBase = strings.TrimRight(strings.TrimSpace(envOrDefault("CIWI_UPDATE_API_BASE", "https://api.github.com")), "/")
		resp.Message = "server requested agent update"
	}
	writeJSON(w, http.StatusOK, resp)
}

func summarizeUpdateFailure(raw string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if normalized == "" {
		return ""
	}
	if len(normalized) <= maxReportedUpdateFailureLength {
		return normalized
	}
	if maxReportedUpdateFailureLength <= 3 {
		return normalized[:maxReportedUpdateFailureLength]
	}
	return strings.TrimSpace(normalized[:maxReportedUpdateFailureLength-3]) + "..."
}

func summarizeRestartStatus(raw string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if normalized == "" {
		return ""
	}
	if len(normalized) <= maxReportedUpdateFailureLength {
		return normalized
	}
	if maxReportedUpdateFailureLength <= 3 {
		return normalized[:maxReportedUpdateFailureLength]
	}
	return strings.TrimSpace(normalized[:maxReportedUpdateFailureLength-3]) + "..."
}

func (s *stateStore) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	type snapshot struct {
		id      string
		state   agentState
		pending string
	}
	snapshots := make([]snapshot, 0, len(s.agents))
	serverVersion := currentVersion()
	for id, a := range s.agents {
		snapshots = append(snapshots, snapshot{
			id:      id,
			state:   a,
			pending: strings.TrimSpace(s.agentUpdates[id]),
		})
	}
	s.mu.Unlock()
	agents := make([]agentView, 0, len(snapshots))
	for _, snap := range snapshots {
		jobInProgress, err := s.agentJobExecutionStore().AgentHasActiveJobExecution(snap.id)
		if err != nil {
			jobInProgress = false
		}
		agents = append(agents, agentViewFromState(snap.id, snap.state, snap.pending, serverVersion, jobInProgress))
	}
	writeJSON(w, http.StatusOK, agentsViewResponse{Agents: agents})
}

func (s *stateStore) leaseJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req protocol.LeaseJobExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	agentCaps := req.Capabilities
	s.mu.Lock()
	if a, ok := s.agents[req.AgentID]; ok {
		agentCaps = mergeCapabilities(a, req.Capabilities)
	}
	s.mu.Unlock()
	if agentCaps == nil {
		agentCaps = map[string]string{}
	}
	agentCaps["agent_id"] = req.AgentID
	hasActive, err := s.agentJobExecutionStore().AgentHasActiveJobExecution(req.AgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hasActive {
		writeJSON(w, http.StatusOK, jobexecution.LeaseViewResponse{
			Assigned: false,
			Message:  "agent already has an active job",
		})
		return
	}

	job, err := s.agentJobExecutionStore().LeaseJobExecution(req.AgentID, agentCaps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		writeJSON(w, http.StatusOK, jobexecution.LeaseViewResponse{Assigned: false, Message: "no matching queued job"})
		return
	}
	slog.Info("job leased to agent", "job_execution_id", job.ID, "agent_id", req.AgentID)
	if err := s.resolveJobSecrets(r.Context(), job); err != nil {
		failMsg := fmt.Sprintf("secret resolution failed before execution: %v", err)
		_, _ = s.agentJobExecutionStore().UpdateJobExecutionStatus(job.ID, protocol.JobExecutionStatusUpdateRequest{
			AgentID: req.AgentID,
			Status:  protocol.JobExecutionStatusFailed,
			Error:   failMsg,
		})
		writeJSON(w, http.StatusOK, jobexecution.LeaseViewResponse{Assigned: false, Message: failMsg})
		return
	}
	jobResponse := jobexecution.ViewFromProtocol(*job)
	writeJSON(w, http.StatusOK, jobexecution.LeaseViewResponse{Assigned: true, JobExecution: &jobResponse})
}
