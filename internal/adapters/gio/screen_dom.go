//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

const domSemanticStateLimit = 4096

type screenDOMRenderer struct {
	theme   *material.Theme
	runtime *giodom.Runtime
}

type domButtonState struct {
	clickable widget.Clickable
}

type domEditorState struct {
	editor widget.Editor
}

type domImageState struct {
	encoded string
	source  paint.ImageOp
}

type domSelectState struct {
	toggle  widget.Clickable
	dismiss widget.Clickable
	options map[string]*widget.Clickable
	list    layout.List
	open    bool
}

func (r *Renderer) layoutScreenDOMFrame(gtx layout.Context, screen *uidsl.ScreenDocument, data any, pendingScrollSection string, notice *nativeNotice, alert *nativeAlert) layout.Dimensions {
	if r.dom == nil || r.dom.theme != r.theme {
		r.dom = &screenDOMRenderer{
			theme: r.theme,
			runtime: giodom.NewRuntime(r.theme, giodom.Options{
				MaxStateSlots: 8192,
			}),
		}
	}
	document := r.buildScreenDOM(screen, data, pendingScrollSection)
	document = r.decorateDOMOverlays(document, notice, alert)
	return r.dom.runtime.Layout(gtx, document)
}

func (r *Renderer) decorateDOMOverlays(document giodom.Element, notice *nativeNotice, alert *nativeAlert) giodom.Element {
	if notice != nil {
		document = giodom.Overlay("notice-overlay", giodom.OverlayProps{Alignment: layout.SE, Align: true}, document, r.domNotice(notice))
	}
	if alert != nil {
		document = giodom.Overlay("alert-overlay", giodom.OverlayProps{Scrim: color.NRGBA{A: 0x70}}, document, r.domAlert(alert))
	}
	if r.pending != nil {
		document = giodom.Overlay("confirmation-overlay", giodom.OverlayProps{Scrim: color.NRGBA{A: 0x70}}, document, r.domConfirmation(r.pending))
	}
	return document
}

func (r *Renderer) domNotice(notice *nativeNotice) giodom.Element {
	buttons := []giodom.Element{}
	if strings.TrimSpace(notice.actionLabel) != "" {
		buttons = append(buttons, r.domCallbackButton("notice-action", notice.actionLabel, "", func() {
			r.dismissNotice()
			if r.onAction != nil && strings.TrimSpace(notice.action.Command) != "" {
				r.onAction(notice.action, cloneStringMap(notice.arguments))
			}
		}))
	}
	buttons = append(buttons, r.domCallbackButton("notice-dismiss", "Dismiss", "", r.dismissNotice))
	wide := giodom.Row("notice-wide", r.metrics.spaceMedium,
		giodom.Element{Kind: giodom.KindText, Key: "notice-message", Grow: true, Text: giodom.TextProps{Value: notice.message, Size: r.metrics.textBody, Color: r.palette.noticeText, Selectable: true}},
		giodom.Flow("notice-actions", r.metrics.spaceSmall, buttons...),
	)
	compact := giodom.Column("notice-compact", r.metrics.spaceSmall,
		giodom.Text("notice-message", notice.message, r.metrics.textBody, r.palette.noticeText),
		giodom.Flow("notice-actions", r.metrics.spaceSmall, buttons...),
	)
	content := giodom.Responsive("notice-responsive", 520, compact, wide)
	card := giodom.Surface("notice-card", giodom.SurfaceProps{
		Fill: r.palette.noticeBackground, Border: r.palette.noticeBorder, BorderWidth: 1,
		Radius: r.metrics.surfaceRadius, Padding: giodom.UniformInsets(r.metrics.spaceMedium),
	}, content)
	card = giodom.Constrain("notice-width", giodom.ConstraintProps{MaxWidth: 480}, card)
	return giodom.Inset("notice-margin", giodom.Insets{Right: 14, Bottom: 14, Left: 14}, card)
}

func (r *Renderer) domAlert(alert *nativeAlert) giodom.Element {
	content := giodom.Column("alert-copy", r.metrics.spaceMedium,
		r.domText("alert-title", defaultString(alert.title, "Something went wrong"), "heading", true, "danger", true),
		r.domText("alert-message", alert.message, "body", false, "", true),
		r.domCallbackButton("alert-dismiss", "Dismiss", "", func() {
			r.mu.Lock()
			r.alert = nil
			r.mu.Unlock()
			r.requestFrame()
		}),
	)
	return r.domModalCard("alert", content)
}

func (r *Renderer) domConfirmation(pending *pendingConfirmation) giodom.Element {
	buttons := giodom.Flow("confirmation-actions", r.metrics.spaceSmall,
		r.domCallbackButton("confirmation-confirm", "Confirm", "check", func() {
			r.mu.Lock()
			if r.pending == pending {
				r.pending = nil
			}
			r.mu.Unlock()
			if r.onAction != nil {
				r.onAction(pending.action, cloneStringMap(pending.arguments))
			}
			r.requestFrame()
		}),
		r.domCallbackButton("confirmation-cancel", "Cancel", "", func() {
			r.mu.Lock()
			if r.pending == pending {
				r.pending = nil
			}
			r.mu.Unlock()
			r.requestFrame()
		}),
	)
	content := giodom.Column("confirmation-copy", r.metrics.spaceMedium,
		r.domText("confirmation-title", pending.title, "heading", true, "", true),
		r.domText("confirmation-message", pending.message, "body", false, "", true),
		buttons,
	)
	return r.domModalCard("confirmation", content)
}

func (r *Renderer) domModalCard(key string, content giodom.Element) giodom.Element {
	card := giodom.Surface(giodom.Key(key+"-card"), giodom.SurfaceProps{
		Fill: r.palette.surface, Border: r.palette.border, BorderWidth: 1,
		Radius: r.metrics.surfaceRadius, Padding: giodom.UniformInsets(r.metrics.sectionPadding),
	}, content)
	return giodom.Constrain(giodom.Key(key+"-width"), giodom.ConstraintProps{MinWidth: 280, MaxWidth: 540}, card)
}

