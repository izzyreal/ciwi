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
	"gioui.org/op/paint"
	"golang.org/x/image/vector"
)

const maxLoaderTextureEntries = 32
const loaderTextureScale = 3

type loaderTextureKey struct {
	size image.Point
	ink  color.NRGBA
}

type loaderTextureEntry struct {
	source   paint.ImageOp
	lastUsed uint64
}

func (r *Renderer) layoutAnimatedLoader(gtx layout.Context, ink color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Min
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{Size: size}
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
	source := r.loaderTexture(size, ink)
	center := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
	angle := float32(float64(gtx.Now.UnixNano()%int64(time.Second)) / float64(time.Second) * 2 * math.Pi)
	bounds := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	transform := op.Affine(f32.Affine2D{}.Rotate(center, angle)).Push(gtx.Ops)
	paintScaledImageOps(gtx.Ops, source, size)
	transform.Pop()
	bounds.Pop()
	return layout.Dimensions{Size: size}
}

func (r *Renderer) loaderTexture(size image.Point, ink color.NRGBA) paint.ImageOp {
	if r.loaderTextures == nil {
		r.loaderTextures = make(map[loaderTextureKey]*loaderTextureEntry)
	}
	r.loaderTextureClock++
	key := loaderTextureKey{size: size, ink: ink}
	if entry, ok := r.loaderTextures[key]; ok {
		entry.lastUsed = r.loaderTextureClock
		return entry.source
	}
	if len(r.loaderTextures) >= maxLoaderTextureEntries {
		var oldestKey loaderTextureKey
		var oldest uint64
		first := true
		for candidateKey, candidate := range r.loaderTextures {
			if first || candidate.lastUsed < oldest {
				oldestKey = candidateKey
				oldest = candidate.lastUsed
				first = false
			}
		}
		delete(r.loaderTextures, oldestKey)
	}
	rasterSize := image.Pt(size.X*loaderTextureScale, size.Y*loaderTextureScale)
	source := paint.NewImageOp(renderLoaderTexture(rasterSize, ink))
	source.Filter = paint.FilterLinear
	r.loaderTextures[key] = &loaderTextureEntry{source: source, lastUsed: r.loaderTextureClock}
	return source
}

func (r *Renderer) resetLoaderTextures() {
	r.loaderTextures = make(map[loaderTextureKey]*loaderTextureEntry)
	r.loaderTextureClock = 0
}

func renderLoaderTexture(size image.Point, ink color.NRGBA) *image.RGBA {
	texture := image.NewRGBA(image.Rectangle{Max: size})
	if size.X <= 0 || size.Y <= 0 || ink.A == 0 {
		return texture
	}
	minimum := float32(min(size.X, size.Y))
	centerX, centerY := float32(size.X)/2, float32(size.Y)/2
	radius := minimum * 9 / 24
	halfStroke := minimum * 1.9 / 24 / 2
	outerRadius, innerRadius := radius+halfStroke, max(float32(0), radius-halfStroke)
	const (
		startAngle = -math.Pi / 2
		endAngle   = math.Pi
		segments   = 48
		capSteps   = 16
	)
	rasterizer := vector.NewRasterizer(size.X, size.Y)
	point := func(radius float32, angle float64) (float32, float32) {
		return centerX + radius*float32(math.Cos(angle)), centerY + radius*float32(math.Sin(angle))
	}
	x, y := point(outerRadius, startAngle)
	rasterizer.MoveTo(x, y)
	for step := 1; step <= segments; step++ {
		angle := startAngle + (endAngle-startAngle)*float64(step)/segments
		x, y = point(outerRadius, angle)
		rasterizer.LineTo(x, y)
	}
	for step := segments; step >= 0; step-- {
		angle := startAngle + (endAngle-startAngle)*float64(step)/segments
		x, y = point(innerRadius, angle)
		rasterizer.LineTo(x, y)
	}
	rasterizer.ClosePath()
	for _, angle := range []float64{startAngle, endAngle} {
		capX, capY := point(radius, angle)
		rasterizer.MoveTo(capX+halfStroke, capY)
		for step := 1; step <= capSteps; step++ {
			capAngle := 2 * math.Pi * float64(step) / capSteps
			rasterizer.LineTo(capX+halfStroke*float32(math.Cos(capAngle)), capY+halfStroke*float32(math.Sin(capAngle)))
		}
		rasterizer.ClosePath()
	}
	rasterizer.Draw(texture, texture.Bounds(), image.NewUniform(ink), image.Point{})
	return texture
}
