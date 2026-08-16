package giodom

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestKeyedStateSurvivesReorderAndRemovedStateIsSwept(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	first := dynamicColumn(1, "a", "b", "c")
	runtime.Layout(testContext(320, 240), first)
	aState := stateValueWithPath(t, runtime, "/key:a/state:selectable")
	bState := stateValueWithPath(t, runtime, "/key:b/state:selectable")

	second := dynamicColumn(2, "c", "a")
	runtime.Layout(testContext(320, 240), second)
	if got := stateValueWithPath(t, runtime, "/key:a/state:selectable"); got != aState {
		t.Fatal("state for key a changed after reordering")
	}
	for path, entry := range runtime.states {
		if entry.value == bState || strings.Contains(path, "/key:b/") {
			t.Fatalf("removed key b retained state at %q", path)
		}
	}
}

func TestStableOverlayRetainsBodyViewportAndSweepsRemovedModalState(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	rows := make([]Element, 12)
	for index := range rows {
		rows[index] = Spacer(Key(fmt.Sprintf("row-%d", index)), 0, 40)
	}
	body := VirtualList("page", ListProps{Axis: layout.Vertical, Viewport: 120, Estimate: 40}, Static(rows...))
	root := func(modal ...Element) Element {
		return Overlay("overlay-shell", OverlayProps{}, body, modal...)
	}

	runtime.Layout(testContext(320, 240), root())
	pageState := viewportState(t, runtime)
	pageState.anchor = "row-4"
	pageState.anchorIndex = 4
	pageState.anchorOffset = 7
	pageState.initialized = true

	modal := selectableTestText("modal", "Modal")
	runtime.Layout(testContext(320, 240), root(modal))
	if current := viewportState(t, runtime); current != pageState {
		t.Fatal("page viewport state changed when modal appeared")
	}
	if pageState.anchor != "row-4" || pageState.anchorIndex != 4 || pageState.anchorOffset != 7 {
		t.Fatalf("page viewport moved when modal appeared: anchor=%q index=%d offset=%d", pageState.anchor, pageState.anchorIndex, pageState.anchorOffset)
	}
	modalState := stateValueWithPath(t, runtime, "/key:modal/state:selectable")

	runtime.Layout(testContext(320, 240), root())
	if current := viewportState(t, runtime); current != pageState {
		t.Fatal("page viewport state changed when modal disappeared")
	}
	if pageState.anchor != "row-4" || pageState.anchorIndex != 4 || pageState.anchorOffset != 7 {
		t.Fatalf("page viewport moved when modal disappeared: anchor=%q index=%d offset=%d", pageState.anchor, pageState.anchorIndex, pageState.anchorOffset)
	}
	for path, entry := range runtime.states {
		if entry.value == modalState || strings.Contains(path, "/key:modal/") {
			t.Fatalf("removed modal retained state at %q", path)
		}
	}
}

func TestDynamicChildrenRejectEmptyAndDuplicateKeys(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	root := Element{
		Kind: KindFlex,
		Key:  "root",
		Flex: FlexProps{Axis: layout.Vertical},
		Children: Keyed(1,
			selectableTestText("same", "first"),
			selectableTestText("same", "second"),
			selectableTestText("", "empty"),
		),
	}
	runtime.Layout(testContext(320, 240), root)
	stats := runtime.Stats()
	if stats.Errors < 2 {
		t.Fatalf("errors = %d, want at least duplicate and empty-key errors", stats.Errors)
	}
	if !strings.Contains(stats.LastError, "empty key") {
		t.Fatalf("last error = %q, want empty-key error", stats.LastError)
	}
}

func TestKeyIdentityPreservesWhitespace(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	runtime.Layout(testContext(320, 240), dynamicColumn(1, "a", " a"))
	if stats := runtime.Stats(); stats.Errors != 0 {
		t.Fatalf("errors = %d: %s", stats.Errors, stats.LastError)
	}
	selectables := 0
	for _, entry := range runtime.states {
		if entry.kind == KindText {
			selectables++
		}
	}
	if selectables != 2 {
		t.Fatalf("text states = %d, want two distinct exact keys", selectables)
	}
}

