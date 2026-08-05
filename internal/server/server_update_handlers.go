package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/izzyreal/ciwi/internal/application"
)

var (
	serverExecutablePathFn              = os.Executable
	serverLooksLikeGoRunBinaryFn        = looksLikeGoRunBinary
	serverDownloadUpdateAssetFn         = downloadUpdateAsset
	serverDownloadTextAssetFn           = downloadTextAsset
	serverVerifyFileSHA256Fn            = verifyFileSHA256
	serverIsLinuxSystemUpdaterEnabledFn = isLinuxSystemUpdaterEnabled
	serverStageLinuxUpdateBinaryFn      = stageLinuxUpdateBinary
	serverTriggerLinuxSystemUpdaterFn   = triggerLinuxSystemUpdater
	serverCopyFileFn                    = copyFile
	serverStartUpdateHelperFn           = startUpdateHelper
	serverExeExtFn                      = exeExt
)

const (
	updateKeyLastCheckedUTC     = "update_last_checked_utc"
	updateKeyMessage            = "update_message"
	updateKeyLatestVersion      = "update_latest_version"
	updateKeyLastApplyStatus    = "update_last_apply_status"
	updateKeyReloadProjectsPend = "update_reload_projects_pending"
	updateStatusRunning         = "running"
	updateStatusFailed          = "failed"
	updateStatusNoop            = "noop"
	updateStatusStaged          = "staged"
	updateStatusSuccess         = "success"
)

func (s *stateStore) updateCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.app().updates.Check(r.Context())
	if err != nil {
		writeApplicationHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updateCheckResponse{
		CurrentVersion: result.CurrentVersion, LatestVersion: result.LatestVersion,
		AvailableVersions: append([]string(nil), result.AvailableVersions...), UpdateAvailable: result.UpdateAvailable,
		ReleaseURL: result.ReleaseURL, AssetName: result.AssetName, Message: result.Message,
	})
}

func (s *stateStore) checkForUpdates(ctx context.Context) updateCheckResponse {
	infos, err := s.fetchAvailableUpdateInfos(ctx)
	if err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			updateKeyLastCheckedUTC:  time.Now().UTC().Format(time.RFC3339Nano),
			"update_current_version": currentVersion(),
			updateKeyMessage:         err.Error(),
			"update_available":       "0",
		})
		return updateCheckResponse{
			CurrentVersion: currentVersion(),
			Message:        err.Error(),
		}
	}

	current := currentVersion()
	availableVersions := make([]string, 0, len(infos))
	for _, candidate := range infos {
		if isVersionNewer(candidate.TagName, current) {
			availableVersions = append(availableVersions, candidate.TagName)
		}
	}
	info := infos[0]
	if len(availableVersions) > 0 {
		for _, candidate := range infos {
			if candidate.TagName == availableVersions[0] {
				info = candidate
				break
			}
		}
	}
	resp := updateCheckResponse{
		CurrentVersion:    current,
		LatestVersion:     info.TagName,
		AvailableVersions: availableVersions,
		UpdateAvailable:   len(availableVersions) > 0,
		ReleaseURL:        info.HTMLURL,
		AssetName:         info.Asset.Name,
	}
	if !resp.UpdateAvailable {
		resp.Message = "already up to date"
	}
	_ = s.persistUpdateStatus(map[string]string{
		updateKeyLastCheckedUTC:  time.Now().UTC().Format(time.RFC3339Nano),
		"update_current_version": currentVersion(),
		updateKeyLatestVersion:   info.TagName,
		"update_release_url":     info.HTMLURL,
		"update_asset_name":      info.Asset.Name,
		"update_available":       boolString(resp.UpdateAvailable),
		updateKeyMessage:         resp.Message,
	})
	return resp
}

func (s *stateStore) updateApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TargetVersion string `json:"target_version"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
	}
	s.applyUpdateTargetHandler(w, r, strings.TrimSpace(req.TargetVersion), false)
}

func (s *stateStore) updateRollbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TargetVersion string `json:"target_version"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
	}
	target := strings.TrimSpace(req.TargetVersion)
	if target == "" {
		http.Error(w, "target_version is required", http.StatusBadRequest)
		return
	}
	s.applyUpdateTargetHandler(w, r, target, true)
}

func (s *stateStore) updateTagsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.listUpdateTags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *stateStore) listUpdateTags(ctx context.Context) (updateTagsResponse, error) {
	tags, err := s.fetchUpdateTags(ctx)
	if err != nil {
		return updateTagsResponse{}, err
	}
	current := strings.TrimSpace(currentVersion())
	if current != "" {
		seen := false
		for _, t := range tags {
			if strings.TrimSpace(t) == current {
				seen = true
				break
			}
		}
		if !seen {
			tags = append([]string{current}, tags...)
		}
	}
	return updateTagsResponse{
		Tags:           tags,
		CurrentVersion: current,
	}, nil
}

