package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
	"github.com/izzyreal/ciwi/internal/domain"
	"github.com/izzyreal/ciwi/internal/protocol"
)

type agentRepositoryAdapter struct {
	state *stateStore
}

func (a agentRepositoryAdapter) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshots := a.state.agentRegistry.snapshots()
	serverVersion := currentVersion()
	result := make([]domain.Agent, 0, len(snapshots))
	for _, snapshot := range snapshots {
		jobInProgress, err := a.state.agentJobExecutionStore().AgentHasActiveJobExecution(snapshot.ID)
		if err != nil {
			jobInProgress = false
		}
		view := agentViewFromState(snapshot.ID, snapshot.State, snapshot.PendingUpdate, serverVersion, jobInProgress)
		result = append(result, domain.Agent{
			ID: view.AgentID, Hostname: view.Hostname, OS: view.OS, Arch: view.Arch, Version: view.Version,
			Authorized: view.Authorized, Deactivated: view.Deactivated, JobInProgress: view.JobInProgress,
			Capabilities: cloneMap(view.Capabilities), LastSeenUTC: view.LastSeenUTC, RecentLog: append([]string(nil), view.RecentLog...),
			NeedsUpdate: view.NeedsUpdate, UpdateTarget: view.UpdateTarget, UpdateRequested: view.UpdateRequested,
			UpdateAttempts: view.UpdateAttempts, UpdateInProgress: view.UpdateInProgress,
			UpdateNextRetry: optionalTimeValue(view.UpdateNextRetryUTC), UpdateLastError: view.UpdateLastError,
		})
	}
	return result, nil
}

func optionalTimeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

type agentMutatorAdapter struct {
	state *stateStore
}

type agentScriptMutatorAdapter struct{ state *stateStore }

func (a agentScriptMutatorAdapter) RunAgentScript(ctx context.Context, request application.RunAgentScriptRequest) (application.RunAgentScriptResult, error) {
	if err := ctx.Err(); err != nil {
		return application.RunAgentScriptResult{}, err
	}
	s := a.state
	s.mu.Lock()
	agent, ok := s.agents[request.AgentID]
	if !ok {
		s.mu.Unlock()
		return application.RunAgentScriptResult{}, agentNotFoundError(request.AgentID)
	}
	if strings.TrimSpace(agent.Capabilities["executor"]) != "script" {
		s.mu.Unlock()
		return application.RunAgentScriptResult{}, application.NewError(application.ErrorInvalidArgument, "agent does not advertise script executor support", nil)
	}
	if !containsString(capabilityShells(agent.Capabilities), request.Shell) {
		s.mu.Unlock()
		return application.RunAgentScriptResult{}, application.NewError(application.ErrorInvalidArgument, "agent does not support requested shell", nil)
	}
	s.mu.Unlock()
	job, err := s.agentJobExecutionStore().CreateJobExecution(protocol.CreateJobExecutionRequest{
		Script:               request.Script,
		RequiredCapabilities: map[string]string{"agent_id": request.AgentID, "executor": "script", "shell": request.Shell},
		TimeoutSeconds:       request.TimeoutSeconds,
		Metadata: domain.ExecutionMetadata{
			domain.ExecutionMetadataAdhoc:        "1",
			domain.ExecutionMetadataAdhocAgentID: request.AgentID,
			domain.ExecutionMetadataAdhocShell:   request.Shell,
		},
	})
	if err != nil {
		return application.RunAgentScriptResult{}, application.WrapInternal("queue agent script", err)
	}
	s.mu.Lock()
	if current, found := s.agents[request.AgentID]; found {
		current.RecentLog = appendAgentLog(current.RecentLog, "ad-hoc script queued ("+request.Shell+") job="+job.ID)
		s.agents[request.AgentID] = current
	}
	s.mu.Unlock()
	return application.RunAgentScriptResult{Queued: true, AgentID: request.AgentID, JobExecutionID: job.ID, Shell: request.Shell, TimeoutSeconds: request.TimeoutSeconds}, nil
}

