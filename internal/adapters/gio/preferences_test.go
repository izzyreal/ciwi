//go:build darwin || linux || windows

package gio

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNativePreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "native-ui.json")
	want := nativePreferences{
		Theme: "space", Disclosures: map[string]bool{"front-project:1": false},
		ConnectionMode: connectionModeExplicit, ServerEndpoint: "tcp://127.0.0.1:8113",
	}
	if err := saveNativePreferences(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadNativePreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferences = %+v, want %+v", got, want)
	}
}

func TestMissingNativePreferencesUseDefaults(t *testing.T) {
	got, err := loadNativePreferences(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "" || len(got.Disclosures) != 0 {
		t.Fatalf("preferences = %+v", got)
	}
	mode, endpoint := got.normalizedConnection()
	if mode != connectionModeDiscover || endpoint != "" {
		t.Fatalf("normalized connection = %q, %q", mode, endpoint)
	}
}

func TestNativePreferenceUpdatesPreserveOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native-ui.json")
	if err := saveNativePreferences(path, nativePreferences{
		Theme: "default", Disclosures: map[string]bool{"front-project:1": false},
		ConnectionMode: connectionModeExplicit, ServerEndpoint: "tcp://buildbox:8113",
	}); err != nil {
		t.Fatal(err)
	}
	if err := updateNativePreferences(path, func(preferences *nativePreferences) {
		preferences.Theme = "jungle"
	}); err != nil {
		t.Fatal(err)
	}
	got, err := loadNativePreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	expanded, exists := got.Disclosures["front-project:1"]
	if got.Theme != "jungle" || !exists || expanded || got.ConnectionMode != connectionModeExplicit || got.ServerEndpoint != "tcp://buildbox:8113" {
		t.Fatalf("preferences = %+v", got)
	}
}

func TestNativePreferencesNormalizeUnknownConnectionModeToDiscovery(t *testing.T) {
	mode, endpoint := (nativePreferences{ConnectionMode: "future-mode", ServerEndpoint: "tcp://buildbox:8113"}).normalizedConnection()
	if mode != connectionModeDiscover || endpoint != "tcp://buildbox:8113" {
		t.Fatalf("normalized connection = %q, %q", mode, endpoint)
	}
}

func TestNativeConnectionSettingsForLaunch(t *testing.T) {
	preferences := nativePreferences{ConnectionMode: connectionModeExplicit, ServerEndpoint: "tcp://saved:8113"}
	saved := nativeConnectionSettingsForLaunch(preferences, "")
	if saved.Mode != connectionModeExplicit || saved.Endpoint != "tcp://saved:8113" || saved.Address != "tcp://saved:8113" {
		t.Fatalf("saved settings = %+v", saved)
	}
	overridden := nativeConnectionSettingsForLaunch(preferences, " quic://override:8113 ")
	if overridden.Mode != connectionModeExplicit || overridden.Endpoint != "quic://override:8113" || overridden.Address != "quic://override:8113" {
		t.Fatalf("overridden settings = %+v", overridden)
	}
	discovered := nativeConnectionSettingsForLaunch(nativePreferences{}, "")
	if discovered.Mode != connectionModeDiscover || discovered.Endpoint != "" || discovered.Address != "" {
		t.Fatalf("discovery settings = %+v", discovered)
	}
}
