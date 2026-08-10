//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	giotext "gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

func (r *Renderer) paintPageBackground(gtx layout.Context) {
	viewport := gtx.Constraints.Max
	if viewport.X <= 0 || viewport.Y <= 0 {
		return
	}
	stack := clip.Rect{Max: viewport}.Push(gtx.Ops)
	textureSize := gradientTextureSize(viewport)
	if !r.pageBackgroundReady || r.pageBackgroundSize != textureSize {
		r.pageBackground = paint.NewImageOp(renderPageBackground(textureSize, r.palette))
		r.pageBackground.Filter = paint.FilterLinear
		r.pageBackgroundSize, r.pageBackgroundReady = textureSize, true
	}
	paintScaledImageOps(gtx.Ops, r.pageBackground, viewport)
	stack.Pop()
}

const maxGradientTextureDimension = 384

func gradientTextureSize(target image.Point) image.Point {
	maximum := max(target.X, target.Y)
	if maximum <= 0 {
		return image.Point{}
	}
	if maximum <= maxGradientTextureDimension {
		return target
	}
	scale := float64(maxGradientTextureDimension) / float64(maximum)
	return image.Pt(max(1, int(math.Round(float64(target.X)*scale))), max(1, int(math.Round(float64(target.Y)*scale))))
}

func paintScaledImageOps(ops *op.Ops, imageOp paint.ImageOp, target image.Point) {
	source := imageOp.Size()
	if source.X <= 0 || source.Y <= 0 || target.X <= 0 || target.Y <= 0 {
		return
	}
	scale := f32.Pt(float32(target.X)/float32(source.X), float32(target.Y)/float32(source.Y))
	transform := op.Affine(f32.AffineId().Scale(f32.Point{}, scale)).Push(ops)
	imageOp.Add(ops)
	paint.PaintOp{}.Add(ops)
	transform.Pop()
}

func cssGradientLine(rect image.Rectangle, angleDegrees float64) (f32.Point, f32.Point) {
	angle := angleDegrees * math.Pi / 180
	direction := f32.Pt(float32(math.Sin(angle)), float32(-math.Cos(angle)))
	width, height := float32(rect.Dx()), float32(rect.Dy())
	halfExtent := (float32(math.Abs(float64(direction.X)))*width + float32(math.Abs(float64(direction.Y)))*height) / 2
	center := f32.Pt(float32(rect.Min.X)+width/2, float32(rect.Min.Y)+height/2)
	return f32.Pt(center.X-direction.X*halfExtent, center.Y-direction.Y*halfExtent),
		f32.Pt(center.X+direction.X*halfExtent, center.Y+direction.Y*halfExtent)
}

func renderPageBackground(size image.Point, colors palette) *image.NRGBA {
	gradient := newSampledGradient(size, colors.pageGradient)
	glowB := newRadialGlow(size, .90, .08, .34, colors.backgroundGlowB, .82)
	glowA := newRadialGlow(size, .12, -.10, .38, colors.backgroundGlowA, .86)
	result := image.NewNRGBA(image.Rectangle{Max: size})
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			base := gradient.pixel(float64(x)+.5, float64(y)+.5)
			base = glowB.composite(base, float64(x)+.5, float64(y)+.5)
			result.SetNRGBA(x, y, glowA.composite(base, float64(x)+.5, float64(y)+.5))
		}
	}
	return result
}

func (r *Renderer) paintHeroSurface(gtx layout.Context, size image.Point) {
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	textureSize := gradientTextureSize(size)
	if !r.heroBackgroundReady || r.heroBackgroundSize != textureSize {
		r.heroBackground = paint.NewImageOp(renderHeroBackground(textureSize, r.palette))
		r.heroBackground.Filter = paint.FilterLinear
		r.heroBackgroundSize, r.heroBackgroundReady = textureSize, true
	}
	paintScaledImageOps(gtx.Ops, r.heroBackground, size)
}

func renderHeroBackground(size image.Point, colors palette) *image.NRGBA {
	gradient := newSampledGradient(size, colors.heroGradient)
	result := image.NewNRGBA(image.Rectangle{Max: size})
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			result.SetNRGBA(x, y, gradient.pixel(float64(x)+.5, float64(y)+.5))
		}
	}
	return result
}

