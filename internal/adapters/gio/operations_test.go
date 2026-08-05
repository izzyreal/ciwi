//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"errors"
	"testing"

	"github.com/izzyreal/ciwi/internal/presentation/operations"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

type recordingNativeActionClient struct {
	called         string
	idempotencyKey string
	err            error
}

func (c *recordingNativeActionClient) record(command, key string) error {
	c.called, c.idempotencyKey = command, key
	return c.err
}

func (c *recordingNativeActionClient) RunPipeline(_ context.Context, _ *cnpv1.RunPipelineRequest, key string) (*cnpv1.RunPipelineResult, error) {
	return &cnpv1.RunPipelineResult{PipelineId: "build", Enqueued: 2}, c.record("run-pipeline", key)
}
func (c *recordingNativeActionClient) RunPipelineChain(_ context.Context, _ *cnpv1.RunPipelineChainRequest, key string) (*cnpv1.RunPipelineChainResult, error) {
	return &cnpv1.RunPipelineChainResult{ChainId: "release", ChainName: "Build and release", Enqueued: 3}, c.record("run-chain", key)
}
func (c *recordingNativeActionClient) ClearExecutionQueue(_ context.Context, key string) (*cnpv1.ClearExecutionQueueResult, error) {
	return &cnpv1.ClearExecutionQueueResult{Cleared: 4}, c.record("clear-queue", key)
}
func (c *recordingNativeActionClient) RemoveQueuedExecution(_ context.Context, id, key string) (*cnpv1.RemoveQueuedExecutionResult, error) {
	return &cnpv1.RemoveQueuedExecutionResult{JobExecutionId: id}, c.record("remove-execution", key)
}
func (c *recordingNativeActionClient) FlushExecutionHistory(_ context.Context, _ *cnpv1.FlushExecutionHistoryRequest, key string) (*cnpv1.FlushExecutionHistoryResult, error) {
	return &cnpv1.FlushExecutionHistoryResult{Flushed: 5}, c.record("flush-history", key)
}
func (c *recordingNativeActionClient) CancelExecution(_ context.Context, id, key string) (*cnpv1.CancelExecutionResult, error) {
	return &cnpv1.CancelExecutionResult{JobExecutionId: id}, c.record("cancel-execution", key)
}
func (c *recordingNativeActionClient) RerunExecution(_ context.Context, id, key string) (*cnpv1.RerunExecutionResult, error) {
	return &cnpv1.RerunExecutionResult{JobExecutionId: id + "-rerun"}, c.record("rerun-execution", key)
}
func (c *recordingNativeActionClient) AgentAction(_ context.Context, _ *cnpv1.AgentActionRequest, key string) (*cnpv1.AgentActionResult, error) {
	return &cnpv1.AgentActionResult{Message: "Agent restarted"}, c.record("agent-action", key)
}
func (c *recordingNativeActionClient) ProjectAction(_ context.Context, _ int64, _ string, key string) (*cnpv1.ProjectActionResult, error) {
	return &cnpv1.ProjectActionResult{Message: "Project reloaded"}, c.record("project-action", key)
}
func (c *recordingNativeActionClient) ImportProject(_ context.Context, _ *cnpv1.ImportProjectRequest, key string) (*cnpv1.ImportProjectResult, error) {
	return &cnpv1.ImportProjectResult{ProjectName: "ciwi"}, c.record("import-project", key)
}
func (c *recordingNativeActionClient) CheckServerUpdates(context.Context) (*cnpv1.ServerUpdateCheckResult, error) {
	return &cnpv1.ServerUpdateCheckResult{}, c.record("check-server-updates", "")
}
func (c *recordingNativeActionClient) ListServerUpdateVersions(context.Context) (*cnpv1.ServerUpdateVersions, error) {
	return &cnpv1.ServerUpdateVersions{}, c.record("refresh-rollback-versions", "")
}
func (c *recordingNativeActionClient) ServerUpdateActionWithKey(_ context.Context, _, _ string, key string) (*cnpv1.ServerUpdateActionResult, error) {
	return &cnpv1.ServerUpdateActionResult{Message: "Update accepted"}, c.record("server-update-action", key)
}

func TestNativeMutationFailureDistinguishesKnownAndAmbiguousOutcomes(t *testing.T) {
	operation := operations.Operation{Class: operations.ClassMutation}
	known := nativeOperationFailure(operation, &cnpclient.Error{
		Code: cnpv1.StatusCode_STATUS_CODE_INVALID_ARGUMENT, Message: "invalid request",
	})
	if known.State != operations.StateFailed {
		t.Fatalf("known server failure state = %s", known.State)
	}
	unknown := nativeOperationFailure(operation, errors.New("connection reset after request write"))
	if unknown.State != operations.StateOutcomeUnknown {
		t.Fatalf("ambiguous transport failure state = %s", unknown.State)
	}
	query := nativeOperationFailure(operations.Operation{Class: operations.ClassQuery}, errors.New("connection reset"))
	if query.State != operations.StateFailed {
		t.Fatalf("query transport failure state = %s", query.State)
	}
}

