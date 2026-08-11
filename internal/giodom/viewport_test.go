package giodom

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestKeyedViewportBuildsOnlyVisibleRows(t *testing.T) {
	const count = 10_000
	built := 0
	children := Lazy(1, count,
		func(index int) Key { return Key(fmt.Sprintf("row-%d", index)) },
		func(index int) Element {
			built++
			return Spacer(Key(fmt.Sprintf("row-%d", index)), 0, 40)
		},
	)
	runtime := NewRuntime(nil, Options{})
	root := VirtualList("viewport", ListProps{Axis: layout.Vertical, Estimate: 40, Overscan: 2, MaxMeasured: 64}, children)
	runtime.Layout(testContext(320, 200), root)

	if built == 0 || built > 12 {
		t.Fatalf("built rows = %d, want 1..12 for a 10,000-row source", built)
	}
	stats := runtime.Stats()
	if stats.VisibleListItems != built {
		t.Fatalf("visible rows = %d, built = %d", stats.VisibleListItems, built)
	}
	if stats.MeasuredListItems > 64 {
		t.Fatalf("measurements = %d, want <= 64", stats.MeasuredListItems)
	}
}

func TestKeyedViewportPreservesAnchorAcrossReorder(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	first := orderedRows(1, 100, 0)
	root := VirtualList("viewport", ListProps{Axis: layout.Vertical, Estimate: 40, MaxMeasured: 64}, first)
	runtime.Layout(testContext(320, 200), root)
	state := viewportState(t, runtime)
	state.anchor = "row-50"
	state.anchorIndex = 50
	state.anchorOffset = 3

	root.Children = orderedRows(2, 100, 30)
	runtime.Layout(testContext(320, 200), root)
	if state.anchor != "row-50" {
		t.Fatalf("anchor = %q, want row-50", state.anchor)
	}
	if state.anchorIndex != 20 {
		t.Fatalf("anchor index = %d, want 20 after reorder", state.anchorIndex)
	}
}

func TestViewportMeasurementCacheIsBounded(t *testing.T) {
	state := keyedViewportState{measurements: make(map[Key]measurement)}
	for index := 0; index < 1000; index++ {
		state.remember(Key(fmt.Sprintf("row-%d", index)), index, 40+index%3, 32)
	}
	if got := len(state.measurements); got != 32 {
		t.Fatalf("measurements = %d, want 32", got)
	}
}

func TestInvalidDynamicViewportDoesNotBuildAmbiguousRows(t *testing.T) {
	built := 0
	children := Lazy(1, 2,
		func(int) Key { return "duplicate" },
		func(index int) Element {
			built++
			return Spacer(Key(fmt.Sprintf("row-%d", index)), 0, 40)
		},
	)
	runtime := NewRuntime(nil, Options{})
	runtime.Layout(testContext(320, 200), VirtualList("viewport", ListProps{Estimate: 40}, children))
	if built != 0 {
		t.Fatalf("built rows = %d, want 0 after duplicate-key rejection", built)
	}
}

func TestKeyedViewportInitiallyFollowsEnd(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	root := VirtualList("viewport", ListProps{
		Axis: layout.Vertical, Estimate: 40, ScrollToEnd: true,
	}, orderedRows(1, 100, 0))
	runtime.Layout(testContext(320, 200), root)
	state := viewportState(t, runtime)
	if state.anchor != "row-95" {
		t.Fatalf("anchor = %q, want row-95 at the initial end", state.anchor)
	}
	if !state.atEnd {
		t.Fatal("viewport did not retain end-following intent")
	}
}

