//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestTimelineSelectionDoesNotCreateTransientNotice(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	data := map[string]any{"jobDetails": map[string]any{
		"timeline": []any{map[string]any{"id": "phase-2", "title": "Ciwi phase 2/4: Check out source"}},
		"output_groups": []any{map[string]any{
			"id": "phase-2", "state_key": "job-output:phase-2",
		}},
	}}
	renderer.SetData(data)
	if !renderer.dispatchRendererAction(nil, "select-timeline-item", map[string]string{"id": "phase-2"}, data) {
		t.Fatal("timeline selection was not handled")
	}
	if renderer.notice != nil || len(renderer.noticeQueue) != 0 {
		t.Fatalf("timeline selection created notice state: active=%#v queued=%d", renderer.notice, len(renderer.noticeQueue))
	}
	if !renderer.disclosures["job-output:phase-2"] || renderer.pendingOutputScroll != "phase-2" {
		t.Fatalf("timeline selection did not retain expansion/scroll behavior: disclosures=%#v scroll=%q", renderer.disclosures, renderer.pendingOutputScroll)
	}
	root := renderer.data.(map[string]any)["jobDetails"].(map[string]any)
	selected := root["selected_timeline_item"].(map[string]any)
	if selected["id"] != "phase-2" {
		t.Fatalf("selected timeline item = %#v", selected)
	}
}

func TestTransientNoticeExpiresDuringRendererFrame(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.ShowNotice("temporary", "", uidsl.Action{}, nil, time.Second)
	renderer.mu.RLock()
	expires := renderer.notice.expires
	renderer.mu.RUnlock()
	renderer.layoutFrame(layout.Context{
		Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: expires.Add(time.Millisecond),
		Constraints: layout.Constraints{Max: image.Pt(320, 480)},
	})
	renderer.mu.RLock()
	defer renderer.mu.RUnlock()
	if renderer.notice != nil {
		t.Fatalf("expired notice remains active: %#v", renderer.notice)
	}
}
