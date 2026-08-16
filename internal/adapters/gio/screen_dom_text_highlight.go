//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

// paintDOMTextHighlight paints a programmatic text range without relying on
// widget.Editor focus. Gio deliberately hides editor selections while another
// control is focused, but output search must leave focus in the search field.
func paintDOMTextHighlight(gtx layout.Context, shaper *text.Shaper, typography nativeTextStyle, value string, start, end int, ink color.NRGBA) {
	if shaper == nil || value == "" || end <= start {
		return
	}
	shaper.LayoutString(text.Parameters{
		Font: typography.font, PxPerEm: fixed.I(gtx.Sp(typography.size)),
		MaxWidth: gtx.Constraints.Max.X, MinWidth: gtx.Constraints.Min.X,
		Locale: gtx.Locale, LineHeightScale: typography.lineHeight,
	}, value)
	glyphs := make([]text.Glyph, 0, len(value))
	for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
		glyphs = append(glyphs, glyph)
	}
	for _, region := range domTextHighlightRegions(glyphs, start, end) {
		paint.FillShape(gtx.Ops, ink, clip.Rect(region).Op())
	}
}

// domTextHighlightRegions converts a rune range to the logical rectangles of
// the shaped glyph clusters it intersects. Matches inside a grapheme cluster
// highlight the complete cluster, which is the same clamping behavior used by
// Gio's editor selection model.
func domTextHighlightRegions(glyphs []text.Glyph, start, end int) []image.Rectangle {
	if end <= start {
		return nil
	}
	runeOffset := 0
	regions := make([]image.Rectangle, 0, 2)
	cluster := image.Rectangle{}
	clusterSet := false
	paragraphBreak := false
	for _, glyph := range glyphs {
		left, right := glyph.X.Floor(), (glyph.X + glyph.Advance).Ceil()
		if right < left {
			left, right = right, left
		}
		bounds := image.Rect(left, int(glyph.Y)-glyph.Ascent.Ceil(), right, int(glyph.Y)+glyph.Descent.Ceil())
		if !clusterSet {
			cluster, clusterSet = bounds, true
		} else {
			cluster = cluster.Union(bounds)
		}
		paragraphBreak = paragraphBreak || glyph.Flags&text.FlagParagraphBreak != 0
		if glyph.Flags&text.FlagClusterBreak == 0 {
			continue
		}
		clusterEnd := runeOffset + int(glyph.Runes)
		if !paragraphBreak && clusterEnd > start && runeOffset < end && cluster.Dx() > 0 && cluster.Dy() > 0 {
			regions = appendDOMTextHighlightRegion(regions, cluster)
		}
		runeOffset = clusterEnd
		cluster, clusterSet, paragraphBreak = image.Rectangle{}, false, false
		if runeOffset >= end {
			break
		}
	}
	return regions
}

func appendDOMTextHighlightRegion(regions []image.Rectangle, region image.Rectangle) []image.Rectangle {
	if len(regions) == 0 {
		return append(regions, region)
	}
	last := &regions[len(regions)-1]
	sameLine := last.Min.Y == region.Min.Y && last.Max.Y == region.Max.Y
	touching := region.Min.X <= last.Max.X+1 && region.Max.X >= last.Min.X-1
	if sameLine && touching {
		*last = last.Union(region)
		return regions
	}
	return append(regions, region)
}