func (r *Renderer) domCallbackButton(key, label, icon string, callback func()) giodom.Element {
	return giodom.Native(giodom.Key(key), giodom.NativeProps{
		NewState: func() any { return new(domButtonState) },
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			state := raw.(*domButtonState)
			for state.clickable.Clicked(gtx) {
				if callback != nil {
					callback()
				}
			}
			return r.layoutDOMControl(gtx, &state.clickable, label, icon, "", "accent")
		},
	})
}

func (r *Renderer) buildScreenDOM(screen *uidsl.ScreenDocument, data any, pendingScrollSection string) giodom.Element {
	if screen == nil {
		return r.domMessage("missing-screen", "Screen unavailable", r.palette.danger)
	}
	root, hidden := applyGioOverride(screen.Screen.Root, r.compact)
	if hidden {
		return r.domMessage("hidden-screen", "", r.palette.text)
	}
	children := make([]giodom.Element, 0, len(root.Children))
	scrollTarget := giodom.Key("")
	for index := range root.Children {
		path := fmt.Sprintf("%s/root/%d", screen.Metadata.Name, index)
		compiled := r.compileDOMNode(root.Children[index], data, path)
		if compiled == nil {
			continue
		}
		key := domNodeKey(root.Children[index], path)
		compiled.Key = key
		children = append(children, *compiled)
		if pendingScrollSection != "" && root.Children[index].ID == pendingScrollSection {
			scrollTarget = key
		}
	}
	props := giodom.ListProps{
		Axis: layout.Vertical, Gap: r.spacing(root.Layout.Gap), Estimate: 160,
		Overscan: 2, MaxMeasured: 1024, SemanticLabel: screen.Metadata.Title,
		ScrollTo: scrollTarget,
	}
	if scrollTarget != "" {
		props.ScrollRevision = uint64(time.Now().UnixNano())
		r.mu.Lock()
		if r.pendingScrollSection == pendingScrollSection {
			r.pendingScrollSection = ""
		}
		r.mu.Unlock()
	}
	page := giodom.VirtualList(giodom.Key("page-list:"+screen.Metadata.Name), props, giodom.Keyed(domElementsRevision(children), children...))
	pageInset := r.pageInset()
	page = giodom.Inset(giodom.Key("page-inset:"+screen.Metadata.Name), giodom.Insets{
		Top: pageInset, Right: pageInset, Bottom: pageInset, Left: pageInset,
	}, page)
	if r.metrics.pageWidth > 0 {
		page = giodom.Constrain(giodom.Key("page-width:"+screen.Metadata.Name), giodom.ConstraintProps{MaxWidth: r.metrics.pageWidth}, page)
		page = giodom.Align(giodom.Key("page-center:"+screen.Metadata.Name), layout.N, page)
	}
	return page
}

func (r *Renderer) compileDOMNode(raw uidsl.Node, data any, path string) *giodom.Element {
	node, hidden := applyGioOverride(raw, r.compact)
	if hidden || !domNodeVisible(node, data) {
		return nil
	}
	if node.Component == "scroller" {
		return r.compileDOMScroller(node, data, path)
	}
	if node.Repeat != nil {
		return r.compileDOMRepeat(node, data, path)
	}
	if node.Style.ToneBinding != "" {
		if value, err := uidsl.Resolve(data, node.Style.ToneBinding); err == nil {
			node.Style.Tone = semanticTone(fmt.Sprint(value))
		}
	}

	var element giodom.Element
	switch node.Component {
	case "page", "column", "row", "list", "section", "card":
		element = r.compileDOMContainer(node, data, path)
	case "disclosure":
		element = r.compileDOMDisclosure(node, data, path)
	case "tree-view":
		element = r.compileDOMTree(node, data, path)
	case "graph-view":
		element = r.compileDOMGraph(node, data, path)
	case "text":
		element = r.compileDOMText(node, data, path)
	case "badge":
		element = r.compileDOMBadge(node, data, path)
	case "button":
		element = r.compileDOMButton(node, data, path)
	case "input":
		element = r.compileDOMInput(node, data, path)
	case "select":
		element = r.compileDOMSelect(node, data, path)
	case "image":
		element = r.compileDOMImage(node, data, path)
	case "icon":
		element = r.compileDOMIcon(node, data, path)
	case "spacer":
		element = r.compileDOMSpacer(node, path)
	case "divider":
		element = giodom.Surface(domNodeKey(node, path), giodom.SurfaceProps{Fill: r.palette.border}, giodom.Spacer("line", 0, 1))
	default:
		element = r.domMessage(giodom.Key(path+"/unsupported"), "Unsupported component: "+node.Component, r.palette.danger)
	}
	element.Key = domNodeKey(node, path)
	return r.decorateDOMNode(element, node, data, path)
}

func (r *Renderer) compileDOMRepeat(node uidsl.Node, data any, path string) *giodom.Element {
	items, err := resolveItems(data, node.Repeat.Source)
	if err != nil {
		element := r.domError(path, err)
		return &element
	}
	repeat := node.Repeat
	clone := node
	clone.Repeat = nil
	elements := make([]giodom.Element, 0, len(items))
	for index, item := range items {
		itemData := mergeData(data, repeat.As, item)
		key := fmt.Sprintf("%d", index)
		if value, resolveErr := uidsl.Resolve(itemData, repeat.Key); resolveErr == nil {
			key = fmt.Sprint(value)
		}
		compiled := r.compileDOMNode(clone, itemData, path+"/"+key)
		if compiled == nil {
			continue
		}
		compiled.Key = giodom.Key(key)
		elements = append(elements, *compiled)
	}
	axis := layout.Vertical
	if node.Layout.Direction == "horizontal" {
		axis = layout.Horizontal
	}
	result := giodom.Element{
		Kind: giodom.KindFlex, Key: domNodeKey(node, path),
		Flex:     giodom.FlexProps{Axis: axis, Alignment: flexAlignment(axis, node.Layout.Align, false), Gap: r.spacing(node.Layout.Gap), Wrap: node.Layout.Wrap},
		Children: giodom.Keyed(domElementsRevision(elements), elements...),
	}
	return r.decorateDOMNode(result, node, data, path)
}