func TestKeyedViewportForceEndAndResetRevisions(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	children := orderedRows(1, 100, 0)
	runtime.Layout(testContext(320, 200), VirtualList("viewport", ListProps{Axis: layout.Vertical, Estimate: 40}, children))
	runtime.Layout(testContext(320, 200), VirtualList("viewport", ListProps{
		Axis: layout.Vertical, Estimate: 40, ScrollToEnd: true, ForceEndRevision: 1,
	}, children))
	if state := viewportState(t, runtime); state.anchor != "row-95" || !state.atEnd {
		t.Fatalf("forced end = anchor %q, atEnd %v", state.anchor, state.atEnd)
	}
	runtime.Layout(testContext(320, 200), VirtualList("viewport", ListProps{
		Axis: layout.Vertical, Estimate: 40, ResetRevision: 2,
	}, children))
	if state := viewportState(t, runtime); state.anchor != "row-0" || state.atEnd {
		t.Fatalf("reset = anchor %q, atEnd %v", state.anchor, state.atEnd)
	}
}

func TestNestedViewportConsumesAvailableScrollBeforeParent(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	inner := VirtualList("inner", ListProps{
		Axis: layout.Vertical, Viewport: 100, NestedScroll: true, Estimate: 40,
	}, orderedRows(1, 20, 0))
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 200}, Static(inner, Spacer("after", 0, 400)))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(10, 50), Scroll: f32.Pt(0, 40),
	}})
	innerState := viewportStateWithPath(t, runtime, "/key:inner/state:viewport")
	outerState := viewportStateWithPath(t, runtime, "/key:outer/state:viewport")
	if innerState.anchorIndex == 0 && innerState.anchorOffset == 0 {
		t.Fatal("nested viewport did not consume available scroll")
	}
	if outerState.anchorIndex != 0 || outerState.anchorOffset != 0 {
		t.Fatalf("outer viewport moved first: index %d offset %d", outerState.anchorIndex, outerState.anchorOffset)
	}
}

func TestNestedViewportWinsTouchDragInsideItsBounds(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	inner := VirtualList("inner", ListProps{
		Axis: layout.Vertical, Viewport: 100, NestedScroll: true, Estimate: 40,
	}, orderedRows(1, 20, 0))
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 200}, Static(inner, Spacer("after", 0, 400)))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 80),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 20),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 0),
	}})
	innerState := viewportStateWithPath(t, runtime, "/key:inner/state:viewport")
	outerState := viewportStateWithPath(t, runtime, "/key:outer/state:viewport")
	if innerState.anchorIndex == 0 && innerState.anchorOffset == 0 {
		t.Fatalf("nested viewport did not win the touch drag: inner=%+v outer-index=%d outer-offset=%d", innerState.scroll.State(), outerState.anchorIndex, outerState.anchorOffset)
	}
	if outerState.anchorIndex != 0 || outerState.anchorOffset != 0 {
		t.Fatalf("outer viewport moved during nested touch drag: index %d offset %d", outerState.anchorIndex, outerState.anchorOffset)
	}
}

func TestNestedViewportPassesBoundaryScrollToParent(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	inner := VirtualList("inner", ListProps{
		Axis: layout.Vertical, Viewport: 100, NestedScroll: true, Estimate: 40, ScrollToEnd: true,
	}, orderedRows(1, 20, 0))
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 200}, Static(inner, Spacer("after", 0, 400)))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(10, 50), Scroll: f32.Pt(0, 40),
	}})
	outerState := viewportStateWithPath(t, runtime, "/key:outer/state:viewport")
	if outerState.anchorIndex == 0 && outerState.anchorOffset == 0 {
		t.Fatal("outer viewport did not receive boundary scroll")
	}
}

func TestNestedViewportPassesBoundaryTouchDragToParent(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	inner := VirtualList("inner", ListProps{
		Axis: layout.Vertical, Viewport: 100, NestedScroll: true, Estimate: 40, ScrollToEnd: true,
	}, orderedRows(1, 20, 0))
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 200}, Static(inner, Spacer("after", 0, 400)))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 80),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 20),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 0),
	}})
	outerState := viewportStateWithPath(t, runtime, "/key:outer/state:viewport")
	if outerState.anchorIndex == 0 && outerState.anchorOffset == 0 {
		t.Fatal("outer viewport did not continue a touch drag at the nested end boundary")
	}
}

