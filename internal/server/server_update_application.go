package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/izzyreal/ciwi/internal/application"
)

type serverUpdateAdapter struct {
	state *stateStore
}

func (a serverUpdateAdapter) GetServerUpdateStatus(ctx context.Context) (application.ServerUpdateStatus, error) {
	if err := ctx.Err(); err != nil {
		return application.ServerUpdateStatus{}, err
	}
	result, err := a.state.getUpdateStatus()
	if err != nil {
		return application.ServerUpdateStatus{}, application.WrapInternal("load server update status", err)
	}
	status := result.Status
	return application.ServerUpdateStatus{
		CurrentVersion: status["update_current_version"], LatestVersion: status[updateKeyLatestVersion],
		UpdateAvailable: status["update_available"] == "1", LastCheckedUTC: status[updateKeyLastCheckedUTC],
		LastApplyStatus: status[updateKeyLastApplyStatus], LastApplyUTC: status["update_last_apply_utc"],
		Message: status[updateKeyMessage], ServerMode: status["update_server_mode"],
		SelfUpdateSupported: status["update_server_self_update_supported"] == "1",
		SelfUpdateReason:    status["update_server_self_update_reason"], AgentTargetVersion: status["update_agent_target_version"],
		BlockedAgentIDs: splitNonempty(status["update_agent_non_service_agents"]),
	}, nil
}

func (a serverUpdateAdapter) CheckForServerUpdates(ctx context.Context) (application.ServerUpdateCheckResult, error) {
	if err := ctx.Err(); err != nil {
		return application.ServerUpdateCheckResult{}, err
	}
	result := a.state.checkForUpdates(ctx)
	return application.ServerUpdateCheckResult{
		CurrentVersion: result.CurrentVersion, LatestVersion: result.LatestVersion,
		AvailableVersions: append([]string(nil), result.AvailableVersions...), UpdateAvailable: result.UpdateAvailable,
		ReleaseURL: result.ReleaseURL, AssetName: result.AssetName, Message: result.Message,
	}, nil
}

func (a serverUpdateAdapter) ListServerUpdateVersions(ctx context.Context) (application.ServerUpdateVersions, error) {
	result, err := a.state.listUpdateTags(ctx)
	if err != nil {
		return application.ServerUpdateVersions{}, application.NewError(application.ErrorUnavailable, err.Error(), err)
	}
	versions := make([]string, 0, len(result.Tags))
	for _, version := range result.Tags {
		if isVersionNewer(result.CurrentVersion, version) {
			versions = append(versions, strings.TrimSpace(version))
		}
	}
	return application.ServerUpdateVersions{Versions: versions, CurrentVersion: result.CurrentVersion}, nil
}

func (a serverUpdateAdapter) ExecuteServerUpdateAction(ctx context.Context, request application.ServerUpdateActionRequest) (application.ServerUpdateActionResult, error) {
	if request.Action == application.ServerUpdateActionRestart {
		message := a.state.requestServerRestart()
		_ = a.state.persistUpdateStatus(map[string]string{updateKeyMessage: message})
		return application.ServerUpdateActionResult{Restarting: true, Message: message, CurrentVersion: currentVersion()}, nil
	}
	rollback := request.Action == application.ServerUpdateActionRollback
	result, err := a.state.applyUpdateTarget(ctx, request.TargetVersion, rollback)
	if err != nil {
		kind := application.ErrorInternal
		if applyErr, ok := err.(*updateApplyError); ok {
			switch applyErr.StatusCode {
			case http.StatusBadRequest:
				kind = application.ErrorInvalidArgument
			case http.StatusConflict:
				kind = application.ErrorConflict
			case http.StatusServiceUnavailable:
				kind = application.ErrorUnavailable
			}
		}
		return application.ServerUpdateActionResult{}, application.NewError(kind, err.Error(), err)
	}
	target := result.TargetVersion
	if target == "" {
		target = result.Target
	}
	return application.ServerUpdateActionResult{
		Updated: result.Updated, Restarting: result.Updated && !result.Staged, Staged: result.Staged,
		Message: result.Message, TargetVersion: target, CurrentVersion: result.CurrentVersion,
	}, nil
}

func splitNonempty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