func (r *Renderer) paintCardSurface(gtx layout.Context, size image.Point) {
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	if !r.surfaceBackgroundReady {
		const dimension = maxGradientTextureDimension
		r.surfaceBackground = paint.NewImageOp(renderSurfaceBackground(image.Pt(dimension, dimension), r.palette))
		r.surfaceBackground.Filter = paint.FilterLinear
		r.surfaceBackgroundReady = true
	}
	paintScaledImageOps(gtx.Ops, r.surfaceBackground, size)
}

func renderSurfaceBackground(size image.Point, colors palette) *image.NRGBA {
	gradient := newThreeStopGradient(size, 145, colors.surface, 1, colors.subtle, colors.subtle)
	glow := newRadialGlow(size, 1, 0, .38, colors.surfaceGlow, 1)
	result := image.NewNRGBA(image.Rectangle{Max: size})
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			px, py := float64(x)+.5, float64(y)+.5
			result.SetNRGBA(x, y, glow.composite(gradient.pixel(px, py), px, py))
		}
	}
	return result
}

type threeStopGradient struct {
	startX, startY, dx, dy, denominator, middlePosition float64
	start, middle, end                                  color.NRGBA
}

func newThreeStopGradient(size image.Point, angleDegrees float64, start color.NRGBA, middlePosition float64, middle, end color.NRGBA) threeStopGradient {
	lineStart, lineEnd := cssGradientLine(image.Rectangle{Max: size}, angleDegrees)
	dx, dy := float64(lineEnd.X-lineStart.X), float64(lineEnd.Y-lineStart.Y)
	return threeStopGradient{
		startX: float64(lineStart.X), startY: float64(lineStart.Y), dx: dx, dy: dy,
		denominator: dx*dx + dy*dy, middlePosition: max(0, min(middlePosition, 1)),
		start: start, middle: middle, end: end,
	}
}

func (gradient threeStopGradient) pixel(x, y float64) color.NRGBA {
	t := 0.0
	if gradient.denominator > 0 {
		t = ((x-gradient.startX)*gradient.dx + (y-gradient.startY)*gradient.dy) / gradient.denominator
	}
	t = max(0, min(t, 1))
	if t <= gradient.middlePosition && gradient.middlePosition > 0 {
		return mixColorSRGB(gradient.start, gradient.middle, t/gradient.middlePosition)
	}
	if gradient.middlePosition >= 1 {
		return gradient.middle
	}
	return mixColorSRGB(gradient.middle, gradient.end, (t-gradient.middlePosition)/(1-gradient.middlePosition))
}

type sampledGradient struct {
	kind                          string
	startX, startY, dx, dy, scale float64
	stops                         []nativeGradientStop
}

func newSampledGradient(size image.Point, gradient nativeGradient) sampledGradient {
	sampled := sampledGradient{kind: gradient.kind, stops: gradient.stops}
	if gradient.kind == "radial" {
		sampled.startX, sampled.startY = float64(size.X)/2, float64(size.Y)/2
		for _, corner := range [][2]float64{{0, 0}, {float64(size.X), 0}, {float64(size.X), float64(size.Y)}, {0, float64(size.Y)}} {
			sampled.scale = max(sampled.scale, math.Hypot(corner[0]-sampled.startX, corner[1]-sampled.startY))
		}
		return sampled
	}
	lineStart, lineEnd := cssGradientLine(image.Rectangle{Max: size}, gradient.angle)
	sampled.startX, sampled.startY = float64(lineStart.X), float64(lineStart.Y)
	sampled.dx, sampled.dy = float64(lineEnd.X-lineStart.X), float64(lineEnd.Y-lineStart.Y)
	sampled.scale = sampled.dx*sampled.dx + sampled.dy*sampled.dy
	return sampled
}

func (gradient sampledGradient) pixel(x, y float64) color.NRGBA {
	if len(gradient.stops) == 0 {
		return color.NRGBA{}
	}
	t := 0.0
	if gradient.kind == "radial" {
		if gradient.scale > 0 {
			t = math.Hypot(x-gradient.startX, y-gradient.startY) / gradient.scale
		}
	} else if gradient.scale > 0 {
		t = ((x-gradient.startX)*gradient.dx + (y-gradient.startY)*gradient.dy) / gradient.scale
	}
	t = max(0, min(t, 1))
	previous := gradient.stops[0]
	if t <= previous.position {
		return previous.color
	}
	for _, next := range gradient.stops[1:] {
		if t <= next.position {
			span := next.position - previous.position
			if span <= 0 {
				return next.color
			}
			return mixColorSRGB(previous.color, next.color, (t-previous.position)/span)
		}
		previous = next
	}
	return previous.color
}