func TestNestedViewportPassesUpwardBoundaryTouchDragToParent(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	inner := VirtualList("inner", ListProps{
		Axis: layout.Vertical, Viewport: 100, NestedScroll: true, Estimate: 40,
	}, orderedRows(1, 20, 0))
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 200}, Static(inner, Spacer("after", 0, 400)))
	layoutInteractiveFrame(runtime, router, root, nil)
	outerState := viewportStateWithPath(t, runtime, "/key:outer/state:viewport")
	outerState.anchor, outerState.anchorIndex, outerState.anchorOffset = "after", 1, 100
	layoutInteractiveFrame(runtime, router, root, nil)
	beforeIndex, beforeOffset := outerState.anchorIndex, outerState.anchorOffset
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 20),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 80),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 100),
	}})
	if outerState.anchorIndex > beforeIndex || (outerState.anchorIndex == beforeIndex && outerState.anchorOffset >= beforeOffset) {
		t.Fatalf("outer viewport did not scroll upward from nested top boundary: before %d/%d after %d/%d", beforeIndex, beforeOffset, outerState.anchorIndex, outerState.anchorOffset)
	}
}

func TestPassThroughParentStillHandlesTouchOutsideNestedViewport(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	inner := VirtualList("inner", ListProps{
		Axis: layout.Vertical, Viewport: 100, NestedScroll: true, Estimate: 40,
	}, orderedRows(1, 20, 0))
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 200}, Static(inner, Spacer("after", 0, 400)))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 160),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 100),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 80),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 40),
	}})
	outerState := viewportStateWithPath(t, runtime, "/key:outer/state:viewport")
	if outerState.anchorIndex == 0 && outerState.anchorOffset == 0 {
		t.Fatal("pass-through parent did not handle touch outside nested viewport")
	}
}

func TestPassThroughViewportTouchDragStartsOnControl(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	rows := make([]Element, 12)
	for index := range rows {
		rows[index] = Control(Key(fmt.Sprintf("control-%d", index)), ButtonProps{Enabled: true}, Spacer("content", 0, 60))
	}
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 60}, Static(rows...))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 160),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 80),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(10, 40),
	}})
	state := viewportState(t, runtime)
	if state.anchorIndex == 0 && state.anchorOffset == 0 {
		t.Fatal("pass-through viewport did not scroll from a drag started on a control")
	}
}

func TestPassThroughViewportTouchDragStartsOnPassiveText(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	rows := make([]Element, 30)
	for index := range rows {
		label := Text(Key(fmt.Sprintf("label-%d", index)), "Queued and In Progress Job Executions", 16, color.NRGBA{})
		if label.Text.Selectable {
			t.Fatal("ordinary presentation text is selectable")
		}
		rows[index] = label
	}
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 24}, Static(rows...))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(40, 160),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(40, 80),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(40, 40),
	}})
	state := viewportState(t, runtime)
	if state.anchorIndex == 0 && state.anchorOffset == 0 {
		t.Fatal("pass-through viewport did not scroll from a drag started on passive text")
	}
}

func TestPassThroughViewportDragOnActionableTextScrollsWithoutClick(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	clicks := 0
	rows := make([]Element, 20)
	for index := range rows {
		label := Text(Key(fmt.Sprintf("label-%d", index)), fmt.Sprintf("History job %d", index), 16, color.NRGBA{})
		rows[index] = Control(Key(fmt.Sprintf("job-%d", index)), ButtonProps{
			Enabled: true, OnClick: func() { clicks++ }, MinHeight: 44,
		}, label)
	}
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 44}, Static(rows...))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(40, 160),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(40, 80),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Move, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(40, 40),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Release, Source: pointer.Touch, PointerID: 1, Position: f32.Pt(40, 40),
	}})
	state := viewportState(t, runtime)
	if state.anchorIndex == 0 && state.anchorOffset == 0 {
		t.Fatal("pass-through viewport did not scroll from actionable job text")
	}
	if clicks != 0 {
		t.Fatalf("drag over actionable job text produced %d click(s)", clicks)
	}

	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 2, Position: f32.Pt(40, 30),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Release, Source: pointer.Touch, PointerID: 2, Position: f32.Pt(40, 30),
	}})
	if clicks != 1 {
		t.Fatalf("stationary tap on actionable job text produced %d clicks, want 1", clicks)
	}
}

