package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/izzyreal/ciwi/internal/domain"
)

const agentActionOperation = "agent_action"

const (
	AgentActionAuthorize       = "authorize"
	AgentActionUnauthorize     = "unauthorize"
	AgentActionActivate        = "activate"
	AgentActionDeactivate      = "deactivate"
	AgentActionRefreshTools    = "refresh-tools"
	AgentActionRestart         = "restart"
	AgentActionUpdate          = "update"
	AgentActionDelete          = "delete"
	AgentActionWipeCache       = "wipe-cache"
	AgentActionFlushJobHistory = "flush-job-history"
)

type AgentRepository interface {
	ListAgents(context.Context) ([]domain.Agent, error)
}

type AgentQueries struct {
	repository AgentRepository
}

func NewAgentQueries(repository AgentRepository) *AgentQueries {
	return &AgentQueries{repository: repository}
}

func (q *AgentQueries) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	if q == nil || q.repository == nil {
		return nil, NewError(ErrorUnavailable, "agent repository unavailable", nil)
	}
	agents, err := q.repository.ListAgents(ctx)
	if err != nil {
		return nil, WrapInternal("list agents", err)
	}
	return agents, nil
}

type AgentActionRequest struct {
	AgentID        string
	Action         string
	IdempotencyKey string
}

type AgentActionResult struct {
	Requested bool   `json:"requested"`
	AgentID   string `json:"agent_id,omitempty"`
	Message   string `json:"message,omitempty"`
	Target    string `json:"target,omitempty"`
}

type AgentMutator interface {
	ExecuteAgentAction(context.Context, AgentActionRequest) (AgentActionResult, error)
}

type AgentCommands struct {
	mutator  AgentMutator
	receipts CommandReceiptRepository
	changes  *ChangeHub
}

func NewAgentCommands(mutator AgentMutator, receipts CommandReceiptRepository, changes *ChangeHub) *AgentCommands {
	return &AgentCommands{mutator: mutator, receipts: receipts, changes: changes}
}

func (c *AgentCommands) Execute(ctx context.Context, request AgentActionRequest) (AgentActionResult, error) {
	request.AgentID = strings.TrimSpace(request.AgentID)
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	if request.AgentID == "" || request.Action == "" {
		return AgentActionResult{}, NewError(ErrorInvalidArgument, "agent id and action are required", nil)
	}
	if !supportedAgentAction(request.Action) {
		return AgentActionResult{}, NewError(ErrorInvalidArgument, "unsupported agent action", nil)
	}
	if c == nil || c.mutator == nil {
		return AgentActionResult{}, NewError(ErrorUnavailable, "agent operator unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return AgentActionResult{}, err
	}
	execute := func() (AgentActionResult, error) {
		result, executeErr := c.mutator.ExecuteAgentAction(ctx, request)
		if executeErr == nil && c.changes != nil {
			c.changes.Publish(ChangeAgents, ChangeAgentEligibility)
		}
		return result, executeErr
	}
	if key == "" || c.receipts == nil {
		return execute()
	}
	fingerprintRequest := request
	fingerprintRequest.IdempotencyKey = ""
	payload, err := json.Marshal(fingerprintRequest)
	if err != nil {
		return AgentActionResult{}, WrapInternal("fingerprint agent action", err)
	}
	sum := sha256.Sum256(payload)
	return executeIdempotentCommand(ctx, c.receipts, key, agentActionOperation, hex.EncodeToString(sum[:]), execute)
}

func supportedAgentAction(action string) bool {
	switch action {
	case AgentActionAuthorize, AgentActionUnauthorize, AgentActionActivate, AgentActionDeactivate,
		AgentActionRefreshTools, AgentActionRestart, AgentActionUpdate, AgentActionDelete,
		AgentActionWipeCache, AgentActionFlushJobHistory:
		return true
	default:
		return false
	}
}