func (a agentMutatorAdapter) ExecuteAgentAction(ctx context.Context, request application.AgentActionRequest) (application.AgentActionResult, error) {
	if err := ctx.Err(); err != nil {
		return application.AgentActionResult{}, err
	}
	s := a.state
	agentID := request.AgentID
	switch request.Action {
	case application.AgentActionAuthorize, application.AgentActionUnauthorize:
		authorized := request.Action == application.AgentActionAuthorize
		s.mu.Lock()
		agent, ok := s.agents[agentID]
		if ok {
			agent.Authorized = authorized
			agent.RecentLog = appendAgentLog(agent.RecentLog, boolLabelServer(authorized, "manual authorization granted", "manual authorization revoked"))
			s.agents[agentID] = agent
		}
		s.mu.Unlock()
		if !ok {
			return application.AgentActionResult{}, agentNotFoundError(agentID)
		}
		if err := s.persistAgentSnapshot(agentID, agent); err != nil {
			return application.AgentActionResult{}, application.WrapInternal("persist agent authorization", err)
		}
		return requestedAgentAction(agentID, boolLabelServer(authorized, "agent authorized", "agent unauthorized")), nil
	case application.AgentActionActivate, application.AgentActionDeactivate:
		deactivated := request.Action == application.AgentActionDeactivate
		s.mu.Lock()
		agent, ok := s.agents[agentID]
		if ok {
			agent.Deactivated = deactivated
			s.agentDeactivated[agentID] = deactivated
			agent.RecentLog = appendAgentLog(agent.RecentLog, boolLabelServer(deactivated, "manual deactivation requested", "manual activation requested"))
			s.agents[agentID] = agent
		}
		s.mu.Unlock()
		if !ok {
			return application.AgentActionResult{}, agentNotFoundError(agentID)
		}
		stateValue := "0"
		if deactivated {
			stateValue = "1"
		}
		if err := s.updateStateStore().SetAppState(agentDeactivatedStateKey(agentID), stateValue); err != nil {
			return application.AgentActionResult{}, application.WrapInternal("persist agent activation", err)
		}
		cancelled := 0
		if deactivated {
			var err error
			cancelled, err = s.cancelActiveJobsForAgent(agentID)
			if err != nil {
				return application.AgentActionResult{}, application.WrapInternal("cancel active agent jobs", err)
			}
		}
		if cancelled > 0 {
			agent.RecentLog = appendAgentLog(agent.RecentLog, "deactivation cancelled active job count="+strconv.Itoa(cancelled))
			s.mu.Lock()
			s.agents[agentID] = agent
			s.mu.Unlock()
		}
		if err := s.persistAgentSnapshot(agentID, agent); err != nil {
			return application.AgentActionResult{}, application.WrapInternal("persist agent activation", err)
		}
		message := boolLabelServer(deactivated, "agent deactivated", "agent activated")
		if cancelled > 0 {
			message += "; cancelled active jobs=" + strconv.Itoa(cancelled)
		}
		return requestedAgentAction(agentID, message), nil
	case application.AgentActionRefreshTools, application.AgentActionRestart, application.AgentActionWipeCache:
		s.mu.Lock()
		agent, ok := s.agents[agentID]
		if ok {
			switch request.Action {
			case application.AgentActionRefreshTools:
				s.agentToolRefresh[agentID] = true
				agent.RecentLog = appendAgentLog(agent.RecentLog, "manual tools refresh requested")
			case application.AgentActionRestart:
				s.agentRestarts[agentID] = true
				agent.RecentLog = appendAgentLog(agent.RecentLog, "manual restart requested")
			case application.AgentActionWipeCache:
				s.agentCacheWipes[agentID] = true
				agent.RecentLog = appendAgentLog(agent.RecentLog, "manual cache wipe requested")
			}
			s.agents[agentID] = agent
		}
		s.mu.Unlock()
		if !ok {
			return application.AgentActionResult{}, agentNotFoundError(agentID)
		}
		message := map[string]string{
			application.AgentActionRefreshTools: "tools refresh requested",
			application.AgentActionRestart:      "agent restart requested",
			application.AgentActionWipeCache:    "agent cache wipe requested",
		}[request.Action]
		return requestedAgentAction(agentID, message), nil
	case application.AgentActionFlushJobHistory:
		if !s.agentExists(agentID) {
			return application.AgentActionResult{}, agentNotFoundError(agentID)
		}
		deletedIDs, err := s.db.FlushJobExecutionHistoryByAgent(agentID)
		if err != nil {
			return application.AgentActionResult{}, application.WrapInternal("flush agent job history", err)
		}
		removed := 0
		for _, jobID := range deletedIDs {
			if err := os.RemoveAll(filepath.Join(s.artifactsDir, jobID)); err == nil {
				removed++
			}
		}
		s.mu.Lock()
		if agent, ok := s.agents[agentID]; ok {
			agent.RecentLog = appendAgentLog(agent.RecentLog, "manual agent job history flush requested")
			s.agents[agentID] = agent
		}
		s.agentHistoryWipes[agentID] = true
		s.mu.Unlock()
		return requestedAgentAction(agentID, "agent job history flushed: sqlite="+strconv.Itoa(len(deletedIDs))+", disk="+strconv.Itoa(removed)+", local=queued"), nil
	case application.AgentActionDelete:
		s.mu.Lock()
		_, ok := s.agents[agentID]
		if ok {
			delete(s.agents, agentID)
			delete(s.agentUpdates, agentID)
			delete(s.agentToolRefresh, agentID)
			delete(s.agentRestarts, agentID)
			delete(s.agentCacheWipes, agentID)
			delete(s.agentHistoryWipes, agentID)
			delete(s.agentDeactivated, agentID)
			delete(s.agentRollout.Slots, agentID)
		}
		s.mu.Unlock()
		if !ok {
			return application.AgentActionResult{}, agentNotFoundError(agentID)
		}
		if err := s.updateStateStore().DeleteAppState(agentSnapshotStateKey(agentID)); err != nil {
			return application.AgentActionResult{}, application.WrapInternal("delete agent snapshot", err)
		}
		if err := s.updateStateStore().DeleteAppState(agentDeactivatedStateKey(agentID)); err != nil {
			return application.AgentActionResult{}, application.WrapInternal("delete agent activation", err)
		}
		return requestedAgentAction(agentID, "agent snapshot deleted"), nil
	case application.AgentActionUpdate:
		return s.requestAgentUpdate(agentID)
	default:
		return application.AgentActionResult{}, application.NewError(application.ErrorInvalidArgument, "unsupported agent action", nil)
	}
}

