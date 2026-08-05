package application

import (
	"context"
	"testing"

	"github.com/izzyreal/ciwi/internal/domain"
)

type agentRepositoryStub struct {
	agents []domain.Agent
}

func (s agentRepositoryStub) ListAgents(context.Context) ([]domain.Agent, error) {
	return s.agents, nil
}

type agentMutatorStub struct {
	request AgentActionRequest
}

type agentScriptMutatorStub struct {
	request RunAgentScriptRequest
}

func (s *agentScriptMutatorStub) RunAgentScript(_ context.Context, request RunAgentScriptRequest) (RunAgentScriptResult, error) {
	s.request = request
	return RunAgentScriptResult{Queued: true, AgentID: request.AgentID, JobExecutionID: "job-script", Shell: request.Shell, TimeoutSeconds: request.TimeoutSeconds}, nil
}

func (s *agentMutatorStub) ExecuteAgentAction(_ context.Context, request AgentActionRequest) (AgentActionResult, error) {
	s.request = request
	return AgentActionResult{Requested: true, AgentID: request.AgentID, Message: request.Action + " requested"}, nil
}

func TestAgentQueriesDelegate(t *testing.T) {
	queries := NewAgentQueries(agentRepositoryStub{agents: []domain.Agent{{ID: "agent-1"}}})
	agents, err := queries.ListAgents(t.Context())
	if err != nil || len(agents) != 1 || agents[0].ID != "agent-1" {
		t.Fatalf("agents = %+v, err = %v", agents, err)
	}
}

func TestAgentCommandsValidateExecuteAndPublish(t *testing.T) {
	mutator := &agentMutatorStub{}
	changes := NewChangeHub()
	commands := NewAgentCommands(mutator, nil, changes)
	watchCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := changes.Watch(watchCtx)
	<-events // initial resync
	result, err := commands.Execute(t.Context(), AgentActionRequest{AgentID: " agent-1 ", Action: "RESTART"})
	if err != nil || !result.Requested || mutator.request.AgentID != "agent-1" || mutator.request.Action != AgentActionRestart {
		t.Fatalf("result = %+v, request = %+v, err = %v", result, mutator.request, err)
	}
	change := <-events
	if len(change.Topics) != 2 || change.Topics[0] != ChangeAgents || change.Topics[1] != ChangeAgentEligibility {
		t.Fatalf("change = %+v", change)
	}
	if _, err := commands.Execute(t.Context(), AgentActionRequest{AgentID: "agent-1", Action: "explode"}); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("unsupported action error = %v", err)
	}
}

func TestAgentScriptCommandsValidateNormalizeAndPublish(t *testing.T) {
	mutator := &agentScriptMutatorStub{}
	changes := NewChangeHub()
	commands := NewAgentScriptCommands(mutator, nil, changes)
	watchCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := changes.Watch(watchCtx)
	<-events
	result, err := commands.Run(t.Context(), RunAgentScriptRequest{
		AgentID: " agent-1 ", Shell: " POSIX ", Script: " echo hello ",
	})
	if err != nil || !result.Queued || result.JobExecutionID != "job-script" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if mutator.request.AgentID != "agent-1" || mutator.request.Shell != "posix" || mutator.request.Script != "echo hello" || mutator.request.TimeoutSeconds != 600 {
		t.Fatalf("normalized request = %+v", mutator.request)
	}
	change := <-events
	if len(change.Topics) != 3 || change.Topics[0] != ChangeQueue || change.Topics[1] != ChangeAgents || change.Topics[2] != ChangeAgentEligibility {
		t.Fatalf("change = %+v", change)
	}
	if _, err := commands.Run(t.Context(), RunAgentScriptRequest{AgentID: "agent-1", Shell: "posix"}); ErrorKindOf(err) != ErrorInvalidArgument {
		t.Fatalf("missing script error = %v", err)
	}
}
