package ui

import "testing"

func TestEmbeddedUIBundle(t *testing.T) {
	screen, err := LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "front-page" {
		t.Fatalf("screen name = %q", screen.Metadata.Name)
	}
	themes, err := LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 9 {
		t.Fatalf("theme count = %d, want 9", len(themes))
	}
}
