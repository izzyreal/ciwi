package giodom

import (
	"fmt"
	"image"
	"math"
	"time"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (r *Runtime) layoutFlex(gtx layout.Context, element Element, identity string) layout.Dimensions {
	children := element.Children
	if children == nil || children.Len() == 0 {
		return applyInsets(element.Flex.Padding, gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		})
	}
	if !r.validateDynamicKeys(identity, children) {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	if element.Flex.Wrap && element.Flex.Axis == layout.Horizontal {
		return applyInsets(element.Flex.Padding, gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutFlow(gtx, element, identity)
		})
	}
	return applyInsets(element.Flex.Padding, gtx, func(gtx layout.Context) layout.Dimensions {
		flexed := make([]layout.FlexChild, 0, children.Len())
		for index := 0; index < children.Len(); index++ {
			child := children.At(index)
			childIdentity, valid := r.childIdentity(identity, children, index)
			if !valid {
				continue
			}
			widgetFn := func(gtx layout.Context) layout.Dimensions {
				if element.Flex.Stretch && element.Flex.Axis == layout.Vertical {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
				}
				return r.layoutElement(gtx, child, childIdentity)
			}
			if weight := flexWeight(child); weight > 0 {
				flexed = append(flexed, layout.Flexed(weight, widgetFn))
			} else {
				flexed = append(flexed, layout.Rigid(widgetFn))
			}
		}
		return layout.Flex{
			Axis: element.Flex.Axis, Alignment: element.Flex.Alignment,
			Spacing: element.Flex.Spacing, Gap: gtx.Dp(element.Flex.Gap),
		}.Layout(gtx, flexed...)
	})
}

func flexWeight(element Element) float32 {
	if element.FlexWeight > 0 {
		return element.FlexWeight
	}
	if element.Grow {
		return 1
	}
	return 0
}

type recordedFlowChild struct {
	call op.CallOp
	size image.Point
	pos  image.Point
}

func (r *Runtime) layoutFlow(gtx layout.Context, element Element, identity string) layout.Dimensions {
	children := element.Children
	gap := gtx.Dp(element.Flex.Gap)
	maxWidth := gtx.Constraints.Max.X
	if maxWidth < 0 || maxWidth > r.maxGeometryPixels {
		r.rejectGeometry(identity, maxWidth)
		return layout.Dimensions{}
	}
	type measuredChild struct {
		element  Element
		identity string
		size     image.Point
	}
	type flowLine struct {
		children []measuredChild
		width    int
	}
	lines := make([]flowLine, 0, 2)
	line := flowLine{}
	finishLine := func() {
		if len(line.children) > 0 {
			lines = append(lines, line)
			line = flowLine{}
		}
	}
	for index := 0; index < children.Len(); index++ {
		childIdentity, valid := r.childIdentity(identity, children, index)
		if !valid {
			continue
		}
		child := children.At(index)
		childContext := gtx
		childContext.Constraints.Min = image.Point{}
		childContext.Constraints.Max.X = maxWidth
		if flexWeight(child) > 0 && child.Kind == KindConstrain && child.Constraint.MinWidth > 0 {
			childContext.Constraints.Max.X = min(maxWidth, max(1, gtx.Dp(child.Constraint.MinWidth)))
		}
		macro := op.Record(gtx.Ops)
		dimensions := r.layoutElement(childContext, child, childIdentity)
		_ = macro.Stop()
		if r.rejectGeometry(childIdentity, dimensions.Size.X, dimensions.Size.Y) {
			continue
		}
		nextWidth := dimensions.Size.X
		if len(line.children) > 0 {
			nextWidth += line.width + gap
		}
		if len(line.children) > 0 && nextWidth > maxWidth {
			finishLine()
			nextWidth = dimensions.Size.X
		}
		line.children = append(line.children, measuredChild{element: child, identity: childIdentity, size: dimensions.Size})
		line.width = nextWidth
	}
	finishLine()

	recorded := make([]recordedFlowChild, 0, children.Len())
	y, usedWidth := 0, 0
	for lineIndex, current := range lines {
		remainingWeight := float32(0)
		for _, child := range current.children {
			remainingWeight += flexWeight(child.element)
		}
		extra := max(0, maxWidth-current.width)
		lineStart := len(recorded)
		x, lineHeight := 0, 0
		for childIndex, child := range current.children {
			if childIndex > 0 {
				x += gap
			}
			allocated := child.size.X
			weight := flexWeight(child.element)
			if weight > 0 && remainingWeight > 0 {
				share := int(math.Round(float64(extra) * float64(weight/remainingWeight)))
				allocated = min(maxWidth-x, allocated+share)
				extra = max(0, extra-share)
				remainingWeight -= weight
			}
			childContext := gtx
			childContext.Constraints.Min = image.Point{}
			childContext.Constraints.Max.X = max(0, maxWidth-x)
			if weight > 0 {
				childContext.Constraints.Min.X = allocated
				childContext.Constraints.Max.X = allocated
			}
			macro := op.Record(gtx.Ops)
			dimensions := r.layoutElement(childContext, child.element, child.identity)
			call := macro.Stop()
			if r.rejectGeometry(child.identity, dimensions.Size.X, dimensions.Size.Y) {
				continue
			}
			recorded = append(recorded, recordedFlowChild{call: call, size: dimensions.Size, pos: image.Pt(x, y)})
			x += dimensions.Size.X
			lineHeight = max(lineHeight, dimensions.Size.Y)
		}
		remaining := max(0, maxWidth-x)
		for index := lineStart; index < len(recorded); index++ {
			recorded[index].pos.X += flowSpacingOffset(element.Flex.Spacing, remaining, index-lineStart, len(recorded)-lineStart)
			recorded[index].pos.Y += flowAlignmentOffset(element.Flex.Alignment, lineHeight-recorded[index].size.Y)
		}
		if element.Flex.Spacing == layout.SpaceEnd {
			usedWidth = max(usedWidth, x)
		} else {
			usedWidth = max(usedWidth, maxWidth)
		}
		y += lineHeight
		if lineIndex < len(lines)-1 {
			y += gap
		}
	}
	height := y
	size := gtx.Constraints.Constrain(image.Pt(usedWidth, height))
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	for _, child := range recorded {
		offset := op.Offset(child.pos).Push(gtx.Ops)
		child.call.Add(gtx.Ops)
		offset.Pop()
	}
	area.Pop()
	return layout.Dimensions{Size: size}
}

