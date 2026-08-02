//go:build darwin

package gio

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

type ActionHandler func(uidsl.Action, map[string]string)

type Renderer struct {
	mu          sync.RWMutex
	screen      *uidsl.ScreenDocument
	data        any
	status      string
	theme       *material.Theme
	palette     palette
	list        layout.List
	buttons     map[string]*widget.Clickable
	disclosures map[string]bool
	statusText  widget.Editor
	shownStatus string
	onAction    ActionHandler
	invalidate  func()
	pending     *pendingConfirmation
}

type pendingConfirmation struct {
	action    uidsl.Action
	arguments map[string]string
	title     string
	message   string
}

type palette struct {
	background, backgroundEnd, surface, subtle, text, muted, accent, border, danger color.NRGBA
}

func NewRenderer(screen *uidsl.ScreenDocument, theme *uidsl.ThemeDocument, onAction ActionHandler) (*Renderer, error) {
	if screen == nil || theme == nil {
		return nil, fmt.Errorf("screen and theme are required")
	}
	colors, err := paletteFromTheme(theme.Theme)
	if err != nil {
		return nil, err
	}
	materialTheme := material.NewTheme()
	materialTheme.Palette.Fg = colors.text
	materialTheme.Palette.Bg = colors.background
	materialTheme.Palette.ContrastBg = colors.accent
	materialTheme.Palette.ContrastFg = colors.surface
	renderer := &Renderer{
		screen: screen, theme: materialTheme, palette: colors, onAction: onAction,
		list: layout.List{Axis: layout.Vertical}, buttons: map[string]*widget.Clickable{}, disclosures: map[string]bool{},
	}
	renderer.statusText.ReadOnly = true
	return renderer, nil
}

func (r *Renderer) SetData(data any) {
	r.mu.Lock()
	r.data = data
	r.mu.Unlock()
}

func (r *Renderer) SetScreenAndData(screen *uidsl.ScreenDocument, data any) {
	r.mu.Lock()
	r.screen = screen
	r.data = data
	r.mu.Unlock()
}

func (r *Renderer) SetStatus(status string) {
	r.mu.Lock()
	r.status = status
	r.mu.Unlock()
}

func (r *Renderer) SetInvalidate(invalidate func()) {
	r.invalidate = invalidate
}

func (r *Renderer) Layout(gtx layout.Context) layout.Dimensions {
	r.mu.RLock()
	screen, data, status := r.screen, r.data, r.status
	r.mu.RUnlock()
	backgroundClip := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	paint.LinearGradientOp{
		Stop1: f32.Pt(0, 0), Color1: r.palette.background,
		Stop2: f32.Pt(float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)), Color2: r.palette.backgroundEnd,
	}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	backgroundClip.Pop()
	if data == nil {
		if status == "" {
			status = "Loading…"
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.layoutStatus(gtx, status)
		})
	}
	root := applyGioOverride(screen.Screen.Root)
	children := root.Children
	body := func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 16, Right: 18, Bottom: 16, Left: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return r.list.Layout(gtx, len(children)+1, func(gtx layout.Context, index int) layout.Dimensions {
				if index == len(children) {
					if status == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: 10, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return r.layoutStatus(gtx, status)
					})
				}
				return layout.Inset{Bottom: spacing(root.Layout.Gap)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutNode(gtx, children[index], data, fmt.Sprintf("%s/root/%d", screen.Metadata.Name, index))
				})
			})
		})
	}
	if r.pending == nil {
		return body(gtx)
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(body),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, color.NRGBA{A: 0x70})
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(r.layoutConfirmation),
	)
}

func (r *Renderer) layoutStatus(gtx layout.Context, status string) layout.Dimensions {
	if status != r.shownStatus {
		r.statusText.SetText(status)
		r.shownStatus = status
	}
	style := material.Editor(r.theme, &r.statusText, "")
	style.TextSize = unit.Sp(14)
	style.Color = r.palette.muted
	return style.Layout(gtx)
}

