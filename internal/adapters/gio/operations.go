//go:build darwin || ios || linux || windows

package gio

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/cnpclient"
)

// nativeClientBroker lets operations wait for a usable transport without
// coupling the presentation coordinator to CNP or connection management.
type nativeClientBroker struct {
	mu                   sync.RWMutex
	client               *cnpclient.Client
	serverInstallationID string
	changed              chan struct{}
}

func newNativeClientBroker() *nativeClientBroker {
	return &nativeClientBroker{changed: make(chan struct{}, 1)}
}

func (b *nativeClientBroker) Set(client *cnpclient.Client) {
	b.mu.Lock()
	b.client = client
	b.serverInstallationID = ""
	if client != nil && client.Welcome() != nil {
		b.serverInstallationID = strings.TrimSpace(client.Welcome().ServerInstallationId)
	}
	b.mu.Unlock()
	select {
	case b.changed <- struct{}{}:
	default:
	}
}

func (b *nativeClientBroker) ServerInstallationID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.serverInstallationID
}

func (b *nativeClientBroker) Wait(ctx context.Context) (*cnpclient.Client, error) {
	for {
		b.mu.RLock()
		client := b.client
		b.mu.RUnlock()
		if client != nil {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.changed:
		}
	}
}

type nativeOperationEffect struct {
	Message       string
	Refresh       bool
	NavigateRoute string
	Notice        bool
	NoticeRoute   string
	NoticeLabel   string
	NoticeSection string
	CancelledJob  string
	Value         any
}

func noticeEffect(notice presentation.TransientNotice) nativeOperationEffect {
	return nativeOperationEffect{
		Message: notice.Message, Notice: true, NoticeRoute: notice.Route,
		NoticeLabel: notice.ActionLabel, NoticeSection: notice.Section,
	}
}

// nativeActionClient is the narrow command surface used by the presentation
// operation adapter. Keeping it separate from the concrete CNP client makes
// command mapping and failure semantics testable without a live transport.
type nativeActionClient interface {
	RunPipeline(context.Context, *cnpv1.RunPipelineRequest, string) (*cnpv1.RunPipelineResult, error)
	RunPipelineChain(context.Context, *cnpv1.RunPipelineChainRequest, string) (*cnpv1.RunPipelineChainResult, error)
	ClearExecutionQueue(context.Context, string) (*cnpv1.ClearExecutionQueueResult, error)
	RemoveQueuedExecution(context.Context, string, string) (*cnpv1.RemoveQueuedExecutionResult, error)
	FlushExecutionHistory(context.Context, *cnpv1.FlushExecutionHistoryRequest, string) (*cnpv1.FlushExecutionHistoryResult, error)
	CancelExecution(context.Context, string, string) (*cnpv1.CancelExecutionResult, error)
	RerunExecution(context.Context, string, string) (*cnpv1.RerunExecutionResult, error)
	AgentAction(context.Context, *cnpv1.AgentActionRequest, string) (*cnpv1.AgentActionResult, error)
	RunAgentScript(context.Context, *cnpv1.RunAgentScriptRequest, string) (*cnpv1.RunAgentScriptResult, error)
	ProjectAction(context.Context, int64, string, string) (*cnpv1.ProjectActionResult, error)
	ImportProject(context.Context, *cnpv1.ImportProjectRequest, string) (*cnpv1.ImportProjectResult, error)
	ValidateManagedYAML(context.Context, *cnpv1.ManagedYAMLRequest) (*cnpv1.ManagedYAMLDefinition, error)
	SaveManagedYAML(context.Context, *cnpv1.ManagedYAMLRequest, string) (*cnpv1.ManagedYAMLDefinition, error)
	UpsertVaultConnection(context.Context, *cnpv1.UpsertVaultConnectionRequest, string) (*cnpv1.VaultConnection, error)
	TestVaultConnection(context.Context, int64) (*cnpv1.TestVaultConnectionResult, error)
	DeleteVaultConnection(context.Context, int64, string) (*cnpv1.DeleteVaultConnectionResult, error)
	CheckServerUpdates(context.Context) (*cnpv1.ServerUpdateCheckResult, error)
	ListServerUpdateVersions(context.Context) (*cnpv1.ServerUpdateVersions, error)
	ServerUpdateActionWithKey(context.Context, string, string, string) (*cnpv1.ServerUpdateActionResult, error)
}

type nativeOperationExecutor struct{ clients *nativeClientBroker }