func flowAlignmentOffset(alignment layout.Alignment, extra int) int {
	if extra <= 0 {
		return 0
	}
	switch alignment {
	case layout.Middle:
		return extra / 2
	case layout.End:
		return extra
	default:
		return 0
	}
}

func flowSpacingOffset(spacing layout.Spacing, extra, index, count int) int {
	if extra <= 0 || count <= 0 {
		return 0
	}
	switch spacing {
	case layout.SpaceStart:
		return extra
	case layout.SpaceSides:
		return extra / 2
	case layout.SpaceBetween:
		if count > 1 {
			return extra * index / (count - 1)
		}
		return extra / 2
	case layout.SpaceAround:
		return extra * (2*index + 1) / (2 * count)
	case layout.SpaceEvenly:
		return extra * (index + 1) / (count + 1)
	default:
		return 0
	}
}

func (r *Runtime) layoutSurface(gtx layout.Context, element Element, identity string) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		if r.rejectGeometry(identity, size.X, size.Y) || size.X == 0 || size.Y == 0 {
			return layout.Dimensions{Size: size}
		}
		paintSafeSurface(gtx, size, element.Surface)
		return layout.Dimensions{Size: size}
	}, func(gtx layout.Context) layout.Dimensions {
		return applyInsets(element.Surface.Padding, gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutOnlyChild(gtx, element, identity)
		})
	})
}

// paintSafeSurface uses nested fills instead of stroked curves. It therefore
// cannot feed an unbounded curve-flattening workload to Gio's GPU renderer.
func paintSafeSurface(gtx layout.Context, size image.Point, props SurfaceProps) {
	rect := image.Rectangle{Max: size}
	radius := clampRadius(gtx.Dp(props.Radius), size)
	borderWidth := max(0, gtx.Dp(props.BorderWidth))
	hasBorder := borderWidth > 0 && props.Border.A != 0
	if hasBorder {
		paint.FillShape(gtx.Ops, props.Border, clip.UniformRRect(rect, radius).Op(gtx.Ops))
	}
	inner := rect
	if hasBorder {
		inner = rect.Inset(min(borderWidth, min(size.X, size.Y)/2))
	}
	if inner.Empty() {
		return
	}
	innerRadius := radius
	if hasBorder {
		innerRadius = max(0, radius-borderWidth)
	}
	offset := op.Offset(inner.Min).Push(gtx.Ops)
	innerRect := image.Rectangle{Max: inner.Size()}
	if props.PaintBackground == nil {
		paint.FillShape(gtx.Ops, props.Fill, clip.UniformRRect(innerRect, innerRadius).Op(gtx.Ops))
	} else {
		area := clip.UniformRRect(innerRect, innerRadius).Push(gtx.Ops)
		props.PaintBackground(gtx, inner.Size())
		area.Pop()
	}
	offset.Pop()
}

func clampRadius(radius int, size image.Point) int {
	return max(0, min(radius, min(size.X, size.Y)/2))
}

