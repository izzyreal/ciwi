package giodom

import (
	"image/color"
	"testing"
)

func TestProgressFillCompositesInSRGB(t *testing.T) {
	background := color.NRGBA{R: 20, G: 30, B: 50, A: 255}
	props := ProgressProps{Color: color.NRGBA{R: 120, G: 230, B: 150, A: 51}, CompositeBackground: &background}
	for _, test := range []struct {
		name    string
		opacity float64
		want    color.NRGBA
	}{
		{"full tint", 1, color.NRGBA{R: 40, G: 70, B: 70, A: 255}},
		{"pulse trough", .58, color.NRGBA{R: 32, G: 53, B: 62, A: 255}},
		{"transparent", 0, background},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := progressFillColor(props, test.opacity); got != test.want {
				t.Fatalf("fill = %#v, want %#v", got, test.want)
			}
		})
	}
	props.CompositeBackground = nil
	if got := progressFillColor(props, 1); got != props.Color {
		t.Fatalf("ordinary fill changed: %#v", got)
	}
	want := props.Color
	want.A = 30
	if got := progressFillColor(props, .58); got != want {
		t.Fatalf("ordinary pulse changed: %#v, want %#v", got, want)
	}
}