type radialGlow struct {
	cx, cy, radius, opacity float64
	color                   color.NRGBA
}

func newRadialGlow(size image.Point, centerX, centerY, stopPosition float64, glow color.NRGBA, opacity float64) radialGlow {
	cx, cy := float64(size.X)*centerX, float64(size.Y)*centerY
	maxRadius := 0.0
	for _, corner := range [][2]float64{{0, 0}, {float64(size.X), 0}, {float64(size.X), float64(size.Y)}, {0, float64(size.Y)}} {
		maxRadius = max(maxRadius, math.Hypot(corner[0]-cx, corner[1]-cy))
	}
	return radialGlow{cx: cx, cy: cy, radius: maxRadius * stopPosition, opacity: opacity * float64(glow.A) / 255, color: color.NRGBA{R: glow.R, G: glow.G, B: glow.B, A: 255}}
}

func (glow radialGlow) composite(background color.NRGBA, x, y float64) color.NRGBA {
	if glow.radius <= 0 || glow.opacity <= 0 {
		return background
	}
	alpha := glow.opacity * max(0, 1-math.Hypot(x-glow.cx, y-glow.cy)/glow.radius)
	return mixColorSRGB(background, glow.color, alpha)
}

func compactLayoutForPlatform(gtx layout.Context, platform string) bool {
	pxPerDp := gtx.Metric.PxPerDp
	if pxPerDp <= 0 {
		pxPerDp = 1
	}
	return compactViewport(platform, float32(gtx.Constraints.Max.X)/pxPerDp, float32(gtx.Constraints.Max.Y)/pxPerDp)
}

func compactViewport(platform string, width, height float32) bool {
	if width <= float32(compactLayoutWidth) {
		return true
	}
	return platform == "ios" && min(width, height) <= float32(compactLayoutWidth)
}

func (r *Renderer) pageInset() unit.Dp {
	if !r.compact {
		return r.metrics.pageInset + r.metrics.spaceLarge
	}
	return max(unit.Dp(2), r.metrics.pageInset*.2)
}

type nativeTextStyle struct {
	font       font.Font
	size       unit.Sp
	lineHeight float32
}

func (r *Renderer) typographyRole(role string) string {
	switch role {
	case "execution-row":
		return "control"
	case "":
		return "body"
	}
	if _, ok := r.typography.Roles[role]; ok {
		return role
	}
	return "body"
}

func (r *Renderer) materialTextLabel(text, role string, strong bool) material.LabelStyle {
	typography := r.nativeTextStyle(role, strong)
	label := material.Label(r.theme, typography.size, text)
	label.Font, label.LineHeightScale = typography.font, typography.lineHeight
	return label
}

func (r *Renderer) nativeTextStyle(role string, strong bool) nativeTextStyle {
	role = r.typographyRole(role)
	definition := r.typography.Roles[role]
	weightName := definition.Weight
	if strong {
		weightName = "strong"
	}
	weight := r.typography.Weights[weightName].Native
	return nativeTextStyle{
		font: font.Font{Typeface: font.Typeface(r.typography.Families[definition.Family].Native), Weight: font.Weight(weight - 400)},
		size: unit.Sp(definition.Size), lineHeight: definition.LineHeight,
	}
}

func (r *Renderer) toneColor(tone string) (color.NRGBA, bool) {
	switch tone {
	case "text":
		return r.palette.text, true
	case "muted":
		return r.palette.muted, true
	case "accent":
		return r.palette.accent, true
	case "accent-strong":
		return r.palette.accentStrong, true
	case "pill":
		return r.palette.pillText, true
	case "success":
		return r.palette.success, true
	case "warning":
		return r.palette.warning, true
	case "awaiting":
		return r.palette.awaitingText, true
	case "danger":
		return r.palette.danger, true
	case "focus":
		return r.palette.focus, true
	case "console-text":
		return r.palette.consoleText, true
	case "console-muted":
		return r.palette.consoleMuted, true
	case "console-accent":
		return r.palette.consoleAccent, true
	default:
		return color.NRGBA{}, false
	}
}