func (r *Runtime) layoutText(gtx layout.Context, element Element, identity string) layout.Dimensions {
	size := element.Text.Size
	if size <= 0 {
		size = unit.Sp(16)
	}
	semantic.LabelOp(element.Text.Value).Add(gtx.Ops)
	label := material.Label(r.theme, size, element.Text.Value)
	if element.Text.Selectable {
		selectable := r.useState(identity, "selectable", KindText, func() any { return new(widget.Selectable) }).(*widget.Selectable)
		label.State = selectable
	}
	if element.Text.Font.Typeface != "" || element.Text.Font.Weight != 0 || element.Text.Font.Style != 0 {
		label.Font = element.Text.Font
	}
	if element.Text.LineHeightScale > 0 {
		label.LineHeightScale = element.Text.LineHeightScale
	}
	label.MaxLines = element.Text.MaxLines
	if element.Text.Color.A != 0 {
		label.Color = element.Text.Color
	}
	return label.Layout(gtx)
}

func (r *Runtime) layoutButton(gtx layout.Context, element Element, identity string) layout.Dimensions {
	clickable := r.useState(identity, "clickable", KindButton, func() any { return new(widget.Clickable) }).(*widget.Clickable)
	for clickable.Clicked(gtx) {
		if element.Button.Enabled && element.Button.OnClick != nil {
			element.Button.OnClick()
		}
	}
	if !element.Button.Enabled {
		gtx = gtx.Disabled()
	}
	if element.Button.Description != "" {
		semantic.DescriptionOp(element.Button.Description).Add(gtx.Ops)
	}
	if element.Children != nil && element.Children.Len() > 0 {
		return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if minimum := gtx.Dp(element.Button.MinHeight); minimum > gtx.Constraints.Min.Y {
				gtx.Constraints.Min.Y = min(minimum, gtx.Constraints.Max.Y)
			}
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				if size.X == 0 || size.Y == 0 || r.rejectGeometry(identity, size.X, size.Y) {
					return layout.Dimensions{Size: size}
				}
				paintSafeSurface(gtx, size, SurfaceProps{
					Fill: element.Button.Fill, Border: element.Button.Border,
					BorderWidth: element.Button.BorderWidth, Radius: element.Button.Radius,
				})
				return layout.Dimensions{Size: size}
			}, func(gtx layout.Context) layout.Dimensions {
				return applyInsets(element.Button.Padding, gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutOnlyChild(gtx, element, identity)
				})
			})
		})
	}
	style := material.Button(r.theme, clickable, element.Button.Label)
	return style.Layout(gtx)
}

type editorState struct {
	editor widget.Editor
}

func (r *Runtime) layoutEditor(gtx layout.Context, element Element, identity string) layout.Dimensions {
	state := r.useState(identity, "editor", KindEditor, func() any { return new(editorState) }).(*editorState)
	state.editor.SingleLine = element.Editor.SingleLine
	if !gtx.Focused(&state.editor) && state.editor.Text() != element.Editor.Value {
		state.editor.SetText(element.Editor.Value)
	}
	changed := false
	for {
		event, ok := state.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.ChangeEvent); ok {
			changed = true
		}
	}
	if changed && element.Editor.OnChange != nil {
		element.Editor.OnChange(state.editor.Text())
	}
	style := material.Editor(r.theme, &state.editor, element.Editor.Placeholder)
	return style.Layout(gtx)
}

func (r *Runtime) layoutSpacer(gtx layout.Context, element Element) layout.Dimensions {
	size := image.Pt(gtx.Dp(element.Spacer.Width), gtx.Dp(element.Spacer.Height))
	return layout.Dimensions{Size: gtx.Constraints.Constrain(size)}
}

func (r *Runtime) layoutResponsive(gtx layout.Context, element Element, identity string) layout.Dimensions {
	breakpoint := gtx.Dp(element.Responsive.Breakpoint)
	child := element.Responsive.Wide
	if gtx.Constraints.Max.X <= breakpoint {
		child = element.Responsive.Compact
	}
	if child == nil {
		r.recordError(fmt.Errorf("%s: responsive variant is missing", identity))
		return layout.Dimensions{}
	}
	childIdentity := identity + "/variant"
	if child.Key != "" {
		childIdentity = identity + "/key:" + identityPart(child.Key)
	}
	return r.layoutElement(gtx, *child, childIdentity)
}

