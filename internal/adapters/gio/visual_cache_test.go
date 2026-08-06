//go:build darwin || linux || windows

package gio

import (
	"image"
	"testing"

	"gioui.org/op"
	"gioui.org/op/paint"
)

func TestVisualOpCacheReusesAndEvictsPersistentOperations(t *testing.T) {
	cache := newVisualOpCache(2)
	var frame op.Ops
	records := 0
	add := func(key visualOpKey) {
		cache.add(&frame, key, func(ops *op.Ops) {
			records++
			paint.PaintOp{}.Add(ops)
		})
	}

	first := visualOpKey{kind: "first"}
	second := visualOpKey{kind: "second"}
	third := visualOpKey{kind: "third"}
	add(first)
	firstEntry := cache.entries[first]
	frame.Reset()
	add(first)
	if records != 1 || cache.entries[first] != firstEntry {
		t.Fatalf("stable key was recorded %d times or replaced", records)
	}
	add(second)
	add(third)
	if len(cache.entries) != 2 {
		t.Fatalf("cache entries = %d, want 2", len(cache.entries))
	}
	if _, ok := cache.entries[first]; ok {
		t.Fatal("least-recently-used entry was not evicted")
	}
	cache.roundedClip(image.Pt(100, 40), 8)
	cache.reset()
	if len(cache.entries) != 0 || len(cache.paths) != 0 || cache.clock != 0 {
		t.Fatalf("reset cache = %d operations, %d paths at clock %d", len(cache.entries), len(cache.paths), cache.clock)
	}
}