func (r *Renderer) layoutIcon(gtx layout.Context, node uidsl.Node, data any) layout.Dimensions {
	if strings.TrimSpace(node.Icon) == "" {
		return r.errorLabel(gtx, fmt.Errorf("icon name is missing"))
	}
	tone := defaultString(node.Style.Tone, "accent")
	if node.Pulse == nil {
		return r.layoutGlyph(gtx, node.Icon, tone, 21)
	}
	value, err := uidsl.Resolve(data, node.Pulse.Binding)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	opacity := heartbeatPulseOpacity(heartbeatUnixMillis(value), gtx.Now)
	if opacity > heartbeatPulseMinimum {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
	}
	semantic.DescriptionOp("Heartbeat").Add(gtx.Ops)
	fade := paint.PushOpacity(gtx.Ops, opacity)
	dimensions := r.layoutGlyph(gtx, node.Icon, tone, 21)
	fade.Pop()
	return dimensions
}

func (r *Renderer) layoutImageSource(gtx layout.Context, source paint.ImageOp, description string, width, height unit.Dp) layout.Dimensions {
	semantic.DescriptionOp(description).Add(gtx.Ops)
	size := gtx.Constraints.Constrain(image.Pt(gtx.Dp(width), gtx.Dp(height)))
	gtx.Constraints = layout.Exact(size)
	return widget.Image{Src: source, Fit: widget.Contain, Position: layout.Center, Scale: 1}.Layout(gtx)
}

func (r *Renderer) layoutGlyph(gtx layout.Context, iconName, tone string, size unit.Dp) layout.Dimensions {
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
	}
	ink, ok := r.toneColor(tone)
	if !ok {
		ink = r.palette.accent
	}
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(size), gtx.Dp(size)))
	if iconName == "loader-2" {
		return r.layoutAnimatedLoader(gtx, ink)
	}
	return icon.Layout(gtx, ink)
}

func (r *Renderer) errorLabel(gtx layout.Context, err error) layout.Dimensions {
	message := "Unknown rendering error"
	if err != nil {
		message = err.Error()
	}
	label := r.materialTextLabel(message, "detail-small", false)
	label.Color = r.palette.danger
	return label.Layout(gtx)
}

func (r *Renderer) buttonNodeState(node *uidsl.Node, data any) (string, bool) {
	label := "Run"
	if node.Text != nil {
		if resolved, err := uidsl.RenderText(data, *node.Text); err == nil {
			label = resolved
		}
	}
	enabled := conditionEnabled(node.Enabled, data)
	r.mu.RLock()
	activeOperations := r.activeOperations
	r.mu.RUnlock()
	if len(activeOperations) > 0 && len(node.Actions) > 0 {
		if arguments, err := actionArguments(node.Actions[0], data); err == nil {
			if fingerprint, err := operations.Fingerprint(node.Actions[0].Command, arguments); err == nil {
				if pending := activeOperations[fingerprint]; pending.ID != "" {
					enabled = false
					if strings.TrimSpace(pending.PendingLabel) != "" {
						label = pending.PendingLabel
					}
					node.Icon = "loader-2"
				}
			}
		}
	}
	return label, enabled
}

type semanticProgress struct {
	state          string
	fraction       float64
	snapshotUnixMS int64
	ratePerMS      float64
}

const (
	progressFrameInterval         = time.Second / 60
	determinateProgressLimit      = .999
	indeterminateProgressDuration = 4 * time.Second
	connectionPulseDuration       = 4 * time.Second
	connectionPulseMinimum        = .58
	heartbeatPulseDuration        = protocol.AgentHeartbeatFadeDuration
	heartbeatPulseMinimum         = .18
	compactLayoutWidth            = unit.Dp(520)
)

func activeSemanticProgress(data any, binding *uidsl.Progress) (semanticProgress, bool) {
	progress, ok := resolveSemanticProgress(data, binding)
	return progress, ok && progress.state != "none" && progress.state != "waiting"
}

