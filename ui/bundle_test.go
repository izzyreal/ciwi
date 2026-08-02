package ui

import (
	"bytes"
	"testing"
)

func TestEmbeddedUIBundle(t *testing.T) {
	screen, err := LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "front-page" {
		t.Fatalf("screen name = %q", screen.Metadata.Name)
	}
	projectScreen, err := LoadScreen("project-details")
	if err != nil {
		t.Fatal(err)
	}
	if projectScreen.Metadata.Name != "project-details" {
		t.Fatalf("project screen name = %q", projectScreen.Metadata.Name)
	}
	jobScreen, err := LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	if jobScreen.Metadata.Name != "job-details" {
		t.Fatalf("job screen name = %q", jobScreen.Metadata.Name)
	}
	settingsScreen, err := LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	if settingsScreen.Metadata.Name != "settings" {
		t.Fatalf("settings screen name = %q", settingsScreen.Metadata.Name)
	}
	themes, err := LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 9 {
		t.Fatalf("theme count = %d, want 9", len(themes))
	}
	logo, err := Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(logo, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("embedded ciwi logo is not a PNG")
	}
}