func (e nativeOperationExecutor) Execute(ctx context.Context, operation operations.Operation) operations.Result {
	if err := validateNativeOperation(operation); err != nil {
		return operations.Result{State: operations.StateFailed, Err: err}
	}
	client, err := e.clients.Wait(ctx)
	if err != nil {
		return nativeOperationFailure(operation, err)
	}
	effect, err := executeNativeOperation(ctx, client, operation)
	if err != nil {
		return nativeOperationFailure(operation, err)
	}
	return operations.Result{State: operations.StateSucceeded, Message: effect.Message, Value: effect}
}

func nativeOperationFailure(operation operations.Operation, err error) operations.Result {
	state := operations.StateFailed
	message := ""
	var remoteError *cnpclient.Error
	if operation.Class == operations.ClassMutation && !errors.As(err, &remoteError) {
		state = operations.StateOutcomeUnknown
		message = "The connection ended before ciwi could confirm the action outcome. It will verify the receipt before considering a safe retry: " + err.Error()
	}
	return operations.Result{State: state, Message: message, Err: err}
}

func validateNativeOperation(operation operations.Operation) error {
	arguments := operation.Arguments
	require := func(key, label string) error {
		if strings.TrimSpace(arguments[key]) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
	switch operation.Command {
	case "run-pipeline":
		_, err := positiveInt64(arguments["pipelineDbId"], "pipeline identifier")
		return err
	case "run-chain":
		if _, err := positiveInt64(arguments["projectId"], "project identifier"); err != nil {
			return err
		}
		return require("chainId", "pipeline chain identifier")
	case "remove-execution", "cancel-execution", "rerun-execution":
		return require("jobExecutionId", "job execution identifier")
	case "delete-execution":
		if len(splitExecutionIDs(arguments["jobExecutionIds"])) == 0 {
			return fmt.Errorf("job execution identifiers are required")
		}
	case "agent-action":
		if err := require("agentId", "agent identifier"); err != nil {
			return err
		}
		return require("action", "agent action")
	case "run-agent-script":
		if err := require("agentId", "agent identifier"); err != nil {
			return err
		}
		if err := require("shell", "script shell"); err != nil {
			return err
		}
		return require("script", "script")
	case "project-action":
		if _, err := positiveInt64(arguments["projectId"], "project identifier"); err != nil {
			return err
		}
		return require("action", "project action")
	case "import-project":
		return require("repoUrl", "repository URL")
	case "validate-managed-yaml", "save-managed-yaml":
		return require("yaml", "YAML definition")
	case "save-vault-connection":
		for _, item := range [][2]string{{"name", "connection name"}, {"url", "Vault URL"}, {"roleId", "AppRole role ID"}, {"secretIdEnv", "secret ID environment variable"}} {
			if err := require(item[0], item[1]); err != nil {
				return err
			}
		}
	case "test-vault-connection", "delete-vault-connection":
		_, err := positiveInt64(arguments["id"], "Vault connection identifier")
		return err
	case "server-update-action":
		if err := require("action", "server update action"); err != nil {
			return err
		}
		action := strings.TrimSpace(arguments["action"])
		if (action == "apply" || action == "rollback") && strings.TrimSpace(arguments["targetVersion"]) == "" {
			return fmt.Errorf("target version is required")
		}
	case "clear-queue", "flush-history", "check-server-updates", "refresh-rollback-versions", "refresh":
		return nil
	default:
		return fmt.Errorf("unsupported coordinated native action %q", operation.Command)
	}
	return nil
}

func executeNativeOperation(ctx context.Context, client nativeActionClient, operation operations.Operation) (nativeOperationEffect, error) {
	arguments := operation.Arguments
	key := operation.IdempotencyKey
	switch operation.Command {
	case "run-pipeline":
		pipelineID, err := positiveInt64(arguments["pipelineDbId"], "pipeline identifier")
		if err != nil {
			return nativeOperationEffect{}, err
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.RunPipeline(commandCtx, &cnpv1.RunPipelineRequest{
			PipelineDbId: pipelineID, Selection: pipelineRunSelection(arguments),
		}, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("run pipeline: %w", err)
		}
		return noticeEffect(presentation.QueuedPipelineNotice(
			result.ProjectName, result.PipelineId, arguments["pipelineJobId"], int(result.Enqueued),
			arguments["dryRun"] == "true", result.JobExecutionIds,
		)), nil
	case "run-chain":
		projectID, err := positiveInt64(arguments["projectId"], "project identifier")
		chainID := strings.TrimSpace(arguments["chainId"])
		if err != nil || chainID == "" {
			return nativeOperationEffect{}, fmt.Errorf("invalid pipeline chain identifier")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.RunPipelineChain(commandCtx, &cnpv1.RunPipelineChainRequest{
			ProjectId: projectID, ChainId: chainID, Selection: pipelineRunSelection(arguments),
		}, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("run chain: %w", err)
		}
		label := strings.TrimSpace(result.ChainName)
		if label == "" {
			label = strings.TrimSpace(result.ChainId)
		}
		return noticeEffect(presentation.QueuedChainNotice(
			result.ProjectName, label, int(result.Enqueued), arguments["dryRun"] == "true",
		)), nil
	case "clear-queue":
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.ClearExecutionQueue(commandCtx, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("clear queue: %w", err)
		}
		return nativeOperationEffect{Message: fmt.Sprintf("Cleared %d queued execution(s)", result.Cleared)}, nil
	case "remove-execution":
		jobID := strings.TrimSpace(arguments["jobExecutionId"])
		if jobID == "" {
			return nativeOperationEffect{}, fmt.Errorf("no execution identifier was supplied")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.RemoveQueuedExecution(commandCtx, jobID, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("remove execution: %w", err)
		}
		return nativeOperationEffect{Message: "Removed queued execution " + result.JobExecutionId}, nil
	case "flush-history", "delete-execution":
		request := &cnpv1.FlushExecutionHistoryRequest{All: operation.Command == "flush-history"}
		if !request.All {
			request.JobExecutionIds = splitExecutionIDs(arguments["jobExecutionIds"])
			if len(request.JobExecutionIds) == 0 {
				return nativeOperationEffect{}, fmt.Errorf("no execution identifiers were supplied")
			}
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.FlushExecutionHistory(commandCtx, request, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("flush history: %w", err)
		}
		return nativeOperationEffect{Message: fmt.Sprintf("Removed %d execution(s) from history", result.Flushed), Refresh: true}, nil
	case "cancel-execution":
		jobID := strings.TrimSpace(arguments["jobExecutionId"])
		if jobID == "" {
			return nativeOperationEffect{}, fmt.Errorf("no execution identifier was supplied")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.CancelExecution(commandCtx, jobID, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("cancel execution: %w", err)
		}
		return nativeOperationEffect{Message: "Execution " + result.JobExecutionId + " marked failed", Refresh: true, CancelledJob: result.JobExecutionId}, nil
	case "rerun-execution":
		jobID := strings.TrimSpace(arguments["jobExecutionId"])
		if jobID == "" {
			return nativeOperationEffect{}, fmt.Errorf("no execution identifier was supplied")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.RerunExecution(commandCtx, jobID, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("rerun execution: %w", err)
		}
		return noticeEffect(presentation.QueuedJobNotice("Queued rerun "+result.JobExecutionId, result.JobExecutionId)), nil
	case "agent-action":
		agentID, action := strings.TrimSpace(arguments["agentId"]), strings.TrimSpace(arguments["action"])
		if agentID == "" || action == "" {
			return nativeOperationEffect{}, fmt.Errorf("agent action is incomplete")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.AgentAction(commandCtx, &cnpv1.AgentActionRequest{AgentId: agentID, Action: action}, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("agent action: %w", err)
		}
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "Agent request accepted"
		}
		return nativeOperationEffect{Message: message, Refresh: true, Notice: true}, nil
	case "run-agent-script":
		agentID := strings.TrimSpace(arguments["agentId"])
		shell := strings.TrimSpace(arguments["shell"])
		script := strings.TrimSpace(arguments["script"])
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.RunAgentScript(commandCtx, &cnpv1.RunAgentScriptRequest{
			AgentId: agentID, Shell: shell, Script: script, TimeoutSeconds: 600,
		}, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("run agent script: %w", err)
		}
		effect := noticeEffect(presentation.QueuedJobNotice("Queued ad-hoc script on "+result.AgentId, result.JobExecutionId))
		effect.NavigateRoute = "/jobs/" + result.JobExecutionId
		return effect, nil
	case "project-action":
		projectID, err := positiveInt64(arguments["projectId"], "project identifier")
		action := strings.TrimSpace(arguments["action"])
		if err != nil || action == "" {
			return nativeOperationEffect{}, fmt.Errorf("project action is incomplete")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		result, err := client.ProjectAction(commandCtx, projectID, action, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("project action: %w", err)
		}
		message := strings.TrimSpace(result.Message)
		if action == "reload" && message == "" {
			message = "Reloaded successfully"
		}
		return nativeOperationEffect{Message: message, Refresh: true}, nil
	case "import-project":
		repoURL := strings.TrimSpace(arguments["repoUrl"])
		if repoURL == "" {
			return nativeOperationEffect{}, fmt.Errorf("repository URL is required")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		result, err := client.ImportProject(commandCtx, &cnpv1.ImportProjectRequest{
			RepoUrl: repoURL, RepoRef: strings.TrimSpace(arguments["repoRef"]), ConfigFile: strings.TrimSpace(arguments["configFile"]),
		}, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("import project: %w", err)
		}
		return nativeOperationEffect{Message: "Imported " + result.ProjectName, Refresh: true}, nil
	case "validate-managed-yaml", "save-managed-yaml":
		projectID := int64(0)
		if rawID := strings.TrimSpace(arguments["projectId"]); rawID != "" && rawID != "0" {
			var err error
			projectID, err = positiveInt64(rawID, "project identifier")
			if err != nil {
				return nativeOperationEffect{}, err
			}
		}
		request := &cnpv1.ManagedYAMLRequest{ProjectId: projectID, Yaml: arguments["yaml"], Revision: strings.TrimSpace(arguments["revision"])}
		commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if operation.Command == "validate-managed-yaml" {
			result, err := client.ValidateManagedYAML(commandCtx, request)
			if err != nil {
				return nativeOperationEffect{}, fmt.Errorf("validate managed YAML: %w", err)
			}
			return nativeOperationEffect{Message: fmt.Sprintf("Valid: %s — %d pipeline(s), %d chain(s)", result.ProjectName, result.Pipelines, result.PipelineChains), Notice: true}, nil
		}
		result, err := client.SaveManagedYAML(commandCtx, request, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("save managed YAML: %w", err)
		}
		return nativeOperationEffect{Message: "Saved " + result.ProjectName, NavigateRoute: "/settings", Refresh: true, Notice: true}, nil
	case "save-vault-connection":
		commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		result, err := client.UpsertVaultConnection(commandCtx, &cnpv1.UpsertVaultConnectionRequest{
			Name: strings.TrimSpace(arguments["name"]), Url: strings.TrimSpace(arguments["url"]), AuthMethod: "approle",
			ApproleMount: strings.TrimSpace(arguments["approleMount"]), RoleId: strings.TrimSpace(arguments["roleId"]),
			SecretIdEnv: strings.TrimSpace(arguments["secretIdEnv"]),
		}, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("save Vault connection: %w", err)
		}
		return nativeOperationEffect{Message: "Saved Vault connection " + result.Name, Refresh: true, Notice: true}, nil
	case "test-vault-connection":
		id, err := positiveInt64(arguments["id"], "Vault connection identifier")
		if err != nil {
			return nativeOperationEffect{}, err
		}
		commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		result, err := client.TestVaultConnection(commandCtx, id)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("test Vault connection: %w", err)
		}
		if !result.Ok {
			return nativeOperationEffect{}, fmt.Errorf("Vault test failed: %s", result.Message)
		}
		return nativeOperationEffect{Message: result.Message, Notice: true}, nil
	case "delete-vault-connection":
		id, err := positiveInt64(arguments["id"], "Vault connection identifier")
		if err != nil {
			return nativeOperationEffect{}, err
		}
		commandCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if _, err := client.DeleteVaultConnection(commandCtx, id, key); err != nil {
			return nativeOperationEffect{}, fmt.Errorf("delete Vault connection: %w", err)
		}
		return nativeOperationEffect{Message: "Deleted Vault connection", Refresh: true, Notice: true}, nil
	case "check-server-updates":
		commandCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		result, err := client.CheckServerUpdates(commandCtx)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("check for updates: %w", err)
		}
		return nativeOperationEffect{Value: result}, nil
	case "refresh-rollback-versions":
		commandCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		result, err := client.ListServerUpdateVersions(commandCtx)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("refresh rollback versions: %w", err)
		}
		return nativeOperationEffect{Value: result}, nil
	case "server-update-action":
		action, target := strings.TrimSpace(arguments["action"]), strings.TrimSpace(arguments["targetVersion"])
		if (action == "apply" || action == "rollback") && target == "" {
			return nativeOperationEffect{}, fmt.Errorf("select a version first")
		}
		commandCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		result, err := client.ServerUpdateActionWithKey(commandCtx, action, target, key)
		if err != nil {
			return nativeOperationEffect{}, fmt.Errorf("server update action: %w", err)
		}
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = "Request accepted"
		}
		return nativeOperationEffect{Message: message, Value: result}, nil
	case "refresh":
		return nativeOperationEffect{Refresh: true, Message: "Refreshed"}, nil
	default:
		return nativeOperationEffect{}, fmt.Errorf("unsupported coordinated native action %q", operation.Command)
	}
}

func positiveInt64(raw, label string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid %s", label)
	}
	return value, nil
}
