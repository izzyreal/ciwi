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
				return r.layoutElement(gtx, child, childIdentity)
			}
			if child.Grow {
				flexed = append(flexed, layout.Flexed(1, widgetFn))
			} else {
				flexed = append(flexed, layout.Rigid(widgetFn))
			}
		}
		return layout.Flex{
			Axis: element.Flex.Axis, Alignment: element.Flex.Alignment,
			Gap: gtx.Dp(element.Flex.Gap),
		}.Layout(gtx, flexed...)
	})
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
	recorded := make([]recordedFlowChild, 0, children.Len())
	x, y, lineHeight, usedWidth := 0, 0, 0, 0
	for index := 0; index < children.Len(); index++ {
		childIdentity, valid := r.childIdentity(identity, children, index)
		if !valid {
			continue
		}
		child := children.At(index)
		childContext := gtx
		childContext.Constraints.Min = image.Point{}
		childContext.Constraints.Max.X = maxWidth
		macro := op.Record(gtx.Ops)
		dimensions := r.layoutElement(childContext, child, childIdentity)
		call := macro.Stop()
		if r.rejectGeometry(childIdentity, dimensions.Size.X, dimensions.Size.Y) {
			continue
		}
		if x > 0 && x+gap+dimensions.Size.X > maxWidth {
			y += lineHeight + gap
			x, lineHeight = 0, 0
		}
		if x > 0 {
			x += gap
		}
		recorded = append(recorded, recordedFlowChild{call: call, size: dimensions.Size, pos: image.Pt(x, y)})
		x += dimensions.Size.X
		lineHeight = max(lineHeight, dimensions.Size.Y)
		usedWidth = max(usedWidth, x)
	}
	height := y + lineHeight
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
	outerColor := props.Fill
	if borderWidth > 0 && props.Border.A != 0 {
		outerColor = props.Border
	}
	paint.FillShape(gtx.Ops, outerColor, clip.UniformRRect(rect, radius).Op(gtx.Ops))
	if borderWidth <= 0 || props.Border.A == 0 {
		return
	}
	inner := rect.Inset(min(borderWidth, min(size.X, size.Y)/2))
	if inner.Empty() {
		return
	}
	innerRadius := max(0, radius-borderWidth)
	offset := op.Offset(inner.Min).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, props.Fill, clip.UniformRRect(image.Rectangle{Max: inner.Size()}, innerRadius).Op(gtx.Ops))
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
	selectable := r.useState(identity, "selectable", KindText, func() any { return new(widget.Selectable) }).(*widget.Selectable)
	label := material.Label(r.theme, size, element.Text.Value)
	label.State = selectable
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
		if props.Indeterminate {
			const cycle = 1600 * time.Millisecond
			phase := float32(gtx.Now.UnixNano()%int64(cycle)) / float32(cycle)
			fraction = .28
			start := int(float32(size.X+size.X/3)*phase) - size.X/3
			progress := image.Rect(max(0, start), 0, min(size.X, start+int(float32(size.X)*fraction)), size.Y)
			if !progress.Empty() {
				paint.FillShape(gtx.Ops, props.Color, clip.Rect(progress).Op())
			}
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 60)})
		} else if width := int(math.Round(float64(size.X) * float64(fraction))); width > 0 {
			paint.FillShape(gtx.Ops, props.Color, clip.Rect(image.Rect(0, 0, min(width, size.X), size.Y)).Op())
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
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutElement(gtx, children.At(1), modalIdentity)
				})
			}),
		)
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, stacked...)
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