func (r *Renderer) layoutNode(gtx layout.Context, raw uidsl.Node, data any, path string) layout.Dimensions {
	node := applyGioOverride(raw)
	if override, ok := raw.Overrides["gio"]; ok && override.Hidden {
		return layout.Dimensions{}
	}
	if node.Visible != nil {
		value, err := uidsl.Resolve(data, node.Visible.Binding)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		equal := fmt.Sprint(value) == defaultString(node.Visible.Equals, "true")
		if (!node.Visible.Not && !equal) || (node.Visible.Not && equal) {
			return layout.Dimensions{}
		}
	}
	if node.Repeat != nil {
		items, err := resolveItems(data, node.Repeat.Source)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		children := make([]layout.FlexChild, 0, len(items))
		for i, item := range items {
			itemData := mergeData(data, node.Repeat.As, item)
			clone := node
			clone.Repeat = nil
			itemPath := fmt.Sprintf("%s/%d", path, i)
			if key, err := uidsl.Resolve(itemData, node.Repeat.Key); err == nil {
				itemPath = path + "/" + fmt.Sprint(key)
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: spacing(node.Layout.Gap)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutNode(gtx, clone, itemData, itemPath)
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}

	content := func(gtx layout.Context) layout.Dimensions {
		if node.Component == "disclosure" {
			return r.layoutDisclosure(gtx, node, data, path)
		}
		if node.Component == "text" || node.Component == "badge" || node.Component == "image" {
			return r.layoutText(gtx, node, data)
		}
		if node.Component == "button" {
			return r.layoutButton(gtx, node, data, path)
		}
		return r.layoutChildren(gtx, node, data, path)
	}
	if node.Component == "card" || node.Component == "disclosure" || node.Component == "section" || node.Style.Role == "hero" {
		content = r.surface(content, node.Component == "card" || node.Component == "disclosure")
	}
	if len(node.Actions) > 0 && node.Component != "button" {
		button := r.button(path)
		for button.Clicked(gtx) {
			r.dispatch(node.Actions[0], data)
		}
		return button.Layout(gtx, content)
	}
	return content(gtx)
}

func (r *Renderer) layoutDisclosure(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	label := "Details"
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		label = resolved
	}
	expanded := r.disclosures[path]
	prefix := "▶ "
	if expanded {
		prefix = "▼ "
	}
	toggle := r.button(path + "/disclosure-toggle")
	for toggle.Clicked(gtx) {
		r.disclosures[path] = !expanded
		expanded = !expanded
		r.requestFrame()
	}
	header := material.Button(r.theme, toggle, prefix+label)
	header.CornerRadius = 8
	if !expanded {
		return header.Layout(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutChildren(gtx, node, data, path+"/content")
			})
		}),
	)
}

func (r *Renderer) layoutChildren(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	axis := layout.Vertical
	if node.Component == "row" || node.Layout.Direction == "horizontal" {
		axis = layout.Horizontal
	}
	children := make([]layout.FlexChild, 0, len(node.Children))
	for i := range node.Children {
		child := node.Children[i]
		widgetFn := func(gtx layout.Context) layout.Dimensions {
			return r.layoutNode(gtx, child, data, fmt.Sprintf("%s/%d", path, i))
		}
		if child.Layout.Grow {
			children = append(children, layout.Flexed(1, widgetFn))
		} else {
			children = append(children, layout.Rigid(widgetFn))
		}
	}
	return layout.Inset{
		Top: spacing(node.Layout.Padding), Right: spacing(node.Layout.Padding),
		Bottom: spacing(node.Layout.Padding), Left: spacing(node.Layout.Padding),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: axis, Alignment: layout.Middle, Gap: gtx.Dp(spacing(node.Layout.Gap))}.Layout(gtx, children...)
	})
}

func (r *Renderer) layoutText(gtx layout.Context, node uidsl.Node, data any) layout.Dimensions {
	text := "ciwi"
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		text = resolved
	}
	var label material.LabelStyle
	switch node.Style.Role {
	case "title":
		label = material.H4(r.theme, text)
	case "heading":
		label = material.H6(r.theme, text)
	case "subtitle":
		label = material.Subtitle1(r.theme, text)
	default:
		label = material.Body1(r.theme, text)
	}
	if node.Style.Tone == "muted" {
		label.Color = r.palette.muted
	}
	if node.Style.Emphasis == "strong" {
		label.Font.Weight = font.Bold
	}
	if node.Style.Truncate {
		label.MaxLines = 1
	}
	return label.Layout(gtx)
}

func (r *Renderer) layoutButton(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	label := "Run"
	if node.Text != nil {
		if resolved, err := uidsl.RenderText(data, *node.Text); err == nil {
			label = resolved
		}
	}
	button := r.button(path)
	for button.Clicked(gtx) {
		if len(node.Actions) > 0 {
			r.dispatch(node.Actions[0], data)
		}
	}
	style := material.Button(r.theme, button, label)
	style.CornerRadius = 9
	return style.Layout(gtx)
}

func (r *Renderer) surface(content layout.Widget, card bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		padding := unit.Dp(0)
		if card {
			padding = 14
		}
		return widget.Border{Color: r.palette.border, CornerRadius: 12, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rect := image.Rectangle{Max: gtx.Constraints.Min}
				paint.FillShape(gtx.Ops, r.palette.surface, clip.UniformRRect(rect, gtx.Dp(12)).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(padding).Layout(gtx, content)
			})
		})
	}
}

func (r *Renderer) dispatch(action uidsl.Action, data any) {
	if r.onAction == nil {
		return
	}
	arguments := make(map[string]string, len(action.Arguments))
	for name, expression := range action.Arguments {
		value, err := uidsl.RenderText(data, uidsl.Text{Template: expression})
		if err != nil {
			r.SetStatus(err.Error())
			return
		}
		arguments[name] = value
	}
	if action.Confirm != nil {
		title, err := uidsl.RenderText(data, uidsl.Text{Template: action.Confirm.Title})
		if err != nil {
			r.SetStatus(err.Error())
			return
		}
		message, err := uidsl.RenderText(data, uidsl.Text{Template: action.Confirm.Message})
		if err != nil {
			r.SetStatus(err.Error())
			return
		}
		r.pending = &pendingConfirmation{action: action, arguments: arguments, title: title, message: message}
		r.requestFrame()
		return
	}
	r.onAction(action, arguments)
}