func (s *stateStore) requestAgentUpdate(agentID string) (application.AgentActionResult, error) {
	target := resolveManualAgentUpdateTarget(currentVersion(), s.getAgentUpdateTarget())
	if target == "" || target == "dev" {
		return application.AgentActionResult{}, application.NewError(application.ErrorFailedPrecondition, "server version is not a release version", nil)
	}
	s.mu.Lock()
	agent, ok := s.agents[agentID]
	if !ok {
		s.mu.Unlock()
		return application.AgentActionResult{}, agentNotFoundError(agentID)
	}
	if !isVersionDifferent(target, strings.TrimSpace(agent.Version)) {
		agent.RecentLog = appendAgentLog(agent.RecentLog, "manual update requested but agent is already at target version")
		s.agents[agentID] = agent
		s.mu.Unlock()
		return application.AgentActionResult{Requested: false, AgentID: agentID, Message: "agent is already at target version"}, nil
	}
	s.agentUpdates[agentID] = target
	agent.UpdateTarget = target
	agent.UpdateSource = updateSourceManual
	agent.UpdateAttempts = 0
	agent.UpdateInProgress = false
	agent.UpdateLastRequestUTC = time.Time{}
	agent.UpdateNextRetryUTC = time.Time{}
	agent.UpdateLastError = ""
	agent.UpdateLastErrorUTC = time.Time{}
	agent.RecentLog = appendAgentLog(agent.RecentLog, "manual update requested to "+target)
	s.agents[agentID] = agent
	s.mu.Unlock()
	return application.AgentActionResult{Requested: true, AgentID: agentID, Target: target}, nil
}

func (s *stateStore) agentExists(agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.agents[agentID]
	return ok
}

func requestedAgentAction(agentID, message string) application.AgentActionResult {
	return application.AgentActionResult{Requested: true, AgentID: agentID, Message: message}
}

func agentNotFoundError(agentID string) error {
	return application.NewError(application.ErrorNotFound, fmt.Sprintf("agent %q not found", agentID), nil)
}

func boolLabelServer(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}
