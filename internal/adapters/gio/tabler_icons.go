//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

type nativeIcon interface {
	Layout(layout.Context, color.NRGBA) layout.Dimensions
}

type tablerIcon struct{ draw func(*clip.Path) }

func (icon tablerIcon) Layout(gtx layout.Context, ink color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Min.X
	if size == 0 {
		size = gtx.Dp(24)
	}
	size = gtx.Constraints.Constrain(image.Pt(size, size)).X
	scale := float32(size) / 24
	transform := op.Affine(f32.Affine2D{}.Scale(f32.Point{}, f32.Pt(scale, scale))).Push(gtx.Ops)
	var path clip.Path
	path.Begin(gtx.Ops)
	icon.draw(&path)
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: path.End(), Width: 1.9}.Op())
	transform.Pop()
	return layout.Dimensions{Size: image.Pt(size, size)}
}

func tablerIcons() map[string]nativeIcon {
	line := func(points ...f32.Point) func(*clip.Path) {
		return func(path *clip.Path) {
			if len(points) == 0 {
				return
			}
			path.MoveTo(points[0])
			for _, point := range points[1:] {
				path.LineTo(point)
			}
		}
	}
	paths := func(draws ...func(*clip.Path)) func(*clip.Path) {
		return func(path *clip.Path) {
			for _, draw := range draws {
				draw(path)
			}
		}
	}
	circle := func(center f32.Point, radius float32) func(*clip.Path) {
		return func(path *clip.Path) {
			const k = float32(4 * (math.Sqrt2 - 1) / 3)
			c := radius * k
			path.MoveTo(f32.Pt(center.X, center.Y-radius))
			path.CubeTo(f32.Pt(center.X+c, center.Y-radius), f32.Pt(center.X+radius, center.Y-c), f32.Pt(center.X+radius, center.Y))
			path.CubeTo(f32.Pt(center.X+radius, center.Y+c), f32.Pt(center.X+c, center.Y+radius), f32.Pt(center.X, center.Y+radius))
			path.CubeTo(f32.Pt(center.X-c, center.Y+radius), f32.Pt(center.X-radius, center.Y+c), f32.Pt(center.X-radius, center.Y))
			path.CubeTo(f32.Pt(center.X-radius, center.Y-c), f32.Pt(center.X-c, center.Y-radius), f32.Pt(center.X, center.Y-radius))
		}
	}
	arc := func(from, c1, c2, to f32.Point) func(*clip.Path) {
		return func(path *clip.Path) { path.MoveTo(from); path.CubeTo(c1, c2, to) }
	}
	roundedRect := func(x, y, width, height, radius float32) func(*clip.Path) {
		const k = float32(4 * (math.Sqrt2 - 1) / 3)
		curve := radius * k
		return func(path *clip.Path) {
			path.MoveTo(f32.Pt(x+radius, y))
			path.LineTo(f32.Pt(x+width-radius, y))
			path.CubeTo(f32.Pt(x+width-radius+curve, y), f32.Pt(x+width, y+radius-curve), f32.Pt(x+width, y+radius))
			path.LineTo(f32.Pt(x+width, y+height-radius))
			path.CubeTo(f32.Pt(x+width, y+height-radius+curve), f32.Pt(x+width-radius+curve, y+height), f32.Pt(x+width-radius, y+height))
			path.LineTo(f32.Pt(x+radius, y+height))
			path.CubeTo(f32.Pt(x+radius-curve, y+height), f32.Pt(x, y+height-radius+curve), f32.Pt(x, y+height-radius))
			path.LineTo(f32.Pt(x, y+radius))
			path.CubeTo(f32.Pt(x, y+radius-curve), f32.Pt(x+radius-curve, y), f32.Pt(x+radius, y))
		}
	}
	definitions := map[string]tablerIcon{
		"arrow-left":        tablerIcon{paths(line(f32.Pt(5, 12), f32.Pt(19, 12)), line(f32.Pt(5, 12), f32.Pt(11, 18)), line(f32.Pt(5, 12), f32.Pt(11, 6)))},
		"arrow-up":          tablerIcon{paths(line(f32.Pt(12, 5), f32.Pt(12, 19)), line(f32.Pt(18, 11), f32.Pt(12, 5), f32.Pt(6, 11)))},
		"arrow-bar-to-down": tablerIcon{paths(line(f32.Pt(4, 20), f32.Pt(20, 20)), line(f32.Pt(12, 14), f32.Pt(12, 4)), line(f32.Pt(12, 14), f32.Pt(16, 10)), line(f32.Pt(12, 14), f32.Pt(8, 10)))},
		"chevron-down":      tablerIcon{line(f32.Pt(6, 9), f32.Pt(12, 15), f32.Pt(18, 9))},
		"chevron-right":     tablerIcon{line(f32.Pt(9, 6), f32.Pt(15, 12), f32.Pt(9, 18))},
		"chevron-up":        tablerIcon{line(f32.Pt(6, 15), f32.Pt(12, 9), f32.Pt(18, 15))},
		"chevrons-down":     tablerIcon{paths(line(f32.Pt(7, 7), f32.Pt(12, 12), f32.Pt(17, 7)), line(f32.Pt(7, 13), f32.Pt(12, 18), f32.Pt(17, 13)))},
		"chevrons-up":       tablerIcon{paths(line(f32.Pt(7, 11), f32.Pt(12, 6), f32.Pt(17, 11)), line(f32.Pt(7, 17), f32.Pt(12, 12), f32.Pt(17, 17)))},
		"player-play":       tablerIcon{line(f32.Pt(7, 4), f32.Pt(7, 20), f32.Pt(20, 12), f32.Pt(7, 4))},
		"loader-2": tablerIcon{func(path *clip.Path) {
			path.MoveTo(f32.Pt(12, 3))
			path.CubeTo(f32.Pt(17, 3), f32.Pt(21, 7), f32.Pt(21, 12))
			path.CubeTo(f32.Pt(21, 17), f32.Pt(17, 21), f32.Pt(12, 21))
			path.CubeTo(f32.Pt(7, 21), f32.Pt(3, 17), f32.Pt(3, 12))
		}},
		"heart": tablerIcon{func(path *clip.Path) {
			path.MoveTo(f32.Pt(19.5, 12.572))
			path.LineTo(f32.Pt(12, 20))
			path.LineTo(f32.Pt(4.5, 12.572))
			path.CubeTo(f32.Pt(1.5, 9.5), f32.Pt(2, 4), f32.Pt(7, 4))
			path.CubeTo(f32.Pt(9, 4), f32.Pt(11, 5), f32.Pt(12, 6))
			path.CubeTo(f32.Pt(13, 5), f32.Pt(15, 4), f32.Pt(17, 4))
			path.CubeTo(f32.Pt(22, 4), f32.Pt(22.5, 9.5), f32.Pt(19.5, 12.572))
		}},
		"check":          tablerIcon{line(f32.Pt(5, 12), f32.Pt(10, 17), f32.Pt(19, 7))},
		"circle-x":       tablerIcon{paths(circle(f32.Pt(12, 12), 9), line(f32.Pt(10, 10), f32.Pt(14, 14)), line(f32.Pt(14, 10), f32.Pt(10, 14)))},
		"status-success": tablerIcon{paths(circle(f32.Pt(12, 12), 9), line(f32.Pt(8.5, 12), f32.Pt(11, 14.5), f32.Pt(16, 9.5)))},
		"status-danger":  tablerIcon{paths(circle(f32.Pt(12, 12), 9), line(f32.Pt(9, 9), f32.Pt(15, 15)), line(f32.Pt(15, 9), f32.Pt(9, 15)))},
		"status-running": tablerIcon{paths(circle(f32.Pt(12, 12), 9), line(f32.Pt(10, 8), f32.Pt(10, 16), f32.Pt(16, 12), f32.Pt(10, 8)))},
		"status-waiting": tablerIcon{paths(line(f32.Pt(7, 3), f32.Pt(17, 3)), line(f32.Pt(7, 21), f32.Pt(17, 21)), line(f32.Pt(8, 3), f32.Pt(8, 7), f32.Pt(12, 11), f32.Pt(16, 7), f32.Pt(16, 3)), line(f32.Pt(8, 21), f32.Pt(8, 17), f32.Pt(12, 13), f32.Pt(16, 17), f32.Pt(16, 21)))},
		"server":         tablerIcon{paths(roundedRect(3, 4, 18, 6, 2), roundedRect(3, 14, 18, 6, 2), circle(f32.Pt(7, 7), .05), circle(f32.Pt(7, 17), .05))},
		"terminal-2":     tablerIcon{paths(line(f32.Pt(4, 5), f32.Pt(20, 5), f32.Pt(20, 19), f32.Pt(4, 19), f32.Pt(4, 5)), line(f32.Pt(8, 9), f32.Pt(11, 12), f32.Pt(8, 15)), line(f32.Pt(13, 15), f32.Pt(16, 15)))},
		"network":        tablerIcon{paths(circle(f32.Pt(12, 5), 2), circle(f32.Pt(5, 18), 2), circle(f32.Pt(19, 18), 2), line(f32.Pt(12, 7), f32.Pt(12, 11), f32.Pt(5, 16)), line(f32.Pt(12, 11), f32.Pt(19, 16)))},
		"trash":          tablerIcon{paths(line(f32.Pt(4, 7), f32.Pt(20, 7)), line(f32.Pt(10, 11), f32.Pt(10, 17)), line(f32.Pt(14, 11), f32.Pt(14, 17)), line(f32.Pt(5, 7), f32.Pt(6, 20), f32.Pt(8, 22), f32.Pt(16, 22), f32.Pt(18, 20), f32.Pt(19, 7)), line(f32.Pt(9, 7), f32.Pt(9, 4), f32.Pt(10, 3), f32.Pt(14, 3), f32.Pt(15, 4), f32.Pt(15, 7)))},
		"zoom-in":        tablerIcon{paths(circle(f32.Pt(10, 10), 7), line(f32.Pt(7, 10), f32.Pt(13, 10)), line(f32.Pt(10, 7), f32.Pt(10, 13)), line(f32.Pt(15, 15), f32.Pt(21, 21)))},
		"zoom-out":       tablerIcon{paths(circle(f32.Pt(10, 10), 7), line(f32.Pt(7, 10), f32.Pt(13, 10)), line(f32.Pt(15, 15), f32.Pt(21, 21)))},
		"refresh":        tablerIcon{paths(arc(f32.Pt(20, 11), f32.Pt(19, 4), f32.Pt(9, 1), f32.Pt(4.5, 9)), line(f32.Pt(4, 5), f32.Pt(4, 9), f32.Pt(8, 9)), arc(f32.Pt(4, 13), f32.Pt(5, 20), f32.Pt(15, 23), f32.Pt(19.5, 15)), line(f32.Pt(20, 19), f32.Pt(20, 15), f32.Pt(16, 15)))},
		"copy":           tablerIcon{paths(line(f32.Pt(8, 8), f32.Pt(20, 8), f32.Pt(20, 20), f32.Pt(8, 20), f32.Pt(8, 8)), line(f32.Pt(16, 8), f32.Pt(16, 4), f32.Pt(4, 4), f32.Pt(4, 16), f32.Pt(8, 16)))},
		"download":       tablerIcon{paths(line(f32.Pt(12, 3), f32.Pt(12, 15)), line(f32.Pt(7, 10), f32.Pt(12, 15), f32.Pt(17, 10)), line(f32.Pt(5, 21), f32.Pt(19, 21)))},
		"adjustments":    tablerIcon{paths(line(f32.Pt(4, 6), f32.Pt(10, 6)), line(f32.Pt(14, 6), f32.Pt(20, 6)), circle(f32.Pt(12, 6), 2), line(f32.Pt(4, 12), f32.Pt(6, 12)), line(f32.Pt(10, 12), f32.Pt(20, 12)), circle(f32.Pt(8, 12), 2), line(f32.Pt(4, 18), f32.Pt(14, 18)), line(f32.Pt(18, 18), f32.Pt(20, 18)), circle(f32.Pt(16, 18), 2))},
		"settings":       tablerIcon{paths(circle(f32.Pt(12, 12), 3), circle(f32.Pt(12, 12), 8), line(f32.Pt(12, 2), f32.Pt(12, 4)), line(f32.Pt(12, 20), f32.Pt(12, 22)), line(f32.Pt(2, 12), f32.Pt(4, 12)), line(f32.Pt(20, 12), f32.Pt(22, 12)), line(f32.Pt(5, 5), f32.Pt(6.5, 6.5)), line(f32.Pt(17.5, 17.5), f32.Pt(19, 19)), line(f32.Pt(19, 5), f32.Pt(17.5, 6.5)), line(f32.Pt(6.5, 17.5), f32.Pt(5, 19)))},
		"vault": tablerIcon{paths(
			func(path *clip.Path) {
				path.MoveTo(f32.Pt(6, 3))
				path.LineTo(f32.Pt(18, 3))
				path.CubeTo(f32.Pt(19.66, 3), f32.Pt(21, 4.34), f32.Pt(21, 6))
				path.LineTo(f32.Pt(21, 18))
				path.CubeTo(f32.Pt(21, 19.66), f32.Pt(19.66, 21), f32.Pt(18, 21))
				path.LineTo(f32.Pt(6, 21))
				path.CubeTo(f32.Pt(4.34, 21), f32.Pt(3, 19.66), f32.Pt(3, 18))
				path.LineTo(f32.Pt(3, 6))
				path.CubeTo(f32.Pt(3, 4.34), f32.Pt(4.34, 3), f32.Pt(6, 3))
			},
			circle(f32.Pt(12, 12), 3),
			line(f32.Pt(9.75, 9.75), f32.Pt(8, 8)),
			line(f32.Pt(14.25, 9.75), f32.Pt(16, 8)),
			line(f32.Pt(14.25, 14.25), f32.Pt(16, 16)),
			line(f32.Pt(9.75, 14.25), f32.Pt(8, 16)),
		)},
	}
	icons := make(map[string]nativeIcon, len(definitions))
	for name, definition := range definitions {
		icons[name] = definition
	}
	return icons
}