func mixColorSRGB(background, foreground color.NRGBA, foregroundWeight float64) color.NRGBA {
	weight := max(0, min(foregroundWeight, 1))
	mix := func(background, foreground uint8) uint8 {
		return uint8(math.Round(float64(background)*(1-weight) + float64(foreground)*weight))
	}
	return color.NRGBA{R: mix(background.R, foreground.R), G: mix(background.G, foreground.G), B: mix(background.B, foreground.B), A: 0xff}
}

func connectionPulseOpacity(now time.Time) float32 {
	cycle := float64(now.UnixNano()%int64(connectionPulseDuration)) / float64(connectionPulseDuration)
	eased := .5 - .5*math.Cos(2*math.Pi*cycle)
	return float32(connectionPulseMinimum + (1-connectionPulseMinimum)*eased)
}

func heartbeatPulseOpacity(lastSeenUnixMS int64, now time.Time) float32 {
	if lastSeenUnixMS <= 0 {
		return heartbeatPulseMinimum
	}
	elapsed := now.Sub(time.UnixMilli(lastSeenUnixMS))
	if elapsed <= 0 {
		return 1
	}
	if elapsed >= heartbeatPulseDuration {
		return heartbeatPulseMinimum
	}
	remaining := 1 - float64(elapsed)/float64(heartbeatPulseDuration)
	return float32(heartbeatPulseMinimum + (1-heartbeatPulseMinimum)*remaining)
}

func heartbeatUnixMillis(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	}
	return 0
}

func resolveSemanticProgress(data any, binding *uidsl.Progress) (semanticProgress, bool) {
	if binding == nil || strings.TrimSpace(binding.Binding) == "" {
		return semanticProgress{}, false
	}
	resolve := func(suffix string) (any, bool) {
		value, err := uidsl.Resolve(data, binding.Binding+"."+suffix)
		return value, err == nil
	}
	stateValue, ok := resolve("state")
	if !ok {
		return semanticProgress{}, false
	}
	progress := semanticProgress{state: strings.ToLower(strings.TrimSpace(fmt.Sprint(stateValue)))}
	if value, ok := resolve("fraction"); ok {
		progress.fraction, _ = strconv.ParseFloat(fmt.Sprint(value), 64)
	}
	if value, ok := resolve("snapshot_unix_ms"); ok {
		progress.snapshotUnixMS, _ = strconv.ParseInt(fmt.Sprint(value), 10, 64)
	}
	if value, ok := resolve("rate_per_ms"); ok {
		progress.ratePerMS, _ = strconv.ParseFloat(fmt.Sprint(value), 64)
	}
	return progress, progress.state != ""
}

func evaluateSemanticProgress(progress semanticProgress, now time.Time) (string, float64) {
	fraction := max(0, min(progress.fraction, 1))
	if progress.state == "determinate" {
		if progress.ratePerMS > 0 {
			elapsed := max(int64(0), now.UnixMilli()-progress.snapshotUnixMS)
			fraction += float64(elapsed) * progress.ratePerMS
		}
		fraction = max(0, min(determinateProgressLimit, fraction))
	}
	return progress.state, fraction
}

