//go:build darwin || ios || linux || windows

package gio

import (
	"math"
	"reflect"
	"testing"
	"time"

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

func TestNativeAnimatedProgressIgnoresSnapshotRefreshes(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	node := uidsl.Node{Component: "card", Progress: &uidsl.Progress{Binding: "item.progress"}}

	for _, state := range []string{"indeterminate", "overrun"} {
		t.Run(state, func(t *testing.T) {
			propsAt := func(snapshot int64) giodom.ProgressProps {
				t.Helper()
				data := map[string]any{"item": map[string]any{"progress": map[string]any{
					"state": state, "fraction": 1, "snapshot_unix_ms": snapshot,
				}}}
				props := renderer.domProgressProps(node, data)
				if props == nil {
					t.Fatal("progress props are nil")
				}
				return *props
			}
			first := propsAt(1_000)
			refreshed := propsAt(9_000)
			if !reflect.DeepEqual(first, refreshed) {
				t.Fatalf("snapshot refresh changed animated progress props: first=%+v refreshed=%+v", first, refreshed)
			}
		})
	}
}

func TestNativeDeterminateProgressIsContinuousAcrossEquivalentSnapshots(t *testing.T) {
	now := time.UnixMilli(4_500)
	before := semanticProgress{state: "determinate", fraction: .2, snapshotUnixMS: 1_000, ratePerMS: .0001}
	refreshed := semanticProgress{state: "determinate", fraction: .5, snapshotUnixMS: 4_000, ratePerMS: .0001}

	beforeState, beforeFraction := evaluateSemanticProgress(before, now)
	refreshedState, refreshedFraction := evaluateSemanticProgress(refreshed, now)
	if beforeState != "determinate" || refreshedState != "determinate" {
		t.Fatalf("states = %q and %q", beforeState, refreshedState)
	}
	if math.Abs(beforeFraction-refreshedFraction) > .000001 {
		t.Fatalf("refresh changed visible fraction from %g to %g", beforeFraction, refreshedFraction)
	}
}

func TestNativeDeterminateProgressResolvesFractionAtLayoutTime(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	node := uidsl.Node{Component: "card", Progress: &uidsl.Progress{Binding: "item.progress"}}
	data := map[string]any{"item": map[string]any{"progress": map[string]any{
		"state": "determinate", "fraction": .2, "snapshot_unix_ms": 1_000, "rate_per_ms": .0001,
	}}}
	props := renderer.domProgressProps(node, data)
	if props == nil || props.FractionAt == nil || !props.Animate {
		t.Fatalf("dynamic progress props = %#v", props)
	}
	if got, want := props.FractionAt(time.UnixMilli(4_500)), float32(.55); math.Abs(float64(got-want)) > .000001 {
		t.Fatalf("layout-time fraction = %g, want %g", got, want)
	}
}
