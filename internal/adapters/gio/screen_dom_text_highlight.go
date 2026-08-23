//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"image/color"
	"sort"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

type domTextHitCluster struct {
	start, end int
	bounds     image.Rectangle
	baseline   int
	rtl        bool
}

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

// domTextRuneAtPoint maps a pointer position to the nearest shaped grapheme
// boundary. It uses the same shaping parameters as painting, so virtual log
// block boundaries never become selection boundaries.
func domTextRuneAtPoint(gtx layout.Context, shaper *text.Shaper, typography nativeTextStyle, value string, point image.Point) int {
	if shaper == nil || value == "" {
		return 0
	}
	shaper.LayoutString(text.Parameters{
		Font: typography.font, PxPerEm: fixed.I(gtx.Sp(typography.size)),
		MaxWidth: gtx.Constraints.Max.X, MinWidth: gtx.Constraints.Min.X,
		Locale: gtx.Locale, LineHeightScale: typography.lineHeight,
	}, value)
	clusters := make([]domTextHitCluster, 0, len(value))
	runeOffset := 0
	bounds := image.Rectangle{}
	boundsSet := false
	baseline := 0
	rtl := false
	for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
		left, right := glyph.X.Floor(), (glyph.X + glyph.Advance).Ceil()
		if right < left {
			left, right = right, left
		}
		glyphBounds := image.Rect(left, int(glyph.Y)-glyph.Ascent.Ceil(), right, int(glyph.Y)+glyph.Descent.Ceil())
		if !boundsSet {
			bounds, boundsSet = glyphBounds, true
		} else {
			bounds = bounds.Union(glyphBounds)
		}
		rtl = rtl || glyph.Flags&text.FlagTowardOrigin != 0
		baseline = int(glyph.Y)
		if glyph.Flags&text.FlagClusterBreak == 0 {
			continue
		}
		end := runeOffset + int(glyph.Runes)
		if boundsSet {
			if bounds.Dx() <= 0 {
				bounds.Max.X = bounds.Min.X + 1
			}
			if bounds.Dy() <= 0 {
				bounds.Max.Y = bounds.Min.Y + max(1, gtx.Sp(typography.size))
			}
			clusters = append(clusters, domTextHitCluster{start: runeOffset, end: end, bounds: bounds, baseline: baseline, rtl: rtl})
		}
		runeOffset = end
		bounds, boundsSet, rtl = image.Rectangle{}, false, false
	}
	if len(clusters) == 0 {
		return min(len([]rune(value)), runeOffset)
	}
	lineDistance := func(cluster domTextHitCluster) int {
		if point.Y < cluster.bounds.Min.Y {
			return cluster.bounds.Min.Y - point.Y
		}
		if point.Y > cluster.bounds.Max.Y {
			return point.Y - cluster.bounds.Max.Y
		}
		return 0
	}
	bestLineDistance := int(^uint(0) >> 1)
	bestBaseline := clusters[0].baseline
	for _, cluster := range clusters {
		if distance := lineDistance(cluster); distance < bestLineDistance {
			bestLineDistance, bestBaseline = distance, cluster.baseline
		}
	}
	line := make([]domTextHitCluster, 0, 16)
	for _, cluster := range clusters {
		if cluster.baseline == bestBaseline {
			line = append(line, cluster)
		}
	}
	sort.SliceStable(line, func(i, j int) bool { return line[i].bounds.Min.X < line[j].bounds.Min.X })
	if len(line) == 0 {
		return 0
	}
	for _, cluster := range line {
		middle := cluster.bounds.Min.X + cluster.bounds.Dx()/2
		if point.X <= cluster.bounds.Max.X {
			before := point.X < middle
			if cluster.rtl {
				before = !before
			}
			if before {
				return cluster.start
			}
			return cluster.end
		}
	}
	return line[len(line)-1].end
}