func TestPassThroughViewportScrollsDownAndBackUp(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	root := VirtualList("outer", ListProps{Axis: layout.Vertical, PassThroughScroll: true, Estimate: 40}, orderedRows(1, 30, 0))
	layoutInteractiveFrame(runtime, router, root, nil)
	for range 20 {
		layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
			Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(10, 100), Scroll: f32.Pt(0, 80),
		}})
	}
	state := viewportState(t, runtime)
	if state.anchorIndex == 0 && state.anchorOffset == 0 {
		t.Fatal("pass-through viewport did not scroll down")
	}
	for range 20 {
		layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
			Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(10, 100), Scroll: f32.Pt(0, -80),
		}})
	}
	if state.anchorIndex != 0 || state.anchorOffset != 0 {
		t.Fatalf("pass-through viewport did not return to top: index %d offset %d", state.anchorIndex, state.anchorOffset)
	}
}

func TestPassThroughViewportPreservesOffsetsBetweenPageCards(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	root := VirtualList("page", ListProps{
		Axis: layout.Vertical, PassThroughScroll: true, Estimate: 120, Gap: 16,
	}, Static(
		Spacer("masthead", 0, 120),
		Spacer("projects", 0, 120),
		Spacer("queued-and-in-progress", 0, 120),
		Spacer("job-execution-history", 0, 120),
		Spacer("page-end", 0, 120),
	))
	layoutInteractiveFrame(runtime, router, root, nil)
	state := viewportState(t, runtime)
	assertOffset := func(want int, wantAnchor Key) {
		t.Helper()
		got := state.prefixAt(state.anchorIndex) + state.anchorOffset
		if got != want || state.anchor != wantAnchor {
			t.Fatalf("page offset = %d at %q (%d/%d), want %d at %q", got, state.anchor, state.anchorIndex, state.anchorOffset, want, wantAnchor)
		}
	}
	scroll := func(delta, want int, wantAnchor Key) {
		t.Helper()
		layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
			Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(10, 100), Scroll: f32.Pt(0, float32(delta)),
		}})
		assertOffset(want, wantAnchor)
		// An input-free frame reconstructs the offset from the retained anchor;
		// this is where gap positions previously snapped to the next card.
		layoutInteractiveFrame(runtime, router, root, nil)
		assertOffset(want, wantAnchor)
	}

	// Each target is inside the 16 px gap immediately before the named card.
	scroll(125, 125, "masthead")
	scroll(136, 261, "projects")
	scroll(136, 397, "queued-and-in-progress")
	scroll(-136, 261, "projects")
	scroll(-136, 125, "masthead")
	scroll(-125, 0, "masthead")
}

func TestViewportReportsLeavingFollowedEnd(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	leftEnd := 0
	root := VirtualList("inner", ListProps{
		Axis: layout.Vertical, Viewport: 100, NestedScroll: true, Estimate: 40, ScrollToEnd: true,
		OnLeaveEnd: func() { leftEnd++ },
	}, orderedRows(1, 20, 0))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(10, 50), Scroll: f32.Pt(0, -40),
	}})
	if leftEnd != 1 {
		t.Fatalf("leave-end callbacks = %d, want 1", leftEnd)
	}
}

