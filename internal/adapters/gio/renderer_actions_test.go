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

func TestInteractiveOutputSearchDebouncesTypingAndCancelsShortQuery(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	data := map[string]any{"jobDetails": map[string]any{
		"id": "job-1", "interactive_log_available": true,
	}}
	renderer.SetData(data)
	type dispatched struct {
		command   string
		arguments map[string]string
	}
	var actions []dispatched
	renderer.onAction = func(action uidsl.Action, arguments map[string]string) {
		actions = append(actions, dispatched{command: action.Command, arguments: cloneStringMap(arguments)})
	}
	renderer.dispatchRendererAction(nil, "change-output-search", map[string]string{"query": "needle"}, data)
	if len(actions) != 1 || actions[0].command != "search-job-log" || actions[0].arguments["debounce"] != "true" {
		t.Fatalf("typed search actions = %#v, want one debounced search", actions)
	}
	renderer.dispatchRendererAction(nil, "change-output-search", map[string]string{"query": "ne"}, data)
	if len(actions) != 2 || actions[1].command != "cancel-job-log-search" {
		t.Fatalf("shortened search actions = %#v, want cancellation", actions)
	}
}

func TestInteractiveSearchResultStopsTailingAndRevealsMatchedGroup(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.outputTailing = true
	data := map[string]any{"jobDetails": map[string]any{
		"id": "job-1", "tailing_label": "Tailing: On", "tailing_tone": "success",
		"output_search_count": "0/0",
		"output_groups": []any{map[string]any{
			"id": "step:1", "state_key": "job-output:job-1:step:1",
		}},
	}}
	renderer.SetData(data)
	renderer.jobLogStreams[nativeJobLogKey("job-1", "step:1")] = jobLogStreamSnapshot{
		JobID: "job-1", ItemID: "step:1", PageLoaded: true,
		Chunks: []jobLogChunkSnapshot{{ID: 7, Text: "prefix needle suffix"}},
	}
	renderer.ApplyJobLogSearch(jobLogSearchSnapshot{
		JobID: "job-1", ItemID: "step:1", Query: "needle", ChunkID: 7,
		StartRune: 7, EndRune: 13, SelectedIndex: 0, TotalMatches: 1,
	})
	stream := renderer.jobLogStreams[nativeJobLogKey("job-1", "step:1")]
	if renderer.outputTailing || renderer.pendingOutputScroll != "step:1" ||
		!renderer.disclosures["job-output:job-1:step:1"] {
		t.Fatalf("search navigation = tailing %v scroll %q disclosures %#v",
			renderer.outputTailing, renderer.pendingOutputScroll, renderer.disclosures)
	}
	if stream.SelectedChunkID != 7 || stream.SelectedStartRune != 7 || stream.SelectedEndRune != 13 {
		t.Fatalf("selected log match = %+v", stream)
	}
	root := renderer.data.(map[string]any)["jobDetails"].(map[string]any)
	if root["tailing_label"] != "Tailing: Off" || root["output_search_count"] != "1/1" {
		t.Fatalf("search bindings = %#v", root)
	}
}

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