func (r *Renderer) compileDOMContainer(node uidsl.Node, data any, path string) giodom.Element {
	axis := layout.Vertical
	if node.Component == "row" || node.Layout.Direction == "horizontal" {
		axis = layout.Horizontal
	}
	stackedCompactRow := r.compact && axis == layout.Horizontal && (node.Style.Role == "hero" || node.Layout.Wrap || compactRowNeedsStack(node.Children)) && node.Style.Role != "compact-toolbar"
	if stackedCompactRow {
		axis = layout.Vertical
	}
	if r.compact && (node.Style.Role == "queued-execution-header" || node.Style.Role == "history-execution-header" || node.Style.Role == "agent-header") {
		return giodom.Spacer(domNodeKey(node, path), 0, 0)
	}
	children := r.compileDOMChildren(node.Children, data, path)
	if stackedCompactRow {
		for index := range children {
			children[index].Grow = false
		}
	}
	if r.compact && (node.Style.Role == "queued-execution-job-row" || node.Style.Role == "history-execution-job-row" || node.Style.Role == "agent-record") {
		children = r.compactDOMRecordChildren(node, data, path)
		axis = layout.Vertical
	}
	props := giodom.FlexProps{
		Axis: axis, Alignment: flexAlignment(axis, node.Layout.Align, false),
		Gap: r.spacing(node.Layout.Gap), Padding: giodom.UniformInsets(r.spacing(node.Layout.Padding)),
		Wrap: axis == layout.Horizontal && (node.Layout.Wrap || node.Style.Role == "hero"),
	}
	return giodom.Element{Kind: giodom.KindFlex, Key: domNodeKey(node, path), Flex: props, Children: giodom.Static(children...)}
}

func (r *Renderer) compileDOMChildren(nodes []uidsl.Node, data any, path string) []giodom.Element {
	children := make([]giodom.Element, 0, len(nodes))
	for index := range nodes {
		compiled := r.compileDOMNode(nodes[index], data, fmt.Sprintf("%s/%d", path, index))
		if compiled == nil {
			continue
		}
		compiled.Grow = nodes[index].Layout.Grow
		children = append(children, *compiled)
	}
	return children
}

func (r *Renderer) compactDOMRecordChildren(node uidsl.Node, data any, path string) []giodom.Element {
	labels := []string{"Job", "Status", "Pipeline", "Build", "Agent", "Created", "Reason", "Actions"}
	if node.Style.Role == "history-execution-job-row" {
		labels = []string{"Job", "Status", "Pipeline", "Build", "Agent", "Created", "Duration"}
	} else if node.Style.Role == "agent-record" {
		labels = []string{"Agent ID", "Host", "Platform", "Version", "Heartbeat", "Health", "Run mode"}
	}
	rows := make([]giodom.Element, 0, len(node.Children))
	for index := range node.Children {
		if index >= len(labels) || !compactNodeHasContent(node.Children[index], data) {
			continue
		}
		value := r.compileDOMNode(node.Children[index], data, fmt.Sprintf("%s/%d", path, index))
		if value == nil {
			continue
		}
		label := r.domText(giodom.Key(fmt.Sprintf("%s/label/%d", path, index)), labels[index], "table-header", false, "muted", true)
		label = giodom.Constrain(label.Key, giodom.ConstraintProps{MinWidth: 82}, label)
		value.Grow = true
		rows = append(rows, giodom.Row(giodom.Key(fmt.Sprintf("%s/row/%d", path, index)), r.metrics.spaceSmall, label, *value))
	}
	return rows
}

func domNodeVisible(node uidsl.Node, data any) bool {
	if node.Visible == nil {
		return true
	}
	value, err := uidsl.Resolve(data, node.Visible.Binding)
	if err != nil {
		return true
	}
	equal := conditionEqual(node.Visible, value)
	return (node.Visible.Not && !equal) || (!node.Visible.Not && equal)
}

func domNodeKey(node uidsl.Node, path string) giodom.Key {
	if node.ID != "" {
		return giodom.Key(node.ID)
	}
	return giodom.Key(path)
}

func domElementsRevision(elements []giodom.Element) uint64 {
	const offset = uint64(1469598103934665603)
	const prime = uint64(1099511628211)
	hash := offset
	for _, element := range elements {
		for _, value := range []byte(element.Key) {
			hash ^= uint64(value)
			hash *= prime
		}
		hash ^= uint64(element.Kind)
		hash *= prime
	}
	return hash
}

func (r *Renderer) decorateDOMNode(element giodom.Element, node uidsl.Node, data any, path string) *giodom.Element {
	if node.Style.Role == "skeleton" {
		fill := r.palette.border
		fill.A = 0x80
		element = giodom.Surface(domNodeKey(node, path), giodom.SurfaceProps{Fill: fill, Radius: r.metrics.controlRadius}, element)
	}
	if node.Progress != nil {
		if progress, active := activeSemanticProgress(data, node.Progress); active {
			state, fraction := evaluateSemanticProgress(progress, time.Now())
			fill := r.palette.success
			track := color.NRGBA{}
			if node.Style.Role == "output-group" {
				fill = r.palette.consoleSuccess
				track = r.palette.consoleSurface
			}
			fill.A = 0x38
			element = giodom.Progress(giodom.Key(path+"/progress"), giodom.ProgressProps{
				Fraction: float32(fraction), Indeterminate: state == "indeterminate",
				Color: fill, Track: track, Radius: r.metrics.surfaceRadius,
			}, element)
		}
	}

	isSurface := node.Component == "card" || node.Component == "section" || node.Component == "disclosure" || node.Component == "graph-view" || node.Style.Role == "hero"
	if isSurface && !(node.Component == "disclosure" && node.Style.Role == "tree-branch") {
		props := giodom.SurfaceProps{
			Fill: r.palette.surface, Border: r.palette.border, BorderWidth: 1,
			Radius: r.metrics.surfaceRadius,
		}
		switch node.Component {
		case "section", "graph-view", "disclosure":
			props.Padding = giodom.UniformInsets(r.metrics.sectionPadding)
		case "card":
			props.Padding = giodom.UniformInsets(r.metrics.cardPadding)
		}
		if node.Layout.Padding != "" {
			props.Padding = giodom.Insets{}
		}
		switch {
		case node.Style.Role == "hero":
			props.Fill = r.palette.heroStart
			props.Padding = giodom.UniformInsets(r.metrics.heroPadding)
		case node.Component == "disclosure" && node.Style.Role == "output-group":
			props.Fill, props.Border = r.palette.consoleSurface, r.palette.consoleBorder
		case node.Component == "disclosure":
			props.Fill, props.BorderWidth = r.palette.surfaceRaised, 0
		case node.Component == "card" && node.Style.Role == "output-system":
			props.Fill, props.Border = r.palette.consoleSurface, r.palette.consoleBorder
		case node.Component == "card" && node.Style.Role == "scheduling-awaiting":
			props.Fill, props.Border = r.palette.awaitingSurface, r.palette.awaitingBorder
		}
		element = giodom.Surface(giodom.Key(path+"/surface"), props, element)
	}
	if node.Style.Role == "queued-execution-job-row" || node.Style.Role == "history-execution-job-row" {
		element = giodom.Surface(giodom.Key(path+"/row-surface"), giodom.SurfaceProps{
			Fill: r.palette.surfaceRaised, Border: r.palette.border, BorderWidth: 1,
			Radius: r.metrics.controlRadius, Padding: giodom.Insets{Top: 7, Right: r.metrics.spaceSmall, Bottom: 7, Left: r.metrics.spaceSmall},
		}, element)
	}
	if len(node.Actions) > 0 && !componentHandlesOwnActions(node.Component) {
		action := node.Actions[0]
		element = giodom.Control(giodom.Key(path+"/activate"), giodom.ButtonProps{
			Enabled: conditionEnabled(node.Enabled, data), Description: domActionDescription(action),
			OnClick: func() { r.dispatch(action, data) },
		}, element)
	}
	if constraint, ok := r.domConstraint(node.Layout); ok {
		element = giodom.Constrain(giodom.Key(path+"/constraint"), constraint, element)
	}
	element.Key = domNodeKey(node, path)
	return &element
}