func TestKeyedViewportReportsAndSwitchesPinnedItem(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	var positions []ListViewportItem
	root := VirtualList("viewport", ListProps{
		Axis: layout.Vertical, Viewport: 200, Estimate: 400,
		PinnedOverlay: func(item ListViewportItem) *Element {
			positions = append(positions, item)
			overlay := Spacer("collapse", 80, 40)
			return &overlay
		},
		PinnedAlignment: layout.NE,
		PinnedInsets:    Insets{Top: 8, Right: 8},
	}, Static(Spacer("row-0", 0, 400), Spacer("row-1", 0, 400)))
	runtime.Layout(testContext(320, 200), root)
	if len(positions) == 0 || positions[len(positions)-1].Key != "row-0" {
		t.Fatalf("initial pinned positions = %#v, want row-0", positions)
	}
	initial := positions[len(positions)-1]
	if initial.Extent != 400 || initial.Viewport != 200 || initial.Offset != 0 {
		t.Fatalf("initial pinned geometry = %#v", initial)
	}

	state := viewportState(t, runtime)
	state.anchor, state.anchorIndex, state.anchorOffset = "row-1", 1, 60
	runtime.Layout(testContext(320, 200), root)
	current := positions[len(positions)-1]
	if current.Key != "row-1" || current.Index != 1 || current.Offset != 60 {
		t.Fatalf("switched pinned position = %#v, want row-1 offset 60", current)
	}
}

func TestPinnedViewportControlOwnsTapAboveNestedScroll(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	clicks := 0
	root := VirtualList("viewport", ListProps{
		Axis: layout.Vertical, Viewport: 200, NestedScroll: true, Estimate: 400,
		PinnedOverlay: func(ListViewportItem) *Element {
			overlay := Control("collapse", ButtonProps{Enabled: true, OnClick: func() { clicks++ }}, Spacer("label", 80, 40))
			return &overlay
		},
		PinnedAlignment: layout.NE,
		PinnedInsets:    Insets{Top: 8, Right: 8},
	}, Static(Spacer("row-0", 0, 800)))
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(280, 20),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(280, 20),
	}})
	if clicks != 1 {
		t.Fatalf("pinned control clicks = %d, want 1", clicks)
	}
	state := viewportState(t, runtime)
	if state.anchorIndex != 0 || state.anchorOffset != 0 {
		t.Fatalf("nested viewport moved during pinned tap: index %d offset %d", state.anchorIndex, state.anchorOffset)
	}
}

func TestNestedViewportChildControlReceivesMouseAndTouchTaps(t *testing.T) {
	for _, source := range []pointer.Source{pointer.Mouse, pointer.Touch} {
		t.Run(source.String(), func(t *testing.T) {
			runtime := NewRuntime(nil, Options{})
			router := new(input.Router)
			clicks := 0
			row := Control("row", ButtonProps{Enabled: true, OnClick: func() { clicks++ }}, Spacer("label", 300, 60))
			root := VirtualList("viewport", ListProps{
				Axis: layout.Vertical, Viewport: 200, NestedScroll: true, Estimate: 60,
			}, Static(row))
			layoutInteractiveFrame(runtime, router, root, nil)
			press := pointer.Event{
				Kind: pointer.Press, Source: source, PointerID: 1, Position: f32.Pt(280, 30),
			}
			if source == pointer.Mouse {
				press.Buttons = pointer.ButtonPrimary
			}
			layoutInteractiveFrame(runtime, router, root, []pointer.Event{press})
			layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
				Kind: pointer.Release, Source: source, PointerID: 1, Position: f32.Pt(280, 30),
			}})
			if clicks != 1 {
				t.Fatalf("nested row clicks = %d, want 1", clicks)
			}
		})
	}
}

func TestNestedControlOwnsTapWithoutActivatingRow(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	rowClicks, actionClicks := 0, 0
	action := Control("action", ButtonProps{Enabled: true, OnClick: func() { actionClicks++ }}, Spacer("action-label", 80, 40))
	content := Row("content", 0, Spacer("before-action", 100, 40), action)
	root := Control("row", ButtonProps{Enabled: true, OnClick: func() { rowClicks++ }}, content)
	layoutInteractiveFrame(runtime, router, root, nil)
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(140, 20),
	}})
	layoutInteractiveFrame(runtime, router, root, []pointer.Event{{
		Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(140, 20),
	}})
	if actionClicks != 1 || rowClicks != 0 {
		t.Fatalf("nested tap = row %d action %d, want row 0 action 1", rowClicks, actionClicks)
	}
}

