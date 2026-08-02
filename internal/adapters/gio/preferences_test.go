//go:build darwin

package gio

import (
	"path/filepath"
	"testing"
)

func TestNativePreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "native-ui.json")
	want := nativePreferences{Theme: "space"}
	if err := saveNativePreferences(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadNativePreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("preferences = %+v, want %+v", got, want)
	}
}

func TestMissingNativePreferencesUseDefaults(t *testing.T) {
	got, err := loadNativePreferences(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != (nativePreferences{}) {
		t.Fatalf("preferences = %+v", got)
	}
}
