package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	ServerUpdateActionApply    = "apply"
	ServerUpdateActionRollback = "rollback"
	ServerUpdateActionRestart  = "restart"
)

type ServerUpdateStatus struct {
	CurrentVersion      string
	LatestVersion       string
	UpdateAvailable     bool
	LastCheckedUTC      string
	LastApplyStatus     string
	LastApplyUTC        string
	Message             string
	ServerMode          string
	SelfUpdateSupported bool
	SelfUpdateReason    string
	AgentTargetVersion  string
	BlockedAgentIDs     []string
}

type ServerUpdateCheckResult struct {
	CurrentVersion    string
	LatestVersion     string
	AvailableVersions []string
	UpdateAvailable   bool
	ReleaseURL        string
	AssetName         string
	Message           string
}

type ServerUpdateVersions struct {
	Versions       []string
	CurrentVersion string
}

type ServerUpdateActionRequest struct {
	Action         string
	TargetVersion  string
	IdempotencyKey string
}

type ServerUpdateActionResult struct {
	Updated        bool
	Restarting     bool
	Staged         bool
	Message        string
	TargetVersion  string
	CurrentVersion string
}

type ServerUpdateBackend interface {
	GetServerUpdateStatus(context.Context) (ServerUpdateStatus, error)
	CheckForServerUpdates(context.Context) (ServerUpdateCheckResult, error)
	ListServerUpdateVersions(context.Context) (ServerUpdateVersions, error)
	ExecuteServerUpdateAction(context.Context, ServerUpdateActionRequest) (ServerUpdateActionResult, error)
}

type ServerUpdateOperations struct {
	backend  ServerUpdateBackend
	changes  *ChangeHub
	receipts CommandReceiptRepository
}

func NewServerUpdateOperations(backend ServerUpdateBackend, changes *ChangeHub, receipts ...CommandReceiptRepository) *ServerUpdateOperations {
	var receiptRepository CommandReceiptRepository
	if len(receipts) > 0 {
		receiptRepository = receipts[0]
	}
	return &ServerUpdateOperations{backend: backend, changes: changes, receipts: receiptRepository}
}

func (o *ServerUpdateOperations) Status(ctx context.Context) (ServerUpdateStatus, error) {
	if o == nil || o.backend == nil {
		return ServerUpdateStatus{}, NewError(ErrorUnavailable, "server update service unavailable", nil)
	}
	return o.backend.GetServerUpdateStatus(ctx)
}

func (o *ServerUpdateOperations) Check(ctx context.Context) (ServerUpdateCheckResult, error) {
	if o == nil || o.backend == nil {
		return ServerUpdateCheckResult{}, NewError(ErrorUnavailable, "server update service unavailable", nil)
	}
	result, err := o.backend.CheckForServerUpdates(ctx)
	if err == nil && o.changes != nil {
		o.changes.Publish(ChangeUpdates)
	}
	return result, err
}

func (o *ServerUpdateOperations) Versions(ctx context.Context) (ServerUpdateVersions, error) {
	if o == nil || o.backend == nil {
		return ServerUpdateVersions{}, NewError(ErrorUnavailable, "server update service unavailable", nil)
	}
	return o.backend.ListServerUpdateVersions(ctx)
}

func (o *ServerUpdateOperations) Execute(ctx context.Context, request ServerUpdateActionRequest) (ServerUpdateActionResult, error) {
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	request.TargetVersion = strings.TrimSpace(request.TargetVersion)
	if request.Action != ServerUpdateActionApply && request.Action != ServerUpdateActionRollback && request.Action != ServerUpdateActionRestart {
		return ServerUpdateActionResult{}, NewError(ErrorInvalidArgument, "a valid server update action is required", nil)
	}
	if request.Action == ServerUpdateActionRollback && request.TargetVersion == "" {
		return ServerUpdateActionResult{}, NewError(ErrorInvalidArgument, "a rollback version is required", nil)
	}
	if o == nil || o.backend == nil {
		return ServerUpdateActionResult{}, NewError(ErrorUnavailable, "server update service unavailable", nil)
	}
	key, err := validateCommandKey(request.IdempotencyKey)
	if err != nil {
		return ServerUpdateActionResult{}, err
	}
	sum := sha256.Sum256([]byte(request.Action + "\x00" + request.TargetVersion))
	result, err := executeIdempotentCommand(ctx, o.receipts, key, "server_update_action", hex.EncodeToString(sum[:]), func() (ServerUpdateActionResult, error) {
		return o.backend.ExecuteServerUpdateAction(ctx, request)
	})
	if err == nil && o.changes != nil {
		o.changes.Publish(ChangeUpdates, ChangeAgents)
	}
	return result, err
}