func TestUnchangedViewportDoesNotRescanCollection(t *testing.T) {
	children := &countingChildren{keys: makeRowKeys(10_000), revision: 1}
	runtime := NewRuntime(nil, Options{})
	root := VirtualList("viewport", ListProps{
		Axis: layout.Vertical, Estimate: 40, Overscan: 4, MaxMeasured: 64,
	}, children)
	runtime.Layout(testContext(320, 200), root)
	children.keyCalls = 0
	runtime.Layout(testContext(320, 200), root)
	if children.keyCalls > 32 {
		t.Fatalf("unchanged frame requested %d keys, want work bounded to the visible window", children.keyCalls)
	}
}

func BenchmarkKeyedViewportTenThousandRowChurn(b *testing.B) {
	runtime := NewRuntime(nil, Options{})
	keys := makeRowKeys(10_000)
	for iteration := 0; iteration < b.N; iteration++ {
		children := orderedRowsWithKeys(uint64(iteration+1), keys, iteration%len(keys))
		root := VirtualList("viewport", ListProps{
			Axis: layout.Vertical, Estimate: 40, Overscan: 4, MaxMeasured: 2048,
		}, children)
		runtime.Layout(testContext(390, 844), root)
	}
}

func BenchmarkKeyedViewportTenThousandRowsSteady(b *testing.B) {
	runtime := NewRuntime(nil, Options{})
	keys := makeRowKeys(10_000)
	root := VirtualList("viewport", ListProps{
		Axis: layout.Vertical, Estimate: 40, Overscan: 4, MaxMeasured: 2048,
	}, orderedRowsWithKeys(1, keys, 0))
	runtime.Layout(testContext(390, 844), root)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		runtime.Layout(testContext(390, 844), root)
	}
}

func orderedRows(revision uint64, count, shift int) Children {
	return orderedRowsWithKeys(revision, makeRowKeys(count), shift)
}

func orderedRowsWithKeys(revision uint64, keys []Key, shift int) Children {
	count := len(keys)
	return Lazy(revision, count,
		func(index int) Key { return keys[(index+shift)%count] },
		func(index int) Element {
			id := (index + shift) % count
			return Spacer(keys[id], 0, 40)
		},
	)
}

func makeRowKeys(count int) []Key {
	keys := make([]Key, count)
	for index := range keys {
		keys[index] = Key(fmt.Sprintf("row-%d", index))
	}
	return keys
}

type countingChildren struct {
	keys       []Key
	revision   uint64
	keyCalls   int
	buildCalls int
}

func (c *countingChildren) Len() int         { return len(c.keys) }
func (c *countingChildren) Dynamic() bool    { return true }
func (c *countingChildren) Revision() uint64 { return c.revision }
func (c *countingChildren) KeyAt(index int) Key {
	c.keyCalls++
	return c.keys[index]
}
func (c *countingChildren) At(index int) Element {
	c.buildCalls++
	return Spacer(c.keys[index], 0, 40)
}

func viewportState(t *testing.T, runtime *Runtime) *keyedViewportState {
	t.Helper()
	for _, entry := range runtime.states {
		if state, ok := entry.value.(*keyedViewportState); ok {
			return state
		}
	}
	t.Fatal("keyed viewport state not found")
	return nil
}

func viewportStateWithPath(t *testing.T, runtime *Runtime, suffix string) *keyedViewportState {
	t.Helper()
	for path, entry := range runtime.states {
		if strings.HasSuffix(path, suffix) {
			return entry.value.(*keyedViewportState)
		}
	}
	t.Fatalf("keyed viewport state ending in %q not found", suffix)
	return nil
}

func layoutInteractiveFrame(runtime *Runtime, router *input.Router, root Element, events []pointer.Event) {
	if len(events) > 0 {
		for _, event := range events {
			router.Queue(event)
		}
	}
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now: time.Unix(1_800_000_000, 0), Constraints: layout.Exact(image.Pt(320, 200)),
	}
	runtime.Layout(gtx, root)
	router.Frame(operations)
}
