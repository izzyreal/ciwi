//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// layoutAnimatedLoader rotates the vector icon directly. It deliberately
// retains no size/color texture cache; the current frame's ops are sufficient.
func (r *Renderer) layoutAnimatedLoader(gtx layout.Context, ink color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Min
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{Size: size}
	}
	icon := r.icons["loader-2"]
	if icon == nil {
		return layout.Dimensions{Size: size}
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
	center := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
	angle := float32(float64(gtx.Now.UnixNano()%int64(time.Second)) / float64(time.Second) * 2 * math.Pi)
	bounds := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	transform := op.Affine(f32.Affine2D{}.Rotate(center, angle)).Push(gtx.Ops)
	iconContext := gtx
	iconContext.Constraints = layout.Exact(size)
	icon.Layout(iconContext, ink)
	transform.Pop()
	bounds.Pop()
	return layout.Dimensions{Size: size}
}