func TestNativeClientBrokerAndExecutorHonorCancellationAndValidation(t *testing.T) {
	broker := newNativeClientBroker()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := broker.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	if broker.ServerInstallationID() != "" {
		t.Fatalf("empty broker server identity = %q", broker.ServerInstallationID())
	}
	result := (nativeOperationExecutor{clients: broker}).Execute(context.Background(), operations.Operation{
		Command: "run-pipeline", Arguments: map[string]string{"pipelineDbId": "0"},
	})
	if result.State != operations.StateFailed || result.Err == nil {
		t.Fatalf("invalid operation result = %#v", result)
	}
}

func TestValidateNativeOperationRejectsInvalidIntentBeforeTransport(t *testing.T) {
	if err := validateNativeOperation(operations.Operation{
		Command: "run-pipeline", Arguments: map[string]string{"pipelineDbId": "0"},
	}); err == nil {
		t.Fatal("expected invalid pipeline operation to fail validation")
	}
	if err := validateNativeOperation(operations.Operation{
		Command: "run-chain", Arguments: map[string]string{"projectId": "2", "chainId": "release"},
	}); err != nil {
		t.Fatalf("valid chain operation failed validation: %v", err)
	}
}

func TestExecuteNativeOperationMapsEveryCommandFamily(t *testing.T) {
	tests := []struct {
		command   string
		arguments map[string]string
		wantCall  string
		wantRoute string
	}{
		{command: "run-pipeline", arguments: map[string]string{"pipelineDbId": "7", "pipelineJobId": "unit-tests", "dryRun": "true"}, wantCall: "run-pipeline"},
		{command: "run-chain", arguments: map[string]string{"projectId": "2", "chainId": "release"}, wantCall: "run-chain"},
		{command: "clear-queue", wantCall: "clear-queue"},
		{command: "remove-execution", arguments: map[string]string{"jobExecutionId": "job-1"}, wantCall: "remove-execution"},
		{command: "flush-history", wantCall: "flush-history"},
		{command: "delete-execution", arguments: map[string]string{"jobExecutionIds": "job-1, job-2"}, wantCall: "flush-history"},
		{command: "cancel-execution", arguments: map[string]string{"jobExecutionId": "job-1"}, wantCall: "cancel-execution"},
		{command: "rerun-execution", arguments: map[string]string{"jobExecutionId": "job-1"}, wantCall: "rerun-execution", wantRoute: "/jobs/job-1-rerun"},
		{command: "agent-action", arguments: map[string]string{"agentId": "agent-1", "action": "restart"}, wantCall: "agent-action"},
		{command: "project-action", arguments: map[string]string{"projectId": "2", "action": "reload"}, wantCall: "project-action"},
		{command: "import-project", arguments: map[string]string{"repoUrl": "https://example.com/ciwi.git"}, wantCall: "import-project"},
		{command: "check-server-updates", wantCall: "check-server-updates"},
		{command: "refresh-rollback-versions", wantCall: "refresh-rollback-versions"},
		{command: "server-update-action", arguments: map[string]string{"action": "apply", "targetVersion": "v1.2.3"}, wantCall: "server-update-action"},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			client := &recordingNativeActionClient{}
			effect, err := executeNativeOperation(context.Background(), client, operations.Operation{
				Command: test.command, Arguments: test.arguments, IdempotencyKey: "request-key",
			})
			if err != nil {
				t.Fatal(err)
			}
			if client.called != test.wantCall {
				t.Fatalf("called = %q, want %q", client.called, test.wantCall)
			}
			if test.wantCall != "check-server-updates" && test.wantCall != "refresh-rollback-versions" && client.idempotencyKey != "request-key" {
				t.Fatalf("idempotency key = %q", client.idempotencyKey)
			}
			if effect.NavigateRoute != test.wantRoute {
				t.Fatalf("navigate route = %q, want %q", effect.NavigateRoute, test.wantRoute)
			}
		})
	}
}

func TestExecuteNativeOperationPreservesRemoteFailure(t *testing.T) {
	client := &recordingNativeActionClient{err: &cnpclient.Error{
		Code: cnpv1.StatusCode_STATUS_CODE_CONFLICT, Message: "already running",
	}}
	_, err := executeNativeOperation(context.Background(), client, operations.Operation{
		Command: "clear-queue", IdempotencyKey: "request-key",
	})
	if err == nil || !errors.Is(err, client.err) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateNativeOperationCoversRequiredArguments(t *testing.T) {
	for _, operation := range []operations.Operation{
		{Command: "run-chain", Arguments: map[string]string{"projectId": "1"}},
		{Command: "remove-execution"},
		{Command: "cancel-execution"},
		{Command: "rerun-execution"},
		{Command: "delete-execution"},
		{Command: "agent-action", Arguments: map[string]string{"agentId": "agent-1"}},
		{Command: "project-action", Arguments: map[string]string{"projectId": "1"}},
		{Command: "import-project"},
		{Command: "server-update-action", Arguments: map[string]string{"action": "apply"}},
		{Command: "unsupported"},
	} {
		if err := validateNativeOperation(operation); err == nil {
			t.Fatalf("operation %#v unexpectedly validated", operation)
		}
	}
}
