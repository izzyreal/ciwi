//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"runtime"
	"strings"
)

func downloadsBindingData(snapshot []nativeDownloadSnapshot) map[string]any {
	items := make([]any, 0, len(snapshot))
	unfinished := 0
	for _, download := range snapshot {
		if download.State != string(downloadCompleted) {
			unfinished++
		}
		items = append(items, downloadBindingItem(download))
	}
	summary := "No downloads"
	if len(snapshot) == 1 {
		summary = "1 download"
	} else if len(snapshot) > 1 {
		summary = fmt.Sprintf("%d downloads", len(snapshot))
	}
	if unfinished > 0 {
		summary += fmt.Sprintf(" · %d unfinished", unfinished)
	}
	return map[string]any{"downloads": map[string]any{
		"items": items, "empty": len(items) == 0, "summary": summary,
	}}
}

func downloadBindingItem(download nativeDownloadSnapshot) map[string]any {
	state := nativeDownloadState(download.State)
	name := strings.TrimSpace(download.FileName)
	if name == "" {
		name = strings.TrimSpace(download.Label)
	}
	if name == "" {
		name = "Preparing download…"
	}
	status := map[nativeDownloadState]string{
		downloadPreparing: "Preparing", downloadDownloading: "Downloading", downloadPaused: "Paused",
		downloadReadyToSave: "Ready to save", downloadSaving: "Saving", downloadCompleted: "Completed",
		downloadFailed: "Failed", downloadSourceChanged: "Source changed",
	}[state]
	if status == "" {
		status = strings.ReplaceAll(download.State, "-", " ")
	}
	detail := ""
	if download.Total > 0 {
		percentage := float64(download.Downloaded) / float64(download.Total) * 100
		detail = fmt.Sprintf("%s / %s · %.0f%%", formatDownloadBytes(download.Downloaded), formatDownloadBytes(download.Total), percentage)
	}
	if state == downloadCompleted && download.Total > 0 {
		detail = formatDownloadBytes(download.Total)
	}
	if download.DestinationMissing {
		detail = "Downloaded file is no longer available"
	}
	progressState, fraction := "none", float64(0)
	if state == downloadCompleted || state == downloadReadyToSave {
		progressState, fraction = "complete", 1
	} else if state == downloadPreparing || state == downloadSaving || (state == downloadDownloading && download.Total <= 0) {
		progressState = "indeterminate"
	} else if download.Total > 0 {
		progressState = "determinate"
		fraction = min(1, max(0, float64(download.Downloaded)/float64(download.Total)))
	}
	return map[string]any{
		"id": download.ID, "name": name, "label": download.Label, "state": download.State,
		"status": status, "status_tone": downloadTone(download.State), "detail": detail, "show_detail": detail != "",
		"error": download.Error, "show_error": strings.TrimSpace(download.Error) != "",
		"progress":     map[string]any{"state": progressState, "fraction": fraction},
		"show_pause":   state == downloadDownloading,
		"show_resume":  state == downloadPaused,
		"show_retry":   state == downloadFailed,
		"show_save":    state == downloadReadyToSave,
		"show_restart": state == downloadSourceChanged,
		"show_discard": state != downloadCompleted,
		"show_remove":  state == downloadCompleted,
		"show_reveal":  download.CanReveal,
		"reveal_label": downloadRevealLabel(),
	}
}

func downloadRevealLabel() string {
	switch runtime.GOOS {
	case "darwin":
		return "Show in Finder"
	case "windows":
		return "Show in Explorer"
	default:
		return "Show in folder"
	}
}

func formatDownloadBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	size := float64(value)
	unit := "B"
	for _, candidate := range units {
		size /= 1024
		unit = candidate
		if size < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", size, unit)
}

func downloadTone(state string) string {
	switch nativeDownloadState(state) {
	case downloadCompleted, downloadReadyToSave:
		return "success"
	case downloadFailed, downloadSourceChanged:
		return "danger"
	case downloadPaused:
		return "warning"
	default:
		return "muted"
	}
}