func (r *Renderer) domConstraint(values uidsl.Layout) (giodom.ConstraintProps, bool) {
	parse := func(value string) unit.Dp {
		value = strings.TrimSpace(value)
		if value == "page" {
			return r.metrics.pageWidth
		}
		parsed, err := strconv.ParseFloat(value, 32)
		if err != nil || parsed <= 0 {
			return 0
		}
		return unit.Dp(parsed)
	}
	props := giodom.ConstraintProps{
		MinWidth: parse(values.MinWidth), MaxWidth: parse(values.MaxWidth),
		MinHeight: parse(values.MinHeight), MaxHeight: parse(values.MaxHeight),
	}
	return props, props != (giodom.ConstraintProps{})
}

func (r *Renderer) compileDOMText(node uidsl.Node, data any, path string) giodom.Element {
	value := ""
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.domError(path, err)
		}
		value = resolved
	}
	role := r.typographyRole(node.Style.Role)
	strong := node.Style.Emphasis == "strong"
	if role == "code" || role == "code-inline" || role == "output-code" {
		return r.compileDOMCodeText(node, data, path, value, role, strong)
	}
	text := r.domText(domNodeKey(node, path), value, role, strong, node.Style.Tone, true)
	if node.Style.Truncate || role == "badge" || role == "table-header" || node.Style.Role == "execution-row" {
		text.Text.MaxLines = 1
	}
	if len(node.Actions) == 0 {
		return text
	}
	return r.domTextAction(node, data, path, value, role, strong)
}

func (r *Renderer) compileDOMCodeText(node uidsl.Node, data any, path, value, role string, strong bool) giodom.Element {
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		NewState: func() any {
			state := &domEditorState{}
			state.editor.ReadOnly = true
			return state
		},
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			state := raw.(*domEditorState)
			state.editor.ReadOnly = true
			state.editor.SingleLine = role == "code-inline" && node.Style.Truncate
			if state.editor.Text() != value {
				state.editor.SetText(value)
			}
			outputID := ""
			if node.ID == "job-output-group-text" {
				if resolved, err := uidsl.Resolve(data, "outputGroup.id"); err == nil {
					outputID = fmt.Sprint(resolved)
					r.outputEditors[outputID] = &state.editor
				}
			} else if node.ID == "job-output-system-text" {
				r.outputEditors[""] = &state.editor
			}
			if pending := r.pendingOutputSelection; pending != nil && pending.itemID == outputID {
				state.editor.SetCaret(pending.start, pending.end)
				r.pendingOutputSelection = nil
			}
			typography := r.nativeTextStyle(role, strong)
			style := material.Editor(r.theme, &state.editor, "")
			style.Font, style.TextSize, style.LineHeightScale = typography.font, typography.size, typography.lineHeight
			style.Color = r.domTextColor(role, node.Style.Tone)
			style.SelectionColor = r.palette.focus
			style.SelectionColor.A = 0xc0
			if role == "code" {
				return layout.UniformInset(12).Layout(gtx, style.Layout)
			}
			return style.Layout(gtx)
		},
	})
}

func (r *Renderer) domText(key giodom.Key, value, role string, strong bool, tone string, selectable bool) giodom.Element {
	style := r.nativeTextStyle(role, strong)
	return giodom.Element{
		Kind: giodom.KindText, Key: key,
		Text: giodom.TextProps{
			Value: value, Size: style.size, Color: r.domTextColor(role, tone),
			Weight: style.font.Weight, Selectable: selectable,
		},
	}
}

func (r *Renderer) domTextColor(role, tone string) color.NRGBA {
	if resolved, ok := r.toneColor(tone); ok {
		return resolved
	}
	switch role {
	case "table-header", "detail", "detail-small":
		return r.palette.muted
	case "output-code", "output-meta":
		return r.palette.consoleText
	case "output-label":
		return r.palette.consoleAccent
	case "link":
		return r.palette.accent
	default:
		return r.palette.text
	}
}

func (r *Renderer) domTextAction(node uidsl.Node, data any, path, value, role string, strong bool) giodom.Element {
	action := node.Actions[0]
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		NewState: func() any { return new(domButtonState) },
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			state := raw.(*domButtonState)
			for state.clickable.Clicked(gtx) {
				r.dispatchFromLayout(gtx, action, data)
			}
			return state.clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := r.materialTextLabel(value, role, strong)
				label.Color = r.domTextColor(role, node.Style.Tone)
				return label.Layout(gtx)
			})
		},
	})
}

