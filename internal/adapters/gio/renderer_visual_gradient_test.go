//go:build darwin || linux || windows

package gio

import (
	"image"
	"testing"

	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestNativePalettePreservesSharedGradientDefinitions(t *testing.T) {
	themes, err := sharedui.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) == 0 {
		t.Fatal("shared themes are empty")
	}
	theme := themes[0]
	for _, candidate := range themes {
		if candidate.Metadata.Name == "mango-kent-dark" {
			theme = candidate
			break
		}
	}
	colors, err := paletteFromTheme(theme.Theme)
	if err != nil {
		t.Fatal(err)
	}
	page := theme.Theme.Gradients["page"]
	if colors.pageGradient.kind != page.Kind || colors.pageGradient.angle != float64(page.Angle) || len(colors.pageGradient.stops) != len(page.Stops) {
		t.Fatalf("native page gradient = %#v, want kind %q, angle %d, %d stops", colors.pageGradient, page.Kind, page.Angle, len(page.Stops))
	}
	for index, stop := range page.Stops {
		want, err := parseColor(stop.Color)
		if err != nil {
			t.Fatal(err)
		}
		got := colors.pageGradient.stops[index]
		if got.color != want || got.position != float64(stop.Position)/100 {
			t.Errorf("native page stop %d = %#v, want %s at %d%%", index, got, stop.Color, stop.Position)
		}
	}

	hero := newSampledGradient(image.Pt(100, 100), colors.heroGradient)
	wantCenter, _ := parseColor(theme.Theme.Gradients["hero"].Stops[0].Color)
	wantCorner, _ := parseColor(theme.Theme.Gradients["hero"].Stops[1].Color)
	if got := hero.pixel(50, 50); got != wantCenter {
		t.Errorf("native hero center = %#v, want %#v", got, wantCenter)
	}
	if got := hero.pixel(0, 0); got != wantCorner {
		t.Errorf("native hero corner = %#v, want %#v", got, wantCorner)
	}
}
