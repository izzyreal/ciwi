//go:build darwin || ios || linux || windows

package gio

import (
	"testing"
	"time"

	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestDownloadsBindingDataSatisfiesScreenAndExposesStateActions(t *testing.T) {
	states := []nativeDownloadState{
		downloadPreparing, downloadDownloading, downloadPaused, downloadReadyToSave,
		downloadSaving, downloadCompleted, downloadFailed, downloadSourceChanged,
	}
	snapshot := make([]nativeDownloadSnapshot, 0, len(states))
	for index, state := range states {
		snapshot = append(snapshot, nativeDownloadSnapshot{
			ID: string(rune('a' + index)), Label: "Artifact", FileName: "artifact.bin", State: string(state),
			Downloaded: 40, Total: 80, UpdatedAt: time.Unix(int64(index+1), 0), CanReveal: state == downloadCompleted,
		})
	}
	data := downloadsBindingData(snapshot)
	screen, err := sharedui.LoadScreen("downloads")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNativeBindings(screen, data); err != nil {
		t.Fatal(err)
	}
	root := data["downloads"].(map[string]any)
	items := root["items"].([]any)
	byState := map[string]map[string]any{}
	for _, raw := range items {
		item := raw.(map[string]any)
		byState[item["state"].(string)] = item
	}
	assertVisible := func(state nativeDownloadState, field string) {
		t.Helper()
		if value, _ := byState[string(state)][field].(bool); !value {
			t.Errorf("%s %s = %v, want true", state, field, byState[string(state)][field])
		}
	}
	assertVisible(downloadDownloading, "show_pause")
	assertVisible(downloadPaused, "show_resume")
	assertVisible(downloadFailed, "show_retry")
	assertVisible(downloadReadyToSave, "show_save")
	assertVisible(downloadSourceChanged, "show_restart")
	assertVisible(downloadCompleted, "show_remove")
	assertVisible(downloadCompleted, "show_reveal")
	for _, state := range states {
		item := byState[string(state)]
		wantDiscard := state != downloadCompleted
		if got, _ := item["show_discard"].(bool); got != wantDiscard {
			t.Errorf("%s show_discard = %t, want %t", state, got, wantDiscard)
		}
	}
}

func TestDownloadsBindingDataMarksMissingCompletedFile(t *testing.T) {
	data := downloadsBindingData([]nativeDownloadSnapshot{{
		ID: "missing", FileName: "gone.zip", State: string(downloadCompleted), Total: 80,
		DestinationMissing: true,
	}})
	item := data["downloads"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["detail"] != "Downloaded file is no longer available" || item["show_reveal"] != false || item["show_remove"] != true {
		t.Fatalf("missing completed binding = %#v", item)
	}
}