func TestStateTableHonorsHardLimit(t *testing.T) {
	const limit = 16
	runtime := NewRuntime(nil, Options{MaxStateSlots: limit})
	elements := make([]Element, 100)
	for index := range elements {
		elements[index] = selectableTestText(Key(fmt.Sprintf("text-%d", index)), "value")
	}
	root := Element{Kind: KindFlex, Key: "root", Flex: FlexProps{Axis: layout.Vertical}, Children: Keyed(1, elements...)}
	runtime.Layout(testContext(320, 1200), root)
	if got := len(runtime.states); got > limit {
		t.Fatalf("state slots = %d, want <= %d", got, limit)
	}
}

func TestColumnGapHasNoPhantomChildren(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	root := Column("column", 10,
		Spacer("first", 10, 20),
		Spacer("second", 10, 30),
	)
	dimensions := runtime.Layout(testLooseContext(100, 100), root)
	if got, want := dimensions.Size.Y, 60; got != want {
		t.Fatalf("height = %d, want %d", got, want)
	}
}

func TestGeometryGuardRejectsOutOfRangeValues(t *testing.T) {
	runtime := NewRuntime(nil, Options{MaxGeometryPixels: 100})
	runtime.frame++
	if !runtime.rejectGeometry("test", 101) {
		t.Fatal("oversized geometry was accepted")
	}
	if got := runtime.stats.GeometryRejects; got != 1 {
		t.Fatalf("geometry rejects = %d, want 1", got)
	}
}

func TestAnimatedProgressUsesLayoutTimeAndRequestsImmediateFrame(t *testing.T) {
	runtime := NewRuntime(nil, Options{})
	router := new(input.Router)
	operations := new(op.Ops)
	now := time.Unix(1_800_000_000, 0)
	resolvedAt := time.Time{}
	progress := Progress("progress", ProgressProps{
		Mode: ProgressDeterminate, Fraction: .2, Animate: true,
		FractionAt: func(at time.Time) float32 {
			resolvedAt = at
			return .6
		},
		Color: color.NRGBA{A: 0xff}, Track: color.NRGBA{A: 0xff}, Radius: 4,
	}, Spacer("content", 100, 20))
	disabled := Element{
		Kind: KindButton, Key: "disabled-progress", Button: ButtonProps{Enabled: false},
		Children: Static(progress),
	}
	gtx := layout.Context{
		Ops: operations, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: now,
		Constraints: layout.Exact(image.Pt(100, 20)),
	}
	runtime.Layout(gtx, disabled)
	if resolvedAt != now {
		t.Fatalf("progress fraction resolved at %v, want %v", resolvedAt, now)
	}
	router.Frame(operations)
	at, wake := router.WakeupTime()
	if !wake || !at.IsZero() {
		t.Fatalf("progress wakeup = (%v, %v), want immediate", at, wake)
	}
}

func BenchmarkKeyedRuntimeReorder(b *testing.B) {
	runtime := NewRuntime(nil, Options{})
	for iteration := 0; iteration < b.N; iteration++ {
		keys := []string{"a", "b", "c", "d", "e", "f"}
		if iteration%2 != 0 {
			keys[0], keys[5] = keys[5], keys[0]
		}
		runtime.Layout(testContext(390, 844), dynamicColumn(uint64(iteration+1), keys...))
	}
}

func dynamicColumn(revision uint64, keys ...string) Element {
	elements := make([]Element, len(keys))
	for index, key := range keys {
		elements[index] = selectableTestText(Key(key), key)
	}
	return Element{Kind: KindFlex, Key: "root", Flex: FlexProps{Axis: layout.Vertical}, Children: Keyed(revision, elements...)}
}

func selectableTestText(key Key, value string) Element {
	element := Text(key, value, 14, color.NRGBA{})
	element.Text.Selectable = true
	return element
}

func stateValueWithPath(t *testing.T, runtime *Runtime, suffix string) any {
	t.Helper()
	for path, entry := range runtime.states {
		if strings.HasSuffix(path, suffix) {
			return entry.value
		}
	}
	t.Fatalf("state ending in %q not found", suffix)
	return nil
}

func testContext(width, height int) layout.Context {
	operations := new(op.Ops)
	return layout.Context{
		Ops: operations, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now: time.Unix(1_800_000_000, 0), Constraints: layout.Exact(image.Pt(width, height)),
	}
}

func testLooseContext(width, height int) layout.Context {
	gtx := testContext(width, height)
	gtx.Constraints.Min = image.Point{}
	return gtx
}