func paletteFromTheme(theme uidsl.Theme) (palette, error) {
	get := func(name string) (color.NRGBA, error) { return parseColor(theme.Colors[name]) }
	var p palette
	var err error
	for name, target := range map[string]*color.NRGBA{
		"background": &p.background, "surface": &p.surface, "surface-subtle": &p.subtle,
		"text": &p.text, "text-muted": &p.muted, "accent": &p.accent, "accent-strong": &p.accentStrong,
		"border": &p.border, "success": &p.success, "warning": &p.warning, "danger": &p.danger, "focus": &p.focus,
	} {
		*target, err = get(name)
		if err != nil {
			return palette{}, fmt.Errorf("theme color %s: %w", name, err)
		}
	}
	p.surfaceRaised = p.subtle
	p.pillBackground, p.pillText = p.subtle, p.accentStrong
	p.noticeBackground, p.noticeText, p.noticeBorder = p.surfaceRaised, p.text, p.border
	p.awaitingSurface, p.awaitingBorder, p.awaitingText = p.surfaceRaised, p.warning, p.warning
	for name, target := range map[string]*color.NRGBA{
		"background-glow-a": &p.backgroundGlowA, "background-glow-b": &p.backgroundGlowB,
		"surface-raised": &p.surfaceRaised, "surface-glow": &p.surfaceGlow,
		"pill-background": &p.pillBackground, "pill-text": &p.pillText,
		"notice-background": &p.noticeBackground, "notice-text": &p.noticeText, "notice-border": &p.noticeBorder,
		"awaiting-surface": &p.awaitingSurface, "awaiting-border": &p.awaitingBorder, "awaiting-text": &p.awaitingText,
		"console-background": &p.consoleBackground, "console-surface": &p.consoleSurface,
		"console-border": &p.consoleBorder, "console-text": &p.consoleText,
		"console-muted": &p.consoleMuted, "console-accent": &p.consoleAccent, "console-success": &p.consoleSuccess,
	} {
		value := strings.TrimSpace(theme.Colors[name])
		if value == "" {
			continue
		}
		if *target, err = parseColor(value); err != nil {
			return palette{}, fmt.Errorf("theme color %s: %w", name, err)
		}
	}
	p.pageGradient = nativeGradient{kind: "linear", angle: 145, stops: []nativeGradientStop{
		{color: p.background, position: 0}, {color: p.background, position: 1},
	}}
	p.heroGradient = nativeGradient{kind: "radial", stops: []nativeGradientStop{
		{color: p.surface, position: 0}, {color: p.subtle, position: 1},
	}}
	if gradient, ok := theme.Gradients["page"]; ok {
		if p.pageGradient, err = parseNativeGradient(gradient); err != nil {
			return palette{}, fmt.Errorf("page gradient: %w", err)
		}
	}
	if gradient, ok := theme.Gradients["hero"]; ok {
		if p.heroGradient, err = parseNativeGradient(gradient); err != nil {
			return palette{}, fmt.Errorf("hero gradient: %w", err)
		}
	}
	return p, nil
}

func parseNativeGradient(gradient uidsl.Gradient) (nativeGradient, error) {
	result := nativeGradient{kind: gradient.Kind, angle: float64(gradient.Angle), stops: make([]nativeGradientStop, 0, len(gradient.Stops))}
	for _, stop := range gradient.Stops {
		value, err := parseColor(stop.Color)
		if err != nil {
			return nativeGradient{}, err
		}
		result.stops = append(result.stops, nativeGradientStop{color: value, position: float64(stop.Position) / 100})
	}
	return result, nil
}

func rendererTheme(document *uidsl.ThemeDocument, typography uidsl.Typography) (*material.Theme, palette, error) {
	if document == nil {
		return nil, palette{}, fmt.Errorf("theme is required")
	}
	colors, err := paletteFromTheme(document.Theme)
	if err != nil {
		return nil, palette{}, err
	}
	theme := material.NewTheme()
	fonts, err := ciwiFontCollection()
	if err != nil {
		return nil, palette{}, err
	}
	theme.Shaper = giotext.NewShaper(giotext.WithCollection(fonts))
	theme.Face = font.Typeface(typography.Families["body"].Native)
	theme.Palette.Fg, theme.Palette.Bg = colors.text, colors.background
	theme.Palette.ContrastBg, theme.Palette.ContrastFg = colors.accent, colors.surface
	return theme, colors, nil
}

var (
	ciwiFontsOnce sync.Once
	ciwiFonts     []font.FontFace
	ciwiFontsErr  error
)

func ciwiFontCollection() ([]font.FontFace, error) {
	ciwiFontsOnce.Do(func() {
		ciwiFonts, ciwiFontsErr = loadCiwiFontCollection()
	})
	return ciwiFonts, ciwiFontsErr
}

func loadCiwiFontCollection() ([]font.FontFace, error) {
	collection := append([]font.FontFace(nil), gofont.Collection()...)
	for _, source := range []struct {
		path     string
		typeface font.Typeface
		weight   font.Weight
	}{
		{"assets/GeistSans-Regular.ttf", "Ciwi Sans", font.Normal},
		{"assets/GeistSans-SemiBold.ttf", "Ciwi Sans", font.SemiBold},
		{"assets/GeistSans-Bold.ttf", "Ciwi Sans", font.Bold},
		{"assets/GeistSans-ExtraBold.ttf", "Ciwi Sans", font.ExtraBold},
		{"assets/GeistMono-Regular.ttf", "Ciwi Mono", font.Normal},
		{"assets/GeistMono-Medium.ttf", "Ciwi Mono", font.Medium},
		{"assets/GeistMono-Bold.ttf", "Ciwi Mono", font.Bold},
	} {
		payload, err := sharedUI.Read(source.path)
		if err != nil {
			return nil, fmt.Errorf("load native font: %w", err)
		}
		faces, err := opentype.ParseCollection(payload)
		if err != nil || len(faces) == 0 {
			return nil, fmt.Errorf("parse native font %q", source.path)
		}
		face := faces[0]
		face.Font.Typeface, face.Font.Weight = source.typeface, source.weight
		collection = append(collection, face)
	}
	return collection, nil
}

