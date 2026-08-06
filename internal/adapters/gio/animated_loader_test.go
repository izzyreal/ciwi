//go:build darwin || linux || windows

package gio

import (
	"image"
	"image/color"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
)

func TestRenderLoaderTextureIsColoredAndTransparent(t *testing.T) {
	ink := color.NRGBA{R: 40, G: 180, B: 240, A: 255}
	texture := renderLoaderTexture(image.Pt(96, 96), ink)
	if texture.Bounds().Size() != image.Pt(96, 96) {
		t.Fatalf("texture size = %v", texture.Bounds().Size())
	}
	if corner := texture.RGBAAt(0, 0); corner.A != 0 {
		t.Fatalf("corner alpha = %d, want transparent", corner.A)
	}
	foundOpaqueInk := false
	for y := texture.Bounds().Min.Y; y < texture.Bounds().Max.Y && !foundOpaqueInk; y++ {
		for x := texture.Bounds().Min.X; x < texture.Bounds().Max.X; x++ {
			pixel := texture.RGBAAt(x, y)
			if pixel.A == 255 && pixel.R == ink.R && pixel.G == ink.G && pixel.B == ink.B {
				foundOpaqueInk = true
				break
			}
		}
	}
	if !foundOpaqueInk {
		t.Fatal("texture contains no opaque loader pixels in the requested color")
	}
}

func TestLoaderTextureCacheReusesEvictsAndResets(t *testing.T) {
	renderer := &Renderer{}
	key := loaderTextureKey{size: image.Pt(18, 18), ink: color.NRGBA{R: 1, A: 255}}
	source := renderer.loaderTexture(key.size, key.ink)
	if source.Size() != image.Pt(key.size.X*loaderTextureScale, key.size.Y*loaderTextureScale) {
		t.Fatalf("loader raster size = %v", source.Size())
	}
	first := renderer.loaderTextures[key]
	renderer.loaderTexture(key.size, key.ink)
	if renderer.loaderTextures[key] != first {
		t.Fatal("stable loader texture was replaced")
	}
	for index := 0; index < maxLoaderTextureEntries; index++ {
		ink := color.NRGBA{R: uint8(index + 2), G: uint8(index), A: 255}
		renderer.loaderTexture(image.Pt(19, 19), ink)
	}
	if len(renderer.loaderTextures) != maxLoaderTextureEntries {
		t.Fatalf("loader cache entries = %d, want %d", len(renderer.loaderTextures), maxLoaderTextureEntries)
	}
	if _, exists := renderer.loaderTextures[key]; exists {
		t.Fatal("least-recently-used loader texture was not evicted")
	}
	renderer.resetLoaderTextures()
	if len(renderer.loaderTextures) != 0 || renderer.loaderTextureClock != 0 {
		t.Fatalf("reset loader cache = %d entries at clock %d", len(renderer.loaderTextures), renderer.loaderTextureClock)
	}
}

func TestAnimatedLoaderReusesTextureAcrossAngles(t *testing.T) {
	renderer := &Renderer{}
	ink := color.NRGBA{R: 40, G: 180, B: 240, A: 255}
	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(18, 18)), Now: time.Unix(1_800_000_000, 0)}
	if dimensions := renderer.layoutAnimatedLoader(gtx, ink); dimensions.Size != image.Pt(18, 18) {
		t.Fatalf("loader dimensions = %v", dimensions.Size)
	}
	key := loaderTextureKey{size: image.Pt(18, 18), ink: ink}
	first := renderer.loaderTextures[key]
	operations.Reset()
	gtx.Now = gtx.Now.Add(time.Second / 3)
	renderer.layoutAnimatedLoader(gtx, ink)
	if len(renderer.loaderTextures) != 1 || renderer.loaderTextures[key] != first {
		t.Fatal("changing the loader angle replaced its cached texture")
	}
}
