//go:build darwin || linux || windows

package gio

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNativePreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "native-ui.json")
	want := nativePreferences{Theme: "space", Disclosures: map[string]bool{"front-project:1": false}}
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
}

func TestNativePreferenceUpdatesPreserveOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native-ui.json")
	if err := saveNativePreferences(path, nativePreferences{Theme: "default", Disclosures: map[string]bool{"front-project:1": false}}); err != nil {
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
	if got.Theme != "jungle" || !exists || expanded {
		t.Fatalf("preferences = %+v", got)
	}
}
