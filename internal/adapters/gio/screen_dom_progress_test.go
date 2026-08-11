//go:build darwin || ios || linux || windows

package gio

import (
	"math"
	"testing"

	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestNativeProgressUsesTranslucentSharedTintAndSemanticMode(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	node := uidsl.Node{Component: "card", Progress: &uidsl.Progress{Binding: "item.progress"}}
	wantColor := renderer.palette.success
	wantColor.A = uint8(math.Round(0xff * float64(renderer.controls.Progress.TintOpacity)))

	for _, test := range []struct {
		state string
		mode  giodom.ProgressMode
	}{
		{state: "overrun", mode: giodom.ProgressOverrun},
		{state: "complete", mode: giodom.ProgressComplete},
	} {
		t.Run(test.state, func(t *testing.T) {
			data := map[string]any{"item": map[string]any{"progress": map[string]any{
				"state": test.state, "fraction": 1,
			}}}
			props := renderer.domProgressProps(node, data)
			if props == nil {
				t.Fatal("progress props are nil")
			}
			if props.Mode != test.mode {
				t.Fatalf("mode = %v, want %v", props.Mode, test.mode)
			}
			if props.Color != wantColor {
				t.Fatalf("color = %#v, want translucent shared tint %#v", props.Color, wantColor)
			}
		})
	}
}
