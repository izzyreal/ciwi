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
	runOptionsScreen, err := LoadScreen("run-options")
	if err != nil {
		t.Fatal(err)
	}
	if runOptionsScreen.Metadata.Name != "run-options" {
		t.Fatalf("run options screen name = %q", runOptionsScreen.Metadata.Name)
	}
	for _, name := range []string{"agents", "connection"} {
		screen, err := LoadScreen(name)
		if err != nil {
			t.Fatal(err)
		}
		if screen.Metadata.Name != name {
			t.Fatalf("%s screen name = %q", name, screen.Metadata.Name)
		}
	}
	themes, err := LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 9 {
		t.Fatalf("theme count = %d, want 9", len(themes))
	}
	for _, theme := range themes {
		for _, token := range []string{
			"background-start", "background-end", "background-glow-a", "background-glow-b",
			"surface-raised", "surface-glow", "pill-background", "pill-text",
			"console-background", "console-surface", "console-border", "console-text", "console-muted", "console-accent",
		} {
			if theme.Theme.Colors[token] == "" {
				t.Errorf("theme %q is missing shared visual color %q", theme.Metadata.Name, token)
			}
		}
		for _, token := range []string{
			"small", "medium", "large", "page", "page-inset", "section-padding", "card-padding", "hero-padding",
			"surface-radius", "control-radius", "control-padding-x", "control-padding-y",
			"text-body", "text-control", "text-code", "text-badge", "text-subtitle", "text-heading", "text-title",
			"image-brand-width", "image-brand-height",
		} {
			if theme.Theme.Dimensions[token] == "" {
				t.Errorf("theme %q is missing shared visual metric %q", theme.Metadata.Name, token)
			}
		}
	}
	logo, err := Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(logo, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("embedded ciwi logo is not a PNG")
	}
}