func (r *Renderer) compileDOMBadge(node uidsl.Node, data any, path string) giodom.Element {
	value := ""
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.domError(path, err)
		}
		value = resolved
	}
	tone, ok := r.toneColor(node.Style.Tone)
	if !ok {
		tone = r.palette.accent
	}
	fill, border, borderWidth, textTone := tone, tone, unit.Dp(1), node.Style.Tone
	if node.Style.Tone == "muted" {
		fill, border, borderWidth, textTone = r.palette.pillBackground, color.NRGBA{}, 0, "pill"
		if node.Style.Emphasis == "strong" {
			border, borderWidth = r.palette.border, 1
		}
	} else {
		fill.A, border.A = 0x24, 0x90
	}
	text := r.domText(giodom.Key(path+"/text"), value, "badge", node.Style.Emphasis == "strong", textTone, true)
	return giodom.Surface(domNodeKey(node, path), giodom.SurfaceProps{
		Fill: fill, Border: border, BorderWidth: borderWidth, Radius: 100,
		Padding: giodom.Insets{Top: 2, Right: 8, Bottom: 2, Left: 8},
	}, text)
}

func (r *Renderer) compileDOMButton(node uidsl.Node, data any, path string) giodom.Element {
	label, enabled := r.buttonNodeState(&node, data)
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		NewState: func() any { return new(domButtonState) },
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			state := raw.(*domButtonState)
			for state.clickable.Clicked(gtx) {
				if enabled && len(node.Actions) > 0 {
					r.dispatchFromLayout(gtx, node.Actions[0], data)
				}
			}
			if !enabled {
				gtx = gtx.Disabled()
			}
			if node.Style.Role == "connection-pulse" {
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
				fade := paint.PushOpacity(gtx.Ops, connectionPulseOpacity(gtx.Now))
				dimensions := r.layoutDOMControl(gtx, &state.clickable, label, node.Icon, node.Style.Role, node.Style.Tone)
				fade.Pop()
				return dimensions
			}
			return r.layoutDOMControl(gtx, &state.clickable, label, node.Icon, node.Style.Role, node.Style.Tone)
		},
	})
}

func (r *Renderer) layoutDOMControl(gtx layout.Context, clickable *widget.Clickable, label, iconName, role, tone string) layout.Dimensions {
	iconOnly := role == "icon-button"
	semantic.DescriptionOp(label).Add(gtx.Ops)
	return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if !iconOnly {
			gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, gtx.Dp(38)))
		}
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paintDOMSurface(gtx, size, r.palette.subtle, r.palette.border, gtx.Dp(1), gtx.Dp(r.metrics.controlRadius))
			return layout.Dimensions{Size: size}
		}, func(gtx layout.Context) layout.Dimensions {
			paddingX, paddingY := r.metrics.controlPaddingX, r.metrics.controlPaddingY
			if iconOnly {
				paddingX, paddingY = 10, 10
			}
			return layout.Inset{Top: paddingY, Right: paddingX, Bottom: paddingY, Left: paddingX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, 2)
				if iconName != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return r.layoutGlyph(gtx, iconName, defaultString(tone, "accent"), 20)
					}))
				}
				if !iconOnly {
					if len(children) > 0 {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Spacer{Width: r.metrics.spaceSmall}.Layout(gtx)
						}))
					}
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						labelStyle := r.materialTextLabel(label, "control", false)
						labelStyle.Color = r.palette.accentStrong
						return labelStyle.Layout(gtx)
					}))
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
		})
	})
}

func paintDOMSurface(gtx layout.Context, size image.Point, fill, border color.NRGBA, borderWidth, radius int) {
	if size.X <= 0 || size.Y <= 0 {
		return
	}
	radius = max(0, min(radius, min(size.X, size.Y)/2))
	rect := image.Rectangle{Max: size}
	outer := fill
	if borderWidth > 0 && border.A != 0 {
		outer = border
	}
	paint.FillShape(gtx.Ops, outer, clip.UniformRRect(rect, radius).Op(gtx.Ops))
	if borderWidth <= 0 || border.A == 0 {
		return
	}
	inner := rect.Inset(min(borderWidth, min(size.X, size.Y)/2))
	if inner.Empty() {
		return
	}
	offset := op.Offset(inner.Min).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rectangle{Max: inner.Size()}, max(0, radius-borderWidth)).Op(gtx.Ops))
	offset.Pop()
}

func domActionDescription(action uidsl.Action) string {
	if action.Command == "navigate" {
		return "Open"
	}
	return action.Command
}

func (r *Renderer) compileDOMInput(node uidsl.Node, data any, path string) giodom.Element {
	if node.Input == nil {
		return r.domMessage(giodom.Key(path+"/missing-input"), "Input configuration is missing", r.palette.danger)
	}
	value, err := uidsl.Resolve(data, node.Input.Value)
	if err != nil {
		return r.domError(path, err)
	}
	text := fmt.Sprint(value)
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		NewState: func() any {
			state := new(domEditorState)
			state.editor.SetText(text)
			return state
		},
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			state := raw.(*domEditorState)
			if !gtx.Focused(&state.editor) && state.editor.Text() != text {
				state.editor.SetText(text)
			}
			state.editor.SingleLine = !node.Input.Multiline
			state.editor.Submit = !node.Input.Multiline
			changed := false
			for {
				event, ok := state.editor.Update(gtx)
				if !ok {
					break
				}
				switch event.(type) {
				case widget.ChangeEvent, widget.SubmitEvent:
					changed = true
				}
			}
			if changed && len(node.Actions) > 0 {
				inputData := mergeData(data, "input", map[string]any{"value": state.editor.Text()})
				r.dispatchFromLayout(gtx, node.Actions[0], inputData)
			}
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				paintDOMSurface(gtx, size, r.palette.surface, r.palette.border, gtx.Dp(1), gtx.Dp(r.metrics.controlRadius))
				return layout.Dimensions{Size: size}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top: r.metrics.controlPaddingY, Right: r.metrics.controlPaddingX,
					Bottom: r.metrics.controlPaddingY, Left: r.metrics.controlPaddingX,
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if node.Input.Multiline && node.Input.MinLines > 1 {
						minimum := gtx.Dp(unit.Dp(float32(node.Input.MinLines) * 24))
						gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, minimum))
					}
					style := material.Editor(r.theme, &state.editor, node.Input.Placeholder)
					role := "control"
					if node.Style.Role == "code" || node.Style.Role == "code-inline" {
						role = "code"
					}
					typography := r.nativeTextStyle(role, false)
					style.Font, style.TextSize, style.LineHeightScale = typography.font, typography.size, typography.lineHeight
					style.Color, style.HintColor = r.palette.text, r.palette.muted
					return style.Layout(gtx)
				})
			})
		},
	})
}