func (r *Renderer) layoutConfirmation(gtx layout.Context) layout.Dimensions {
	pending := r.pending
	if pending == nil {
		return layout.Dimensions{}
	}
	if gtx.Constraints.Max.X > gtx.Dp(560) {
		gtx.Constraints.Max.X = gtx.Dp(560)
	}
	gtx.Constraints.Min = image.Point{}
	confirm := r.button("confirmation/confirm")
	cancel := r.button("confirmation/cancel")
	for cancel.Clicked(gtx) {
		r.pending = nil
		r.requestFrame()
	}
	for confirm.Clicked(gtx) {
		r.pending = nil
		r.onAction(pending.action, pending.arguments)
		r.requestFrame()
	}
	return r.surface(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 22, Right: 22, Bottom: 22, Left: 22}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(material.H6(r.theme, pending.title).Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 12, Bottom: 20}.Layout(gtx, material.Body1(r.theme, pending.message).Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(material.Button(r.theme, cancel, "Cancel").Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 10}.Layout(gtx, material.Button(r.theme, confirm, "Run").Layout)
						}),
					)
				}),
			)
		})
	}, true)(gtx)
}

func (r *Renderer) requestFrame() {
	if r.invalidate != nil {
		r.invalidate()
	}
}

func (r *Renderer) button(key string) *widget.Clickable {
	if button := r.buttons[key]; button != nil {
		return button
	}
	button := new(widget.Clickable)
	r.buttons[key] = button
	return button
}

func (r *Renderer) errorLabel(gtx layout.Context, err error) layout.Dimensions {
	label := material.Body2(r.theme, err.Error())
	label.Color = r.palette.danger
	return label.Layout(gtx)
}

func applyGioOverride(node uidsl.Node) uidsl.Node {
	if override, ok := node.Overrides["gio"]; ok {
		if override.Layout != (uidsl.Layout{}) {
			node.Layout = mergeLayout(node.Layout, override.Layout)
		}
		if override.Style != (uidsl.Style{}) {
			node.Style = mergeStyle(node.Style, override.Style)
		}
	}
	return node
}

func mergeLayout(base, override uidsl.Layout) uidsl.Layout {
	if override.Direction != "" {
		base.Direction = override.Direction
	}
	if override.Gap != "" {
		base.Gap = override.Gap
	}
	if override.Padding != "" {
		base.Padding = override.Padding
	}
	if override.Align != "" {
		base.Align = override.Align
	}
	if override.Justify != "" {
		base.Justify = override.Justify
	}
	if override.MinWidth != "" {
		base.MinWidth = override.MinWidth
	}
	if override.MaxWidth != "" {
		base.MaxWidth = override.MaxWidth
	}
	if override.Wrap {
		base.Wrap = true
	}
	if override.Grow {
		base.Grow = true
	}
	return base
}

func mergeStyle(base, override uidsl.Style) uidsl.Style {
	if override.Role != "" {
		base.Role = override.Role
	}
	if override.Emphasis != "" {
		base.Emphasis = override.Emphasis
	}
	if override.Tone != "" {
		base.Tone = override.Tone
	}
	if override.Truncate {
		base.Truncate = true
	}
	return base
}

func mergeData(root any, name string, value any) map[string]any {
	result := map[string]any{}
	if existing, ok := root.(map[string]any); ok {
		for key, item := range existing {
			result[key] = item
		}
	}
	result[name] = value
	return result
}

func resolveItems(root any, path string) ([]any, error) {
	value, err := uidsl.Resolve(root, path)
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("binding %q is not a list", path)
	}
	return items, nil
}

func paletteFromTheme(theme uidsl.Theme) (palette, error) {
	get := func(name string) (color.NRGBA, error) { return parseColor(theme.Colors[name]) }
	var p palette
	var err error
	for name, target := range map[string]*color.NRGBA{
		"background": &p.background, "surface": &p.surface, "surface-subtle": &p.subtle,
		"text": &p.text, "text-muted": &p.muted, "accent": &p.accent,
		"border": &p.border, "danger": &p.danger,
	} {
		*target, err = get(name)
		if err != nil {
			return palette{}, fmt.Errorf("theme color %s: %w", name, err)
		}
	}
	p.backgroundEnd = p.background
	if gradient, ok := theme.Gradients["page"]; ok && len(gradient.Stops) >= 2 {
		p.background, err = parseColor(gradient.Stops[0].Color)
		if err != nil {
			return palette{}, fmt.Errorf("page gradient start: %w", err)
		}
		p.backgroundEnd, err = parseColor(gradient.Stops[len(gradient.Stops)-1].Color)
		if err != nil {
			return palette{}, fmt.Errorf("page gradient end: %w", err)
		}
	}
	return p, nil
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

func spacing(value string) unit.Dp {
	switch value {
	case "small":
		return 8
	case "medium":
		return 16
	case "large":
		return 28
	}
	if parsed, err := strconv.ParseFloat(value, 32); err == nil {
		return unit.Dp(parsed)
	}
	return 0
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
