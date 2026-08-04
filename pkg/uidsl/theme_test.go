package uidsl

import (
	"strings"
	"testing"
)

const validTheme = `apiVersion: ciwi.ui/v1
kind: Theme
metadata:
  name: default
  title: Default
theme:
  colors:
    background: "#eef8f3"
    surface: "#ffffff"
    surface-subtle: "#f5fbf8"
    text: "#18231e"
    text-muted: "#607068"
    accent: "#2f7f67"
    accent-strong: "#1c5f4b"
    border: "#b8d8ca"
    success: "#2d8a62"
    warning: "#b97818"
    danger: "#b84747"
    focus: "#2567d8"
  gradients:
    page:
      kind: linear
      angle: 135
      stops:
        - color: "#eef8f3"
          position: 0
        - color: "#dcefe7"
          position: 100
`

func TestParseTheme(t *testing.T) {
	theme, err := ParseTheme([]byte(validTheme))
	if err != nil {
		t.Fatal(err)
	}
	if theme.Metadata.Name != "default" || theme.Theme.Colors["accent"] != "#2f7f67" {
		t.Fatalf("theme = %#v", theme)
	}
}

func TestParseThemeRequiresSemanticTokens(t *testing.T) {
	payload := strings.Replace(validTheme, "    danger: \"#b84747\"\n", "", 1)
	_, err := ParseTheme([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "danger") {
		t.Fatalf("ParseTheme() error = %v", err)
	}
}

func TestParseThemeRejectsInvalidDimension(t *testing.T) {
	payload := strings.Replace(validTheme, "  gradients:\n", "  dimensions: {text-body: nope}\n  gradients:\n", 1)
	_, err := ParseTheme([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "text-body") {
		t.Fatalf("ParseTheme() error = %v", err)
	}
}