func (r *Renderer) compileDOMSelect(node uidsl.Node, data any, path string) giodom.Element {
	if node.Select == nil {
		return r.domMessage(giodom.Key(path+"/missing-select"), "Select configuration is missing", r.palette.danger)
	}
	value, err := uidsl.Resolve(data, node.Select.Value)
	if err != nil {
		return r.domError(path, err)
	}
	items, err := resolveItems(data, node.Select.Options)
	if err != nil {
		return r.domError(path, err)
	}
	options := make([]nativeSelectOption, 0, len(items))
	selectedValue := fmt.Sprint(value)
	selectedLabel := selectedValue
	for _, item := range items {
		itemData := mergeData(data, node.Select.As, item)
		optionValue, valueErr := uidsl.Resolve(itemData, node.Select.OptionValue)
		optionLabel, labelErr := uidsl.Resolve(itemData, node.Select.OptionLabel)
		if valueErr != nil {
			return r.domError(path, valueErr)
		}
		if labelErr != nil {
			return r.domError(path, labelErr)
		}
		entry := nativeSelectOption{value: fmt.Sprint(optionValue), label: fmt.Sprint(optionLabel)}
		options = append(options, entry)
		if entry.value == selectedValue {
			selectedLabel = entry.label
		}
	}
	enabled := conditionEnabled(node.Enabled, data)
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		NewState: func() any {
			return &domSelectState{options: map[string]*widget.Clickable{}, list: layout.List{Axis: layout.Vertical}}
		},
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			state := raw.(*domSelectState)
			seen := make(map[string]bool, len(options))
			for _, option := range options {
				seen[option.value] = true
				button := state.options[option.value]
				if button == nil {
					button = new(widget.Clickable)
					state.options[option.value] = button
				}
				for button.Clicked(gtx) {
					state.open = false
					if len(node.Actions) > 0 && option.value != selectedValue {
						selectionData := mergeData(data, "selection", map[string]any{"value": option.value, "label": option.label})
						r.dispatchFromLayout(gtx, node.Actions[0], selectionData)
					}
					r.requestFrame()
				}
			}
			for key := range state.options {
				if !seen[key] {
					delete(state.options, key)
				}
			}
			for state.toggle.Clicked(gtx) {
				if enabled {
					state.open = !state.open
					r.requestFrame()
				}
			}
			if !enabled {
				state.open = false
			}
			for state.dismiss.Clicked(gtx) {
				state.open = false
				r.requestFrame()
			}
			return r.layoutDOMSelect(gtx, state, node, options, selectedLabel, selectedValue, enabled)
		},
	})
}

func (r *Renderer) layoutDOMSelect(gtx layout.Context, state *domSelectState, node uidsl.Node, options []nativeSelectOption, selectedLabel, selectedValue string, enabled bool) layout.Dimensions {
	if !node.Layout.Grow {
		gtx.Constraints.Min.X = 0
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	header := func(gtx layout.Context) layout.Dimensions {
		icon := "chevron-down"
		if state.open {
			icon = "chevron-up"
		}
		return r.layoutDOMControl(gtx, &state.toggle, selectedLabel, icon, "select", "accent")
	}
	if !state.open {
		return header(gtx)
	}
	headerMacro := op.Record(gtx.Ops)
	headerDimensions := header(gtx)
	headerCall := headerMacro.Stop()
	overlayMacro := op.Record(gtx.Ops)
	scrimContext := gtx
	scrimContext.Constraints = layout.Exact(gtx.Constraints.Max)
	state.dismiss.Layout(scrimContext, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	})
	menuMacro := op.Record(gtx.Ops)
	menuContext := gtx
	menuContext.Constraints.Min = image.Point{}
	menuContext.Constraints.Max.X = min(menuContext.Constraints.Max.X, gtx.Dp(420))
	menuContext.Constraints.Min.X = min(menuContext.Constraints.Max.X, max(headerDimensions.Size.X, gtx.Dp(160)))
	menuContext.Constraints.Max.Y = min(menuContext.Constraints.Max.Y, gtx.Dp(320))
	menuDimensions := layout.Background{}.Layout(menuContext, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		paintDOMSurface(gtx, size, r.palette.surface, r.palette.border, gtx.Dp(1), gtx.Dp(r.metrics.controlRadius))
		return layout.Dimensions{Size: size}
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return state.list.Layout(gtx, len(options), func(gtx layout.Context, index int) layout.Dimensions {
				option := options[index]
				button := state.options[option.value]
				return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					fill := r.palette.surface
					if option.value == selectedValue {
						fill = r.palette.subtle
					}
					return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						size := gtx.Constraints.Min
						paintDOMSurface(gtx, size, fill, color.NRGBA{}, 0, gtx.Dp(r.metrics.controlRadius))
						return layout.Dimensions{Size: size}
					}, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 8, Right: 10, Bottom: 8, Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							label := r.materialTextLabel(option.label, "control", option.value == selectedValue)
							label.Color = r.palette.text
							return label.Layout(gtx)
						})
					})
				})
			})
		})
	})
	menuCall := menuMacro.Stop()
	menuY := headerDimensions.Size.Y + gtx.Dp(4)
	if menuY+menuDimensions.Size.Y > gtx.Constraints.Max.Y && menuDimensions.Size.Y+gtx.Dp(4) < gtx.Constraints.Max.Y {
		menuY = -menuDimensions.Size.Y - gtx.Dp(4)
	}
	op.Offset(image.Pt(0, menuY)).Add(gtx.Ops)
	menuCall.Add(gtx.Ops)
	op.Defer(gtx.Ops, overlayMacro.Stop())
	headerCall.Add(gtx.Ops)
	return headerDimensions
}

