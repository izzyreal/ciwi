//go:build darwin || ios || linux || windows

package gio

import (
	"testing"
	"time"

	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestNativeJobPropertyDurationResolvesAtFrameTime(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	node := uidsl.Node{Component: "text", Text: &uidsl.Text{Binding: "detailRow.value"}, Style: uidsl.Style{Role: "detail"}}
	data := map[string]any{
		"detailRow": map[string]any{"label": "Duration", "value": "", "live_duration_started_unix_ms": int64(1_000)},
		"jobDetails": map[string]any{
			"progress":                         map[string]any{"snapshot_unix_ms": int64(6_900)},
			"duration_client_snapshot_unix_ms": int64(20_000),
		},
	}
	element := renderer.compileDOMText(node, data, "duration")
	if element.Text.ValueAt == nil || !element.Text.Animate {
		t.Fatalf("live duration text = %#v", element.Text)
	}
	if got := element.Text.ValueAt(time.UnixMilli(20_999)); got != "6s" {
		t.Fatalf("frame-time duration = %q, want 6s", got)
	}
	labelNode := uidsl.Node{Component: "text", Text: &uidsl.Text{Binding: "detailRow.label"}, Style: uidsl.Style{Role: "detail"}}
	if label := renderer.compileDOMText(labelNode, data, "duration-label"); label.Text.ValueAt != nil || label.Text.Value != "Duration" {
		t.Fatalf("duration label unexpectedly became dynamic: %#v", label.Text)
	}
}