func (r *Runtime) layoutProgress(gtx layout.Context, element Element, identity string) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		if r.rejectGeometry(identity, size.X, size.Y) || size.X == 0 || size.Y == 0 {
			return layout.Dimensions{Size: size}
		}
		props := element.Progress
		paint.FillShape(gtx.Ops, props.Track, clip.UniformRRect(image.Rectangle{Max: size}, clampRadius(gtx.Dp(props.Radius), size)).Op(gtx.Ops))
		fraction := max(float32(0), min(float32(1), props.Fraction))
		progressColor := props.Color
		switch props.Mode {
		case ProgressIndeterminate:
			const cycle = 4 * time.Second
			elapsed := gtx.Now.UnixNano() + int64(props.Phase)
			phase := float64(elapsed%int64(cycle)) / float64(cycle)
			position := .5 - .5*math.Cos(2*math.Pi*phase)
			width := max(1, int(float64(size.X)*.22))
			start := int(float64(max(0, size.X-width)) * position)
			paint.FillShape(gtx.Ops, progressColor, clip.Rect(image.Rect(start, 0, min(size.X, start+width), size.Y)).Op())
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 60)})
		case ProgressOverrun:
			const cycle = 2 * time.Second
			elapsed := gtx.Now.UnixNano() + int64(props.Phase)
			phase := float64(elapsed%int64(cycle)) / float64(cycle)
			opacity := .58 + .42*(.5-.5*math.Cos(2*math.Pi*phase))
			progressColor.A = uint8(math.Round(float64(progressColor.A) * opacity))
			paint.FillShape(gtx.Ops, progressColor, clip.UniformRRect(image.Rectangle{Max: size}, clampRadius(gtx.Dp(props.Radius), size)).Op(gtx.Ops))
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 60)})
		case ProgressComplete:
			paint.FillShape(gtx.Ops, progressColor, clip.UniformRRect(image.Rectangle{Max: size}, clampRadius(gtx.Dp(props.Radius), size)).Op(gtx.Ops))
		default:
			if width := int(math.Round(float64(size.X) * float64(fraction))); width > 0 {
				paint.FillShape(gtx.Ops, progressColor, clip.Rect(image.Rect(0, 0, min(width, size.X), size.Y)).Op())
			}
			if props.Animate {
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 60)})
			}
		}
		return layout.Dimensions{Size: size}
	}, func(gtx layout.Context) layout.Dimensions {
		return r.layoutOnlyChild(gtx, element, identity)
	})
}

func (r *Runtime) layoutOverlay(gtx layout.Context, element Element, identity string) layout.Dimensions {
	children := element.Children
	if children == nil || children.Len() == 0 {
		return layout.Dimensions{}
	}
	bodyIdentity, _ := r.childIdentity(identity, children, 0)
	viewport := gtx.Constraints
	stacked := []layout.StackChild{layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = viewport
		return r.layoutElement(gtx, children.At(0), bodyIdentity)
	})}
	if children.Len() > 1 {
		modalIdentity, _ := r.childIdentity(identity, children, 1)
		stacked = append(stacked,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				paint.Fill(gtx.Ops, element.Overlay.Scrim)
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				alignment := layout.Center
				if element.Overlay.Align {
					alignment = element.Overlay.Alignment
				}
				return alignment.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutElement(gtx, children.At(1), modalIdentity)
				})
			}),
		)
	}
	alignment := layout.Center
	if element.Overlay.Align {
		alignment = element.Overlay.Alignment
	}
	return layout.Stack{Alignment: alignment}.Layout(gtx, stacked...)
}

func (r *Runtime) layoutConstrain(gtx layout.Context, element Element, identity string) layout.Dimensions {
	props := element.Constraint
	if props.MinWidth > 0 {
		gtx.Constraints.Min.X = max(gtx.Constraints.Min.X, min(gtx.Constraints.Max.X, gtx.Dp(props.MinWidth)))
	}
	if props.MinHeight > 0 {
		gtx.Constraints.Min.Y = max(gtx.Constraints.Min.Y, min(gtx.Constraints.Max.Y, gtx.Dp(props.MinHeight)))
	}
	if props.MaxWidth > 0 {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(props.MaxWidth))
		gtx.Constraints.Min.X = min(gtx.Constraints.Min.X, gtx.Constraints.Max.X)
	}
	if props.MaxHeight > 0 {
		gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(props.MaxHeight))
		gtx.Constraints.Min.Y = min(gtx.Constraints.Min.Y, gtx.Constraints.Max.Y)
	}
	if r.rejectGeometry(identity, gtx.Constraints.Min.X, gtx.Constraints.Min.Y, gtx.Constraints.Max.X, gtx.Constraints.Max.Y) {
		return layout.Dimensions{}
	}
	return r.layoutOnlyChild(gtx, element, identity)
}

func (r *Runtime) layoutOnlyChild(gtx layout.Context, element Element, identity string) layout.Dimensions {
	if element.Children == nil || element.Children.Len() == 0 {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	childIdentity, valid := r.childIdentity(identity, element.Children, 0)
	if !valid {
		return layout.Dimensions{}
	}
	return r.layoutElement(gtx, element.Children.At(0), childIdentity)
}

func applyInsets(insets Insets, gtx layout.Context, content layout.Widget) layout.Dimensions {
	return layout.Inset{
		Top: insets.Top, Right: insets.Right, Bottom: insets.Bottom, Left: insets.Left,
	}.Layout(gtx, content)
}