func (r *Renderer) compileDOMImage(node uidsl.Node, data any, path string) giodom.Element {
	if node.Image == nil {
		return r.domMessage(giodom.Key(path+"/missing-image"), "Image source is missing", r.palette.danger)
	}
	var encoded string
	if node.Image.Binding != "" {
		value, err := uidsl.Resolve(data, node.Image.Binding)
		if err != nil {
			return r.domError(path, err)
		}
		encoded = strings.TrimSpace(fmt.Sprint(value))
	}
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		NewState: func() any { return new(domImageState) },
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			state := raw.(*domImageState)
			var source paint.ImageOp
			if node.Image.Binding != "" {
				if encoded == "" {
					return layout.Dimensions{}
				}
				if state.encoded != encoded {
					payload, decodeErr := base64.StdEncoding.DecodeString(encoded)
					if decodeErr != nil {
						return r.errorLabel(gtx, fmt.Errorf("decode bound image: %w", decodeErr))
					}
					decoded, _, decodeErr := image.Decode(bytes.NewReader(payload))
					if decodeErr != nil {
						return r.errorLabel(gtx, fmt.Errorf("decode bound image: %w", decodeErr))
					}
					state.encoded = encoded
					state.source = paint.NewImageOp(decoded)
					state.source.Filter = paint.FilterNearest
				}
				source = state.source
			} else {
				var ok bool
				source, ok = r.images[node.Image.Asset]
				if !ok {
					return r.errorLabel(gtx, fmt.Errorf("image asset %q is unavailable", node.Image.Asset))
				}
			}
			width, height := r.metrics.imageBrandWidth, r.metrics.imageBrandHeight
			if node.Style.Role == "project-icon" {
				width, height = 72, 72
			} else if node.Style.Role == "job-header-icon" {
				width, height = 100, 100
			}
			return r.layoutImageSource(gtx, source, node.Image.Description, width, height)
		},
	})
}

func (r *Renderer) compileDOMIcon(node uidsl.Node, data any, path string) giodom.Element {
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		Layout: func(gtx layout.Context, _ any) layout.Dimensions {
			return r.layoutIcon(gtx, node, data)
		},
	})
}

func (r *Renderer) compileDOMSpacer(node uidsl.Node, path string) giodom.Element {
	width, _ := strconv.ParseFloat(strings.TrimSpace(node.Layout.MinWidth), 32)
	height, _ := strconv.ParseFloat(strings.TrimSpace(node.Layout.MinHeight), 32)
	element := giodom.Spacer(domNodeKey(node, path), unit.Dp(width), unit.Dp(height))
	if node.Layout.Grow {
		element.Grow = true
	}
	return element
}

func (r *Renderer) compileDOMDisclosure(node uidsl.Node, data any, path string) giodom.Element {
	label := "Details"
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.domError(path, err)
		}
		label = resolved
	}
	stateKey, persistent := r.disclosureStateKey(node, data, path)
	expanded, exists := r.disclosures[stateKey]
	if !exists {
		expanded = disclosureDefaultExpanded(node.Disclosure, data)
		r.rememberDOMDisclosure(stateKey, expanded, persistent)
	}
	if persistent {
		r.persistentDisclosures[stateKey] = true
	}
	navigatePresentation := r.compact && node.Disclosure != nil && node.Disclosure.CompactPresentation == "navigate"
	navigateAction, hasNavigateAction := disclosureNavigationAction(node.Disclosure)
	iconOnlyToggle := node.Style.Role == "project-row" || node.Style.Role == "execution-row"
	icon := "chevron-right"
	if expanded {
		icon = "chevron-down"
	}
	toggleRole := "disclosure-toggle"
	if iconOnlyToggle {
		toggleRole = "icon-button"
	}
	toggleNode := uidsl.Node{
		Component: "button", Text: &uidsl.Text{Literal: label}, Icon: icon,
		Style: uidsl.Style{Role: toggleRole, Tone: node.Style.Tone},
	}
	if navigatePresentation && hasNavigateAction {
		toggleNode.Actions = []uidsl.Action{navigateAction}
	} else {
		toggleNode.Actions = []uidsl.Action{{Command: "toggle"}}
	}
	header := r.compileDOMButton(toggleNode, data, path+"/toggle")
	if !(navigatePresentation && hasNavigateAction) {
		header.Native.Layout = func(original func(layout.Context, any) layout.Dimensions) func(layout.Context, any) layout.Dimensions {
			return func(gtx layout.Context, raw any) layout.Dimensions {
				state := raw.(*domButtonState)
				for state.clickable.Clicked(gtx) {
					r.setDisclosureState(stateKey, !expanded, persistent)
				}
				role := "disclosure-toggle"
				if iconOnlyToggle {
					role = "icon-button"
				}
				return r.layoutDOMControl(gtx, &state.clickable, label, icon, role, node.Style.Tone)
			}
		}(header.Native.Layout)
	}
	summary := []giodom.Element{}
	if node.Disclosure != nil {
		summary = append(summary, r.compileDOMChildren(node.Disclosure.Summary, data, path+"/summary")...)
	}
	if iconOnlyToggle {
		summary = append(summary, header)
	} else {
		summary = append([]giodom.Element{header}, summary...)
	}
	headerRow := giodom.Element{
		Kind: giodom.KindFlex, Key: giodom.Key(path + "/header"),
		Flex:     giodom.FlexProps{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: r.metrics.spaceSmall, Wrap: node.Style.Role == "project-row"},
		Children: giodom.Static(summary...),
	}
	content := []giodom.Element{headerRow}
	if expanded && !navigatePresentation {
		children := node.Children
		if node.Style.Role == "output-group" {
			children = withDefaultConsoleText(children)
		}
		content = append(content, r.compileDOMChildren(children, data, path+"/body")...)
	}
	return giodom.Element{
		Kind: giodom.KindFlex, Key: domNodeKey(node, path),
		Flex: giodom.FlexProps{
			Axis: layout.Vertical, Alignment: layout.Start, Gap: r.spacing(node.Layout.Gap),
			Padding: giodom.UniformInsets(r.spacing(node.Layout.Padding)),
		},
		Children: giodom.Static(content...),
	}
}

