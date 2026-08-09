package giodom

import (
	"fmt"
	"testing"

	"gioui.org/layout"
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