func embeddedImages() (map[string]paint.ImageOp, error) {
	payload, err := sharedUI.Read("assets/ciwi-logo.png")
	if err != nil {
		return nil, err
	}
	decoded, err := png.Decode(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("decode ciwi logo: %w", err)
	}
	logo := paint.NewImageOp(decoded)
	logo.Filter = paint.FilterNearest
	return map[string]paint.ImageOp{"ciwi-logo": logo}, nil
}

func parseColor(value string) (color.NRGBA, error) {
	value = strings.TrimPrefix(value, "#")
	if len(value) != 6 && len(value) != 8 {
		return color.NRGBA{}, fmt.Errorf("invalid color")
	}
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return color.NRGBA{}, err
	}
	if len(value) == 6 {
		return color.NRGBA{R: byte(parsed >> 16), G: byte(parsed >> 8), B: byte(parsed), A: 0xff}, nil
	}
	return color.NRGBA{R: byte(parsed >> 24), G: byte(parsed >> 16), B: byte(parsed >> 8), A: byte(parsed)}, nil
}

func metricsFromTheme(theme uidsl.Theme, typography uidsl.Typography) visualMetrics {
	value := func(name string, fallback float32) float32 {
		raw := strings.TrimSpace(theme.Dimensions[name])
		parsed, err := strconv.ParseFloat(raw, 32)
		if raw == "" || err != nil || parsed < 0 {
			return fallback
		}
		return float32(parsed)
	}
	const density = float32(.88)
	dense := func(name string, fallback float32) float32 { return value(name, fallback) * density }
	return visualMetrics{
		spaceSmall: unit.Dp(dense("small", 8)), spaceMedium: unit.Dp(dense("medium", 16)), spaceLarge: unit.Dp(dense("large", 24)),
		pageWidth: unit.Dp(value("page", 1150)), pageInset: unit.Dp(value("page-inset", 16)),
		sectionPadding: unit.Dp(value("section-padding", 14)), cardPadding: unit.Dp(value("card-padding", 16)), heroPadding: unit.Dp(value("hero-padding", 16)),
		surfaceRadius: unit.Dp(value("surface-radius", 12)), controlRadius: unit.Dp(value("control-radius", 8)),
		controlPaddingX: unit.Dp(value("control-padding-x", 12)), controlPaddingY: unit.Dp(value("control-padding-y", 8)),
		textBody: typographySize(typography, "body", 16), textControl: typographySize(typography, "control", 14),
		textCode: typographySize(typography, "code", 13), textBadge: typographySize(typography, "badge", 12),
		textSubtitle: typographySize(typography, "subtitle", 16), textHeading: typographySize(typography, "heading", 18),
		textTitle: typographySize(typography, "title", 28), textJobTitle: typographySize(typography, "job-title", 20),
		imageBrandWidth: unit.Dp(value("image-brand-width", 110)), imageBrandHeight: unit.Dp(value("image-brand-height", 91)),
	}
}

func typographySize(typography uidsl.Typography, role string, fallback float32) unit.Sp {
	if style, ok := typography.Roles[role]; ok && style.Size > 0 {
		return unit.Sp(style.Size)
	}
	return unit.Sp(fallback)
}

func (r *Renderer) spacing(value string) unit.Dp {
	switch value {
	case "small":
		return r.metrics.spaceSmall
	case "medium":
		return r.metrics.spaceMedium
	case "large":
		return r.metrics.spaceLarge
	case "section-padding":
		return r.metrics.sectionPadding
	}
	parsed, _ := strconv.ParseFloat(value, 32)
	return unit.Dp(parsed)
}
