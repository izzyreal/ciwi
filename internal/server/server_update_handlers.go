package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *stateStore) updateCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info, err := s.fetchLatestUpdateInfo(r.Context())
	if err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_checked_utc": time.Now().UTC().Format(time.RFC3339Nano),
			"update_current_version":  currentVersion(),
			"update_message":          err.Error(),
			"update_available":        "0",
		})
		writeJSON(w, http.StatusOK, updateCheckResponse{
			CurrentVersion: currentVersion(),
			Message:        err.Error(),
		})
		return
	}

	resp := updateCheckResponse{
		CurrentVersion:  currentVersion(),
		LatestVersion:   info.TagName,
		UpdateAvailable: isVersionNewer(info.TagName, currentVersion()),
		ReleaseURL:      info.HTMLURL,
		AssetName:       info.Asset.Name,
	}
	if !resp.UpdateAvailable {
		resp.Message = "already up to date"
	}
	_ = s.persistUpdateStatus(map[string]string{
		"update_last_checked_utc": time.Now().UTC().Format(time.RFC3339Nano),
		"update_current_version":  currentVersion(),
		"update_latest_version":   info.TagName,
		"update_release_url":      info.HTMLURL,
		"update_asset_name":       info.Asset.Name,
		"update_available":        boolString(resp.UpdateAvailable),
		"update_message":          resp.Message,
	})
	writeJSON(w, http.StatusOK, resp)
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
	tags, err := s.fetchUpdateTags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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
	writeJSON(w, http.StatusOK, updateTagsResponse{
		Tags:           tags,
		CurrentVersion: current,
	})
}

func (s *stateStore) applyUpdateTargetHandler(w http.ResponseWriter, r *http.Request, targetVersion string, rollback bool) {
	s.update.mu.Lock()
	if s.update.inProgress {
		s.update.mu.Unlock()
		http.Error(w, "update already in progress", http.StatusConflict)
		return
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
		"update_last_apply_utc":    time.Now().UTC().Format(time.RFC3339Nano),
		"update_last_apply_status": "running",
		"update_message":           "update started",
	})

	exePath, err := os.Executable()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve executable path: %v", err), http.StatusInternalServerError)
		return
	}
	exePath, _ = filepath.Abs(exePath)
	if looksLikeGoRunBinary(exePath) {
		msg := "self-update is unavailable for go run binaries; run built ciwi binary instead"
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           msg,
		})
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	info, err := s.fetchUpdateInfoForTag(r.Context(), targetVersion)
	if err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           err.Error(),
		})
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !isVersionDifferent(info.TagName, currentVersion()) {
		msg := "already at target version"
		if !rollback {
			msg = "already up to date"
		}
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "noop",
			"update_message":           msg,
			"update_latest_version":    info.TagName,
		})
		_ = s.setAgentUpdateTarget(currentVersion())
		writeJSON(w, http.StatusOK, updateApplyResponse{
			Updated: false,
			Message: msg,
			Target:  info.TagName,
		})
		return
	}

	newBinPath, err := downloadUpdateAsset(r.Context(), info.Asset.URL, info.Asset.Name)
	if err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           "download update asset: " + err.Error(),
		})
		http.Error(w, fmt.Sprintf("download update asset: %v", err), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(info.ChecksumAsset.URL) != "" {
		checksumText, err := downloadTextAsset(r.Context(), info.ChecksumAsset.URL)
		if err != nil {
			_ = s.persistUpdateStatus(map[string]string{
				"update_last_apply_status": "failed",
				"update_message":           "download checksum asset: " + err.Error(),
			})
			http.Error(w, fmt.Sprintf("download checksum asset: %v", err), http.StatusBadRequest)
			return
		}
		if err := verifyFileSHA256(newBinPath, info.Asset.Name, checksumText); err != nil {
			_ = s.persistUpdateStatus(map[string]string{
				"update_last_apply_status": "failed",
				"update_message":           "checksum verification failed: " + err.Error(),
			})
			http.Error(w, fmt.Sprintf("checksum verification failed: %v", err), http.StatusBadRequest)
			return
		}
	}

	if isLinuxSystemUpdaterEnabled() {
		if err := stageLinuxUpdateBinary(info.TagName, info, newBinPath); err != nil {
			_ = s.persistUpdateStatus(map[string]string{
				"update_last_apply_status": "failed",
				"update_message":           "stage update: " + err.Error(),
			})
			http.Error(w, fmt.Sprintf("stage update: %v", err), http.StatusInternalServerError)
			return
		}
		if err := triggerLinuxSystemUpdater(); err != nil {
			_ = s.persistUpdateStatus(map[string]string{
				"update_last_apply_status": "failed",
				"update_message":           "trigger updater: " + err.Error(),
			})
			http.Error(w, fmt.Sprintf("trigger updater: %v", err), http.StatusInternalServerError)
			return
		}
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "staged",
			"update_message":           updateApplyMessage(rollback, true),
			"update_latest_version":    info.TagName,
		})
		_ = s.setAgentUpdateTarget(info.TagName)
		writeJSON(w, http.StatusOK, updateApplyResponse{
			Updated:        true,
			Message:        updateApplyMessage(rollback, true),
			TargetVersion:  info.TagName,
			CurrentVersion: currentVersion(),
			Staged:         true,
		})
		return
	}

	helperPath := filepath.Join(filepath.Dir(newBinPath), "ciwi-update-helper-"+strconv.FormatInt(time.Now().UnixNano(), 10)+exeExt())
	if err := copyFile(exePath, helperPath, 0o755); err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           "prepare update helper: " + err.Error(),
		})
		http.Error(w, fmt.Sprintf("prepare update helper: %v", err), http.StatusInternalServerError)
		return
	}

	if err := startUpdateHelper(helperPath, exePath, newBinPath, os.Getpid(), os.Args[1:]); err != nil {
		_ = s.persistUpdateStatus(map[string]string{
			"update_last_apply_status": "failed",
			"update_message":           "start update helper: " + err.Error(),
		})
		http.Error(w, fmt.Sprintf("start update helper: %v", err), http.StatusInternalServerError)
		return
	}
	_ = s.persistUpdateStatus(map[string]string{
		"update_last_apply_status": "success",
		"update_message":           updateApplyMessage(rollback, false),
		"update_latest_version":    info.TagName,
	})
	_ = s.setAgentUpdateTarget(info.TagName)

	writeJSON(w, http.StatusOK, updateApplyResponse{
		Updated:        true,
		Message:        updateApplyMessage(rollback, false),
		TargetVersion:  info.TagName,
		CurrentVersion: currentVersion(),
	})

	go func() {
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}()
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
	state, err := s.db.ListAppState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Always expose live runtime version; persisted status can be stale across restarts.
	state["update_current_version"] = currentVersion()
	writeJSON(w, http.StatusOK, updateStatusResponse{Status: state})
}

func (s *stateStore) persistUpdateStatus(values map[string]string) error {
	for k, v := range values {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if err := s.db.SetAppState(k, v); err != nil {
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