func (r *Renderer) rememberDOMDisclosure(key string, expanded, persistent bool) {
	if strings.TrimSpace(key) == "" {
		return
	}
	if len(r.disclosures) >= domSemanticStateLimit {
		for candidate := range r.disclosures {
			if !r.persistentDisclosures[candidate] {
				delete(r.disclosures, candidate)
				if len(r.disclosures) < domSemanticStateLimit {
					break
				}
			}
		}
	}
	if len(r.disclosures) >= domSemanticStateLimit {
		return
	}
	r.disclosures[key] = expanded
	if persistent {
		r.persistentDisclosures[key] = true
	}
}

func (r *Renderer) compileDOMTree(node uidsl.Node, data any, path string) giodom.Element {
	if node.TreeView == nil {
		return r.domMessage(giodom.Key(path+"/missing-tree"), "Tree configuration is missing", r.palette.danger)
	}
	items, err := resolveItems(data, node.TreeView.Nodes)
	if err != nil {
		return r.domError(path, err)
	}
	filter := ""
	if node.TreeView.Filter != "" {
		if value, resolveErr := uidsl.Resolve(data, node.TreeView.Filter); resolveErr == nil {
			filter = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	elements := make([]giodom.Element, 0, len(items))
	for index, item := range items {
		itemData := mergeData(data, node.TreeView.As, item)
		if !treeEntryVisible(itemData, node.TreeView, filter) {
			continue
		}
		keyValue, keyErr := uidsl.Resolve(itemData, node.TreeView.NodeKey)
		if keyErr != nil {
			return r.domError(path, keyErr)
		}
		key := fmt.Sprint(keyValue)
		entry, entryErr := treeEntryNode(node, itemData, key)
		if entryErr != nil {
			return r.domError(path, entryErr)
		}
		compiled := r.compileDOMNode(entry, itemData, fmt.Sprintf("%s/%d:%s", path, index, key))
		if compiled == nil {
			continue
		}
		compiled.Key = giodom.Key(key)
		elements = append(elements, *compiled)
	}
	return giodom.Element{
		Kind: giodom.KindFlex, Key: domNodeKey(node, path),
		Flex:     giodom.FlexProps{Axis: layout.Vertical, Alignment: layout.Start, Gap: 4},
		Children: giodom.Keyed(domElementsRevision(elements), elements...),
	}
}

func (r *Renderer) compileDOMGraph(node uidsl.Node, data any, path string) giodom.Element {
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		NewState: func() any { return newDOMGraphState(r.dom) },
		Layout: func(gtx layout.Context, raw any) layout.Dimensions {
			return r.layoutDOMGraphView(gtx, node, data, path, raw.(*domGraphState))
		},
	})
}

func (r *Renderer) compileDOMScroller(node uidsl.Node, data any, path string) *giodom.Element {
	if node.Repeat == nil {
		element := r.domMessage(giodom.Key(path+"/missing-repeat"), "Scroller repeat configuration is missing", r.palette.danger)
		return &element
	}
	items, err := resolveItems(data, node.Repeat.Source)
	if err != nil {
		element := r.domError(path, err)
		return &element
	}
	repeat := node.Repeat
	axis := layout.Horizontal
	if node.Layout.Direction == "vertical" {
		axis = layout.Vertical
	}
	keyAt := func(index int) giodom.Key {
		itemData := mergeData(data, repeat.As, items[index])
		if value, resolveErr := uidsl.Resolve(itemData, repeat.Key); resolveErr == nil {
			return giodom.Key(fmt.Sprint(value))
		}
		return giodom.Key(strconv.Itoa(index))
	}
	revision := uint64(1469598103934665603)
	for index := range items {
		for _, value := range []byte(keyAt(index)) {
			revision ^= uint64(value)
			revision *= 1099511628211
		}
	}
	build := func(index int) giodom.Element {
		itemData := mergeData(data, repeat.As, items[index])
		key := keyAt(index)
		container := node
		container.Component = "column"
		if axis == layout.Horizontal {
			container.Component = "row"
		}
		container.Repeat = nil
		container.Actions = nil
		compiled := r.compileDOMContainer(container, itemData, path+"/"+string(key))
		compiled.Key = key
		return compiled
	}
	children := giodom.Lazy(revision, len(items), keyAt, build)
	viewport := unit.Dp(0)
	if node.Layout.MaxHeight != "" {
		parsed, _ := strconv.ParseFloat(node.Layout.MaxHeight, 32)
		viewport = unit.Dp(parsed)
	}
	if node.Layout.MaxWidth != "" && axis == layout.Horizontal {
		parsed, _ := strconv.ParseFloat(node.Layout.MaxWidth, 32)
		viewport = unit.Dp(parsed)
	}
	if viewport <= 0 || (r.compact && node.ID == "job-output-groups") {
		elements := make([]giodom.Element, 0, len(items))
		for index := range items {
			elements = append(elements, build(index))
		}
		result := giodom.Element{
			Kind: giodom.KindFlex, Key: domNodeKey(node, path),
			Flex:     giodom.FlexProps{Axis: axis, Alignment: layout.Start, Gap: r.spacing(node.Layout.Gap)},
			Children: giodom.Keyed(revision, elements...),
		}
		return r.decorateDOMNode(result, node, data, path)
	}
	scrollTarget := giodom.Key("")
	scrollRevision := uint64(0)
	if node.ID == "job-output-groups" && r.pendingOutputScroll != "" {
		scrollTarget = giodom.Key(r.pendingOutputScroll)
		scrollRevision = r.outputScrollRevision
		r.pendingOutputScroll = ""
	}
	result := giodom.VirtualList(domNodeKey(node, path), giodom.ListProps{
		Axis: axis, Gap: r.spacing(node.Layout.Gap), Viewport: viewport,
		Estimate: 100, Overscan: 2, MaxMeasured: 512, ScrollToEnd: node.ID == "job-output-groups" && r.outputTailing,
		ScrollTo: scrollTarget, ScrollRevision: scrollRevision,
	}, children)
	return r.decorateDOMNode(result, node, data, path)
}

func (r *Renderer) domError(path string, err error) giodom.Element {
	if err == nil {
		return r.domMessage(giodom.Key(path+"/error"), "Unknown rendering error", r.palette.danger)
	}
	return r.domMessage(giodom.Key(path+"/error"), err.Error(), r.palette.danger)
}

func (r *Renderer) domMessage(key giodom.Key, message string, ink color.NRGBA) giodom.Element {
	return giodom.Text(key, message, r.metrics.textBody, ink)
}
