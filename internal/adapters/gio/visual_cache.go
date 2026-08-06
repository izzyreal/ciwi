//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

const maxVisualOpCacheEntries = 256

type visualOpKey struct {
	kind    string
	variant string
	size    image.Point
	radius  int
	width   int
	color1  color.NRGBA
	color2  color.NRGBA
}

type visualOpEntry struct {
	ops      op.Ops
	call     op.CallOp
	lastUsed uint64
}

type roundedPathKey struct {
	size   image.Point
	radius int
}

type visualPathEntry struct {
	ops      op.Ops
	path     clip.PathSpec
	lastUsed uint64
}

// visualOpCache keeps immutable drawing operations in Ops instances that are
// never reset. Gio includes an Ops version in its GPU path-cache key, so paths
// recorded directly into the per-frame Ops are otherwise tessellated again on
// every frame.
type visualOpCache struct {
	entries map[visualOpKey]*visualOpEntry
	paths   map[roundedPathKey]*visualPathEntry
	clock   uint64
	limit   int
}

func newVisualOpCache(limit int) *visualOpCache {
	if limit <= 0 {
		limit = maxVisualOpCacheEntries
	}
	return &visualOpCache{
		entries: make(map[visualOpKey]*visualOpEntry),
		paths:   make(map[roundedPathKey]*visualPathEntry),
		limit:   limit,
	}
}

func (c *visualOpCache) add(dst *op.Ops, key visualOpKey, record func(*op.Ops)) {
	if c == nil {
		record(dst)
		return
	}
	c.clock++
	if entry, ok := c.entries[key]; ok {
		entry.lastUsed = c.clock
		entry.call.Add(dst)
		return
	}
	if len(c.entries) >= c.limit {
		var oldestKey visualOpKey
		var oldest uint64
		first := true
		for candidateKey, candidate := range c.entries {
			if first || candidate.lastUsed < oldest {
				oldestKey = candidateKey
				oldest = candidate.lastUsed
				first = false
			}
		}
		delete(c.entries, oldestKey)
	}
	entry := &visualOpEntry{lastUsed: c.clock}
	macro := op.Record(&entry.ops)
	record(&entry.ops)
	entry.call = macro.Stop()
	c.entries[key] = entry
	entry.call.Add(dst)
}

func (c *visualOpCache) reset() {
	if c == nil {
		return
	}
	c.entries = make(map[visualOpKey]*visualOpEntry)
	c.paths = make(map[roundedPathKey]*visualPathEntry)
	c.clock = 0
}

func (c *visualOpCache) roundedClip(size image.Point, radius int) clip.Op {
	c.clock++
	key := roundedPathKey{size: size, radius: radius}
	if entry, ok := c.paths[key]; ok {
		entry.lastUsed = c.clock
		return clip.Outline{Path: entry.path}.Op()
	}
	if len(c.paths) >= c.limit {
		var oldestKey roundedPathKey
		var oldest uint64
		first := true
		for candidateKey, candidate := range c.paths {
			if first || candidate.lastUsed < oldest {
				oldestKey = candidateKey
				oldest = candidate.lastUsed
				first = false
			}
		}
		delete(c.paths, oldestKey)
	}
	entry := &visualPathEntry{lastUsed: c.clock}
	entry.path = clip.UniformRRect(image.Rectangle{Max: size}, radius).Path(&entry.ops)
	c.paths[key] = entry
	return clip.Outline{Path: entry.path}.Op()
}

func (r *Renderer) cachedRoundedClip(gtx layout.Context, size image.Point, radius unit.Dp) clip.Op {
	if r.visualOps == nil {
		return clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(radius)).Op(gtx.Ops)
	}
	return r.visualOps.roundedClip(size, gtx.Dp(radius))
}

func (r *Renderer) cachedRoundedClipPx(ops *op.Ops, size image.Point, radius int) clip.Op {
	if r.visualOps == nil {
		return clip.UniformRRect(image.Rectangle{Max: size}, radius).Op(ops)
	}
	return r.visualOps.roundedClip(size, radius)
}

func (r *Renderer) paintCachedRoundedFill(gtx layout.Context, size image.Point, radius unit.Dp, fill color.NRGBA) {
	r.paintCachedRoundedFillPx(gtx.Ops, size, gtx.Dp(radius), fill)
}

func (r *Renderer) paintCachedRoundedFillPx(ops *op.Ops, size image.Point, radiusPx int, fill color.NRGBA) {
	key := visualOpKey{kind: "rounded-fill", size: size, radius: radiusPx, color1: fill}
	r.visualOps.add(ops, key, func(cachedOps *op.Ops) {
		paint.FillShape(cachedOps, fill, clip.UniformRRect(image.Rectangle{Max: size}, radiusPx).Op(cachedOps))
	})
}

func (r *Renderer) paintCachedRoundedBorder(gtx layout.Context, size image.Point, radius, width unit.Dp, ink color.NRGBA) {
	radiusPx := gtx.Dp(radius)
	widthPx := gtx.Dp(width)
	key := visualOpKey{kind: "rounded-border", size: size, radius: radiusPx, width: widthPx, color1: ink}
	r.visualOps.add(gtx.Ops, key, func(ops *op.Ops) {
		whalf := (widthPx + 1) / 2
		innerSize := image.Pt(size.X-whalf*2, size.Y-whalf*2)
		rect := image.Rectangle{Max: innerSize}.Add(image.Pt(whalf, whalf))
		paint.FillShape(ops, ink, clip.Stroke{
			Path:  clip.UniformRRect(rect, radiusPx).Path(ops),
			Width: float32(widthPx),
		}.Op())
	})
}

func (r *Renderer) layoutCachedBorder(gtx layout.Context, ink color.NRGBA, radius, width unit.Dp, content layout.Widget) layout.Dimensions {
	dims := content(gtx)
	r.paintCachedRoundedBorder(gtx, dims.Size, radius, width, ink)
	return dims
}