func (s *stateStore) applyUpdateTargetHandler(w http.ResponseWriter, r *http.Request, targetVersion string, rollback bool) {
	action := application.ServerUpdateActionApply
	if rollback {
		action = application.ServerUpdateActionRollback
	}
	result, err := s.app().updates.Execute(r.Context(), application.ServerUpdateActionRequest{
		Action: action, TargetVersion: targetVersion, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeApplicationHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updateApplyResponse{
		Updated: result.Updated, Message: result.Message, Target: result.TargetVersion,
		TargetVersion: result.TargetVersion, CurrentVersion: result.CurrentVersion, Staged: result.Staged,
	})
}

func writeApplicationHTTPError(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	switch application.ErrorKindOf(err) {
	case application.ErrorInvalidArgument:
		statusCode = http.StatusBadRequest
	case application.ErrorNotFound:
		statusCode = http.StatusNotFound
	case application.ErrorConflict:
		statusCode = http.StatusConflict
	case application.ErrorFailedPrecondition:
		statusCode = http.StatusPreconditionFailed
	case application.ErrorUnavailable:
		statusCode = http.StatusServiceUnavailable
	case application.ErrorUnsupported:
		statusCode = http.StatusNotImplemented
	}
	http.Error(w, err.Error(), statusCode)
}

type updateApplyError struct {
	StatusCode int
	Message    string
}

func (e *updateApplyError) Error() string { return e.Message }

func (s *stateStore) applyUpdateTarget(ctx context.Context, targetVersion string, rollback bool) (updateApplyResponse, error) {
	s.update.mu.Lock()
	if s.update.inProgress {
		s.update.mu.Unlock()
		return updateApplyResponse{}, &updateApplyError{StatusCode: http.StatusConflict, Message: "update already in progress"}
	}
	s.update.inProgress = true
	s.update.lastMessage = "update started"
	s.update.mu.Unlock()
	defer func() {
		s.update.mu.Lock()
		s.update.inProgress = false
		s.update.mu.Unlock()
	}()
	_ = s.persistUpdateStatus(map[string]string{
		"update_last_apply_utc":  time.Now().UTC().Format(time.RFC3339Nano),
		updateKeyLastApplyStatus: updateStatusRunning,
		updateKeyMessage:         "update started",
	})

	exePath, err := serverExecutablePathFn()
	if err != nil {
		msg := "resolve executable path: " + err.Error()
		return updateApplyResponse{}, s.failUpdateApply(http.StatusInternalServerError, msg, msg)
	}
	exePath, _ = filepath.Abs(exePath)
	if serverLooksLikeGoRunBinaryFn(exePath) {
		msg := "self-update is unavailable for go run binaries; run built ciwi binary instead"
		return updateApplyResponse{}, s.failUpdateApply(http.StatusBadRequest, msg, msg)
	}

	info, err := s.fetchUpdateInfoForTag(ctx, targetVersion)
	if err != nil {
		return updateApplyResponse{}, s.failUpdateApply(http.StatusBadRequest, err.Error(), err.Error())
	}
	if !isVersionDifferent(info.TagName, currentVersion()) {
		msg := "already at target version"
		if !rollback {
			msg = "already up to date"
		}
		_ = s.persistUpdateStatus(map[string]string{
			updateKeyLastApplyStatus: updateStatusNoop,
			updateKeyMessage:         msg,
			updateKeyLatestVersion:   info.TagName,
		})
		_ = s.setAgentUpdateTarget(currentVersion())
		return updateApplyResponse{
			Updated: false,
			Message: msg,
			Target:  info.TagName,
		}, nil
	}

	newBinPath, err := serverDownloadUpdateAssetFn(ctx, info.Asset.URL, info.Asset.Name)
	if err != nil {
		msg := "download update asset: " + err.Error()
		return updateApplyResponse{}, s.failUpdateApply(http.StatusBadRequest, msg, msg)
	}
	if strings.TrimSpace(info.ChecksumAsset.URL) != "" {
		checksumText, err := serverDownloadTextAssetFn(ctx, info.ChecksumAsset.URL)
		if err != nil {
			msg := "download checksum asset: " + err.Error()
			return updateApplyResponse{}, s.failUpdateApply(http.StatusBadRequest, msg, msg)
		}
		if err := serverVerifyFileSHA256Fn(newBinPath, info.Asset.Name, checksumText); err != nil {
			msg := "checksum verification failed: " + err.Error()
			return updateApplyResponse{}, s.failUpdateApply(http.StatusBadRequest, msg, msg)
		}
	}

	if serverIsLinuxSystemUpdaterEnabledFn() {
		if err := serverStageLinuxUpdateBinaryFn(info.TagName, info, newBinPath); err != nil {
			msg := "stage update: " + err.Error()
			return updateApplyResponse{}, s.failUpdateApply(http.StatusInternalServerError, msg, msg)
		}
		if err := serverTriggerLinuxSystemUpdaterFn(); err != nil {
			msg := "trigger updater: " + err.Error()
			return updateApplyResponse{}, s.failUpdateApply(http.StatusInternalServerError, msg, msg)
		}
		_ = s.persistUpdateStatus(map[string]string{
			updateKeyLastApplyStatus:    updateStatusStaged,
			updateKeyMessage:            updateApplyMessage(rollback, true),
			updateKeyLatestVersion:      info.TagName,
			updateKeyReloadProjectsPend: "1",
		})
		_ = s.setAgentUpdateTarget(info.TagName)
		return updateApplyResponse{
			Updated:        true,
			Message:        updateApplyMessage(rollback, true),
			TargetVersion:  info.TagName,
			CurrentVersion: currentVersion(),
			Staged:         true,
		}, nil
	}

	helperPath := filepath.Join(filepath.Dir(newBinPath), "ciwi-update-helper-"+strconv.FormatInt(time.Now().UnixNano(), 10)+serverExeExtFn())
	if err := serverCopyFileFn(exePath, helperPath, 0o755); err != nil {
		msg := "prepare update helper: " + err.Error()
		return updateApplyResponse{}, s.failUpdateApply(http.StatusInternalServerError, msg, msg)
	}

	if err := serverStartUpdateHelperFn(helperPath, exePath, newBinPath, os.Getpid(), os.Args[1:]); err != nil {
		msg := "start update helper: " + err.Error()
		return updateApplyResponse{}, s.failUpdateApply(http.StatusInternalServerError, msg, msg)
	}
	_ = s.persistUpdateStatus(map[string]string{
		updateKeyLastApplyStatus:    updateStatusSuccess,
		updateKeyMessage:            updateApplyMessage(rollback, false),
		updateKeyLatestVersion:      info.TagName,
		updateKeyReloadProjectsPend: "1",
	})
	_ = s.setAgentUpdateTarget(info.TagName)

	result := updateApplyResponse{
		Updated:        true,
		Message:        updateApplyMessage(rollback, false),
		TargetVersion:  info.TagName,
		CurrentVersion: currentVersion(),
	}

	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
	return result, nil
}

func (s *stateStore) failUpdateApply(statusCode int, responseMessage, persistMessage string) error {
	_ = s.persistUpdateStatus(map[string]string{
		updateKeyLastApplyStatus: updateStatusFailed,
		updateKeyMessage:         strings.TrimSpace(persistMessage),
	})
	return &updateApplyError{StatusCode: statusCode, Message: strings.TrimSpace(responseMessage)}
}

func updateApplyMessage(rollback, staged bool) string {
	if rollback {
		if staged {
			return "staged rollback and triggered linux updater"
		}
		return "rollback helper started, restarting"
	}
	if staged {
		return "staged update and triggered linux updater"
	}
	return "update helper started, restarting"
}

func (s *stateStore) updateStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.getUpdateStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *stateStore) getUpdateStatus() (updateStatusResponse, error) {
	state, err := s.updateStateStore().ListAppState()
	if err != nil {
		return updateStatusResponse{}, err
	}
	// Always expose live runtime version; persisted status can be stale across restarts.
	state["update_current_version"] = currentVersion()
	capability := detectServerUpdateCapability()
	state["update_server_mode"] = capability.Mode
	state["update_server_self_update_supported"] = boolString(capability.Supported)
	state["update_server_self_update_reason"] = capability.Reason
	state["update_agent_target_version"] = strings.TrimSpace(s.getAgentUpdateTarget())
	blockedAgents := s.listAgentsBlockedOnNonServiceSelfUpdate()
	state["update_agent_non_service_agents"] = strings.Join(blockedAgents, ",")
	return updateStatusResponse{Status: state}, nil
}

func (s *stateStore) persistUpdateStatus(values map[string]string) error {
	for k, v := range values {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if err := s.updateStateStore().SetAppState(k, v); err != nil {
			return err
		}
	}
	return nil
}

func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
