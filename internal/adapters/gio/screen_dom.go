//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"math"
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

type domStyleContext struct {
	tone     string
	emphasis string
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
	rootResolvedStyle, rootStyle := r.resolveDOMStyle(root.Component, root.Style, data, domStyleContext{})
	root.Style = rootResolvedStyle
	children := make([]giodom.Element, 0, len(root.Children))
	scrollTarget := giodom.Key("")
	for index := range root.Children {
		path := fmt.Sprintf("%s/root/%d", screen.Metadata.Name, index)
		compiled := r.compileDOMNodeWithStyle(root.Children[index], data, path, rootStyle)
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
	pageInset := r.pageInset()
	if len(children) > 0 {
		first := children[0]
		children[0] = giodom.Inset(first.Key, giodom.Insets{Top: pageInset}, first)
		lastIndex := len(children) - 1
		last := children[lastIndex]
		children[lastIndex] = giodom.Inset(last.Key, giodom.Insets{Bottom: pageInset}, last)
	}
	page := giodom.StockList(giodom.Key("page-list:"+screen.Metadata.Name), props, giodom.Keyed(domElementsRevision(children), children...))
	page = giodom.Inset(giodom.Key("page-inset:"+screen.Metadata.Name), giodom.Insets{
		Right: pageInset, Left: pageInset,
	}, page)
	if r.metrics.pageWidth > 0 {
		page = giodom.Constrain(giodom.Key("page-width:"+screen.Metadata.Name), giodom.ConstraintProps{MaxWidth: r.metrics.pageWidth}, page)
		page = giodom.Align(giodom.Key("page-center:"+screen.Metadata.Name), layout.N, page)
	}
	return page
}

func (r *Renderer) compileDOMNode(raw uidsl.Node, data any, path string) *giodom.Element {
	return r.compileDOMNodeWithStyle(raw, data, path, domStyleContext{})
}

func (r *Renderer) compileDOMNodeWithStyle(raw uidsl.Node, data any, path string, inherited domStyleContext) *giodom.Element {
	node, hidden := applyGioOverride(raw, r.compact)
	if hidden || !domNodeVisible(node, data) {
		return nil
	}
	if node.Component == "scroller" {
		resolvedStyle, childStyle := r.resolveDOMStyle(node.Component, node.Style, data, inherited)
		node.Style = resolvedStyle
		return r.compileDOMScroller(node, data, path, childStyle)
	}
	if node.Repeat != nil {
		return r.compileDOMRepeat(node, data, path, inherited)
	}
	resolvedStyle, childStyle := r.resolveDOMStyle(node.Component, node.Style, data, inherited)
	node.Style = resolvedStyle
	if node.Style.Role == "skeleton" {
		element := r.compileDOMSkeleton(node, path)
		return r.decorateDOMNode(element, node, data, path)
	}

	var element giodom.Element
	switch node.Component {
	case "page", "column", "row", "list", "section", "card":
		element = r.compileDOMContainer(node, data, path, childStyle)
	case "disclosure":
		element = r.compileDOMDisclosure(node, data, path, childStyle)
	case "tree-view":
		element = r.compileDOMTree(node, data, path, childStyle)
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

func (r *Renderer) resolveDOMStyle(component string, style uidsl.Style, data any, inherited domStyleContext) (uidsl.Style, domStyleContext) {
	if style.ToneBinding != "" {
		if value, err := uidsl.Resolve(data, style.ToneBinding); err == nil {
			style.Tone = semanticTone(fmt.Sprint(value))
		}
	}
	if style.Tone == "" {
		if local := domLocalTone(component, style.Role); local != "" {
			style.Tone = local
		} else {
			style.Tone = inherited.tone
		}
	}
	if style.Emphasis == "" && !r.domResetsEmphasis(component, style.Role) {
		style.Emphasis = inherited.emphasis
	}
	return style, domStyleContext{tone: style.Tone, emphasis: style.Emphasis}
}

func domLocalTone(component, role string) string {
	switch component {
	case "button", "icon":
		return "accent"
	case "input", "select":
		return "text"
	}
	switch role {
	case "title", "job-title", "heading", "tree-label":
		return "text"
	case "table-header", "tree-detail":
		return "muted"
	case "link":
		return "accent"
	case "output-system", "output-group", "output-code", "output-meta":
		return "console-text"
	case "output-label":
		return "console-accent"
	}
	return ""
}

func (r *Renderer) domResetsEmphasis(component, role string) bool {
	if component == "button" || component == "input" || component == "select" || component == "badge" {
		return true
	}
	if component == "text" {
		_, defined := r.typography.Roles[role]
		return defined
	}
	return false
}

func (r *Renderer) compileDOMRepeat(node uidsl.Node, data any, path string, inherited domStyleContext) *giodom.Element {
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
		compiled := r.compileDOMNodeWithStyle(clone, itemData, path+"/"+key, inherited)
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
		Flex: giodom.FlexProps{
			Axis: axis, Alignment: flexAlignment(axis, node.Layout.Align, false), Spacing: domFlexSpacing(node.Layout.Justify),
			Gap: r.spacing(node.Layout.Gap), Wrap: node.Layout.Wrap,
		},
		Children: giodom.Keyed(domElementsRevision(elements), elements...),
	}
	return r.decorateDOMNode(result, node, data, path)
}

func (r *Renderer) compileDOMContainer(node uidsl.Node, data any, path string, childStyle domStyleContext) giodom.Element {
	if r.compact && node.ID == "project-header" {
		return r.compileDOMCompactProjectHeader(node, data, path, childStyle)
	}
	if r.compact && node.Style.Role == "compact-action-row" {
		return r.compileDOMCompactActionRow(node, data, path, childStyle)
	}
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
	weights := domGridWeights(node.Style.Role, len(node.Children))
	children := r.compileDOMChildren(node.Children, data, path, childStyle)
	if weights != nil && !r.compact {
		children = r.compileDOMGridChildren(node.Children, data, path, weights, childStyle)
	}
	if stackedCompactRow {
		for index := range children {
			children[index].Grow = false
		}
	}
	if r.compact && (node.Style.Role == "queued-execution-job-row" || node.Style.Role == "history-execution-job-row" || node.Style.Role == "agent-record") {
		children = r.compactDOMRecordChildren(node, data, path, childStyle)
		axis = layout.Vertical
	}
	props := giodom.FlexProps{
		Axis: axis, Alignment: flexAlignment(axis, node.Layout.Align, false),
		Spacing: domFlexSpacing(node.Layout.Justify),
		Gap:     r.spacing(node.Layout.Gap), Padding: giodom.UniformInsets(r.spacing(node.Layout.Padding)),
		Wrap:    axis == layout.Horizontal && (node.Layout.Wrap || node.Style.Role == "hero") && !(weights != nil && !r.compact),
		Stretch: node.Style.Role == "report-stack",
	}
	if node.Style.Role == "hero-brand" {
		props.Gap = max(props.Gap, r.metrics.spaceMedium+r.metrics.spaceSmall/2)
	}
	if node.Style.Role == "queued-execution-header" || node.Style.Role == "history-execution-header" || node.Style.Role == "agent-header" {
		props.Padding = giodom.Insets{Top: 6, Right: r.metrics.spaceSmall, Bottom: 6, Left: r.metrics.spaceSmall}
	}
	return giodom.Element{Kind: giodom.KindFlex, Key: domNodeKey(node, path), Flex: props, Children: giodom.Static(children...)}
}

func (r *Renderer) compileDOMGridChildren(nodes []uidsl.Node, data any, path string, weights []float32, inherited domStyleContext) []giodom.Element {
	children := make([]giodom.Element, 0, len(nodes))
	for index := range nodes {
		compiled := r.compileDOMNodeWithStyle(nodes[index], data, fmt.Sprintf("%s/%d", path, index), inherited)
		if compiled == nil {
			empty := giodom.Spacer(giodom.Key(fmt.Sprintf("%s/empty/%d", path, index)), 0, 0)
			compiled = &empty
		}
		compiled.FlexWeight = weights[index]
		children = append(children, *compiled)
	}
	return children
}

func domGridWeights(role string, childCount int) []float32 {
	var weights []float32
	switch role {
	case "queued-execution-header", "queued-execution-job-row":
		weights = []float32{2, 1, 1.25, 1.1, 1.2, 1.35, 2.25, .85}
	case "history-execution-header", "history-execution-job-row":
		weights = []float32{2, 1, 1.25, 1.1, 1.2, 1.35, 1}
	case "agent-header":
		weights = []float32{1.6, 1.35, 1.05, .8, 1.2, .9, .8}
	case "agent-record":
		weights = []float32{1.6, 1.35, 1.05, .8, 1.2, .9, .8, .2}
	}
	if len(weights) != childCount {
		return nil
	}
	return weights
}

func domFlexSpacing(justify string) layout.Spacing {
	switch strings.ToLower(strings.TrimSpace(justify)) {
	case "end", "flex-end":
		return layout.SpaceStart
	case "center":
		return layout.SpaceSides
	case "space-around":
		return layout.SpaceAround
	case "space-between":
		return layout.SpaceBetween
	case "space-evenly":
		return layout.SpaceEvenly
	default:
		return layout.SpaceEnd
	}
}

func (r *Renderer) compileDOMCompactProjectHeader(node uidsl.Node, data any, path string, inherited domStyleContext) giodom.Element {
	var logo, title, metadata, back *giodom.Element
	for index := range node.Children {
		child := node.Children[index]
		switch child.Style.Role {
		case "project-icon":
			logo = r.compileDOMNodeWithStyle(child, data, fmt.Sprintf("%s/%d", path, index), inherited)
			if logo == nil {
				empty := giodom.Spacer(giodom.Key(path+"/compact-logo-empty"), 0, 0)
				logo = &empty
			}
		case "project-header-back":
			child.Style.Role = "icon-button"
			back = r.compileDOMNodeWithStyle(child, data, fmt.Sprintf("%s/%d", path, index), inherited)
		case "project-header-copy":
			for copyIndex := range child.Children {
				copyChild := child.Children[copyIndex]
				switch copyChild.Style.Role {
				case "title":
					title = r.compileDOMNodeWithStyle(copyChild, data, fmt.Sprintf("%s/%d/%d", path, index, copyIndex), inherited)
				case "project-header-metadata":
					metadata = r.compileDOMNodeWithStyle(copyChild, data, fmt.Sprintf("%s/%d/%d", path, index, copyIndex), inherited)
				}
			}
		}
	}
	if logo == nil || title == nil || metadata == nil || back == nil {
		return r.domMessage(giodom.Key(path+"/compact-header-error"), "Compact project header is incomplete", r.palette.danger)
	}
	title.Grow = true
	top := giodom.Element{
		Kind: giodom.KindFlex, Key: giodom.Key(path + "/compact-top"),
		Flex:     giodom.FlexProps{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: r.metrics.spaceMedium},
		Children: giodom.Static(*back, *title, *logo),
	}
	return giodom.Element{
		Kind: giodom.KindFlex, Key: domNodeKey(node, path),
		Flex:     giodom.FlexProps{Axis: layout.Vertical, Alignment: layout.Start, Gap: r.metrics.spaceSmall},
		Children: giodom.Static(top, *metadata),
	}
}

func (r *Renderer) compileDOMCompactActionRow(node uidsl.Node, data any, path string, inherited domStyleContext) giodom.Element {
	content, actions := make([]giodom.Element, 0, len(node.Children)), make([]giodom.Element, 0, 2)
	for index := range node.Children {
		child := node.Children[index]
		if child.Component == "spacer" {
			continue
		}
		compiled := r.compileDOMNodeWithStyle(child, data, fmt.Sprintf("%s/%d", path, index), inherited)
		if compiled == nil {
			continue
		}
		compiled.Grow = false
		if child.Component == "button" {
			actions = append(actions, *compiled)
		} else {
			content = append(content, *compiled)
		}
	}
	children := make([]giodom.Element, 0, 2)
	if len(content) > 0 {
		children = append(children, giodom.Element{
			Kind: giodom.KindFlex, Key: giodom.Key(path + "/compact-content"),
			Flex: giodom.FlexProps{Axis: layout.Vertical, Alignment: layout.Start, Gap: r.metrics.spaceSmall}, Children: giodom.Static(content...),
		})
	}
	if len(actions) > 0 {
		children = append(children, giodom.Element{
			Kind: giodom.KindFlex, Key: giodom.Key(path + "/compact-actions"),
			Flex: giodom.FlexProps{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: r.metrics.spaceSmall, Wrap: true}, Children: giodom.Static(actions...),
		})
	}
	return giodom.Element{
		Kind: giodom.KindFlex, Key: domNodeKey(node, path),
		Flex: giodom.FlexProps{
			Axis: layout.Vertical, Alignment: layout.Start, Gap: r.metrics.spaceSmall,
			Padding: giodom.UniformInsets(r.spacing(node.Layout.Padding)),
		},
		Children: giodom.Static(children...),
	}
}

func (r *Renderer) compileDOMChildren(nodes []uidsl.Node, data any, path string, inherited domStyleContext) []giodom.Element {
	children := make([]giodom.Element, 0, len(nodes))
	for index := range nodes {
		compiled := r.compileDOMNodeWithStyle(nodes[index], data, fmt.Sprintf("%s/%d", path, index), inherited)
		if compiled == nil {
			continue
		}
		compiled.Grow = nodes[index].Layout.Grow
		children = append(children, *compiled)
	}
	return children
}

func (r *Renderer) compactDOMRecordChildren(node uidsl.Node, data any, path string, inherited domStyleContext) []giodom.Element {
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
		value := r.compileDOMNodeWithStyle(node.Children[index], data, fmt.Sprintf("%s/%d", path, index), inherited)
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

func (r *Renderer) compileDOMSkeleton(node uidsl.Node, path string) giodom.Element {
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		Layout: func(gtx layout.Context, _ any) layout.Dimensions {
			size := gtx.Constraints.Min
			if size.X <= 0 || size.Y <= 0 {
				return layout.Dimensions{Size: size}
			}
			const cycle = 2200 * time.Millisecond
			phase := float64(gtx.Now.UnixNano()%int64(cycle)) / float64(cycle)
			opacity := float32(.35 + .55*(.5-.5*math.Cos(2*math.Pi*phase)))
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(progressFrameInterval)})
			fade := paint.PushOpacity(gtx.Ops, opacity)
			paintDOMSurface(gtx, size, r.palette.border, color.NRGBA{}, 0, gtx.Dp(r.metrics.controlRadius))
			fade.Pop()
			return layout.Dimensions{Size: size}
		},
	})
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
	var progressProps *giodom.ProgressProps
	if node.Component != "disclosure" {
		progressProps = r.domProgressProps(node, data)
	}

	isOutputConsole := node.ID == "job-output-groups"
	isSurface := node.Component == "card" || node.Component == "section" || node.Component == "disclosure" || node.Component == "graph-view" || node.Style.Role == "hero" || node.Style.Role == "execution-section-header" || isOutputConsole
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
		if node.Component == "disclosure" && r.domProgressProps(node, data) != nil {
			props.Padding = giodom.Insets{}
		}
		if node.Component == "card" || node.Component == "section" || node.Style.Role == "hero" {
			props.PaintBackground = r.paintCardSurface
		}
		switch {
		case node.Style.Role == "hero":
			props.Padding = giodom.UniformInsets(r.metrics.heroPadding)
		case node.Component == "disclosure" && node.Style.Role == "output-group":
			props.Fill, props.Border = r.palette.consoleSurface, r.palette.consoleBorder
			props.PaintBackground = nil
		case node.Component == "disclosure":
			props.Fill = r.palette.surfaceRaised
		case node.Component == "card" && node.Style.Role == "output-system":
			props.Fill, props.Border = r.palette.consoleSurface, r.palette.consoleBorder
			props.PaintBackground = nil
		case node.Component == "card" && node.Style.Role == "scheduling-awaiting":
			props.Fill, props.Border = r.palette.awaitingSurface, r.palette.awaitingBorder
			props.PaintBackground = nil
		case node.Style.Role == "execution-section-header":
			props.Fill, props.BorderWidth, props.Radius = r.palette.subtle, 0, r.metrics.controlRadius
		case isOutputConsole:
			props.Fill, props.Border, props.Padding = r.palette.consoleBackground, r.palette.consoleBorder, giodom.UniformInsets(r.metrics.spaceSmall)
			props.PaintBackground = nil
		}
		if progressProps != nil {
			content := element
			if props.Padding != (giodom.Insets{}) {
				content = giodom.Inset(giodom.Key(path+"/surface-content-inset"), props.Padding, content)
				props.Padding = giodom.Insets{}
			}
			content = giodom.Progress(giodom.Key(path+"/progress"), *progressProps, content)
			element = giodom.Surface(giodom.Key(path+"/surface"), props, content)
			progressProps = nil
		} else {
			element = giodom.Surface(giodom.Key(path+"/surface"), props, element)
		}
	}
	if progressProps != nil {
		switch node.Style.Role {
		case "execution-row":
			progressProps.Track = r.palette.surfaceRaised
		case "execution-section-header":
			progressProps.Track = r.palette.subtle
		}
		element = giodom.Progress(giodom.Key(path+"/progress"), *progressProps, element)
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

func (r *Renderer) domProgressProps(node uidsl.Node, data any) *giodom.ProgressProps {
	progress, active := activeSemanticProgress(data, node.Progress)
	if !active {
		return nil
	}
	state, fraction := evaluateSemanticProgress(progress, time.Now())
	mode := giodom.ProgressDeterminate
	switch state {
	case "indeterminate":
		mode = giodom.ProgressIndeterminate
	case "overrun":
		mode = giodom.ProgressOverrun
	case "complete", "completed":
		mode = giodom.ProgressComplete
	}
	fill := r.palette.success
	if node.Style.Role == "output-group" {
		fill = r.palette.consoleSuccess
	}
	track := color.NRGBA{}
	base := r.domProgressBase(node)
	tintOpacity := float64(r.controls.Progress.TintOpacity)
	if base.A == 0xff {
		fill = mixColorSRGB(base, fill, tintOpacity)
	} else {
		fill.A = uint8(math.Round(0xff * tintOpacity))
	}
	return &giodom.ProgressProps{
		Mode: mode, Fraction: float32(fraction), Animate: state == "determinate" && progress.ratePerMS > 0, Color: fill,
		Track: track, Radius: r.metrics.surfaceRadius, Phase: -time.Duration(progress.snapshotUnixMS) * time.Millisecond,
	}
}

func (r *Renderer) domProgressBase(node uidsl.Node) color.NRGBA {
	switch node.Style.Role {
	case "execution-row":
		return r.palette.surfaceRaised
	case "execution-section-header":
		return r.palette.subtle
	case "output-group":
		return r.palette.consoleSurface
	}
	switch node.Component {
	case "card", "section", "graph-view":
		return r.palette.surface
	case "disclosure":
		return r.palette.surfaceRaised
	default:
		return color.NRGBA{}
	}
}

func (r *Renderer) domConstraint(values uidsl.Layout) (giodom.ConstraintProps, bool) {
	props := giodom.ConstraintProps{
		MinWidth: r.domLayoutDimension(values.MinWidth), MaxWidth: r.domLayoutDimension(values.MaxWidth),
		MinHeight: r.domLayoutDimension(values.MinHeight), MaxHeight: r.domLayoutDimension(values.MaxHeight),
	}
	// Layout dimensions describe the complete rendered box, matching the
	// browser's global border-box sizing. Surface padding and borders therefore
	// consume space inside these constraints rather than expanding them.
	return props, props != (giodom.ConstraintProps{})
}

func (r *Renderer) domLayoutDimension(value string) unit.Dp {
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
	displayValue := value
	if role == "output-code" {
		displayValue = strings.TrimSuffix(displayValue, "\n")
		displayValue = strings.TrimSuffix(displayValue, "\r")
	}
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
			if state.editor.Text() != displayValue {
				state.editor.SetText(displayValue)
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
			Font: style.font, LineHeightScale: style.lineHeight, Selectable: selectable,
		},
	}
}

func (r *Renderer) domTextColor(role, tone string) color.NRGBA {
	if resolved, ok := r.toneColor(tone); ok {
		return resolved
	}
	if local := domLocalTone("text", role); local != "" {
		if resolved, ok := r.toneColor(local); ok {
			return resolved
		}
	}
	return r.palette.text
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
				if state.clickable.Hovered() || gtx.Focused(&state.clickable) {
					label.Color = r.palette.accentStrong
				}
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
		fill = mixColorSRGB(r.palette.surface, tone, float64(r.controls.Badge.TintOpacity))
		border.A = uint8(math.Round(0xff * float64(r.controls.Badge.BorderOpacity)))
	}
	text := r.domText(giodom.Key(path+"/text"), value, "badge", node.Style.Emphasis == "strong", textTone, true)
	badge := giodom.Surface(domNodeKey(node, path), giodom.SurfaceProps{
		Fill: fill, Border: border, BorderWidth: borderWidth, Radius: 100,
		Padding: giodom.Insets{
			Top: unit.Dp(r.controls.Badge.PaddingY), Right: unit.Dp(r.controls.Badge.PaddingX),
			Bottom: unit.Dp(r.controls.Badge.PaddingY), Left: unit.Dp(r.controls.Badge.PaddingX),
		},
	}, text)
	badge.FitContent = true
	return badge
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
				dimensions := r.layoutDOMControlWithOptions(gtx, &state.clickable, label, node.Icon, node.Style.Role, node.Style.Tone, domControlOptions{
					Enabled: enabled, FillWidth: node.Layout.Grow,
					MinimumWidth: r.domLayoutDimension(node.Layout.MinWidth), MinimumHeight: r.domLayoutDimension(node.Layout.MinHeight),
					ReservedLabels: r.domButtonReservedLabels(node, data, label),
				})
				fade.Pop()
				return dimensions
			}
			return r.layoutDOMControlWithOptions(gtx, &state.clickable, label, node.Icon, node.Style.Role, node.Style.Tone, domControlOptions{
				Enabled: enabled, FillWidth: node.Layout.Grow,
				MinimumWidth: r.domLayoutDimension(node.Layout.MinWidth), MinimumHeight: r.domLayoutDimension(node.Layout.MinHeight),
				ReservedLabels: r.domButtonReservedLabels(node, data, label),
			})
		},
	})
}

type domControlOptions struct {
	Enabled        bool
	FillWidth      bool
	MinimumWidth   unit.Dp
	MinimumHeight  unit.Dp
	TrailingIcon   bool
	ReservedLabels []string
}

func (r *Renderer) layoutDOMControl(gtx layout.Context, clickable *widget.Clickable, label, iconName, role, tone string) layout.Dimensions {
	return r.layoutDOMControlWithOptions(gtx, clickable, label, iconName, role, tone, domControlOptions{
		Enabled: true, ReservedLabels: []string{label},
	})
}

func (r *Renderer) layoutDOMControlWithOptions(gtx layout.Context, clickable *widget.Clickable, label, iconName, role, tone string, options domControlOptions) layout.Dimensions {
	disclosureChevron := role == "disclosure-chevron"
	iconOnly := role == "icon-button" || role == "tailing-toggle" || disclosureChevron
	buttonMetrics := r.controls.Button
	iconSize := buttonMetrics.IconSize.Native
	iconGap := buttonMetrics.IconGap.Native
	trailingIcon := options.TrailingIcon
	if role != "select" && !iconOnly {
		trailingIcon = buttonMetrics.IconPosition == "trailing"
	}
	if role == "select" {
		iconSize = r.controls.Select.ChevronSize
		iconGap = r.controls.Select.ChevronGap
	}
	if len(options.ReservedLabels) == 0 {
		options.ReservedLabels = []string{label}
	}
	semantic.DescriptionOp(label).Add(gtx.Ops)
	// Controls size to their own metrics. A surrounding layout's cross-axis
	// minimum belongs to that layout rather than each control inside it.
	gtx.Constraints.Min.Y = 0
	// Cross-axis constraints belong to the containing layout. Only controls
	// explicitly marked grow inherit its horizontal minimum; all other controls
	// size to their shared content metrics.
	if !options.FillWidth {
		gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, gtx.Dp(options.MinimumWidth))
	}
	return clickable.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		minimum := unit.Dp(buttonMetrics.MinimumHeight.Native)
		if role == "select" {
			minimum = unit.Dp(r.controls.Select.MinimumHeight)
		}
		if iconOnly {
			minimum = unit.Dp(buttonMetrics.IconOnlySize.Native)
		}
		if disclosureChevron {
			iconSize = r.controls.Disclosure.ChevronSize
			minimum = unit.Dp(max(float32(24), iconSize+4))
		}
		minimum = max(minimum, options.MinimumHeight)
		gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, gtx.Dp(minimum)))
		if iconOnly {
			gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, max(gtx.Constraints.Min.X, gtx.Dp(minimum)))
		}
		fill, border := r.palette.surface, r.palette.border
		inkTone := defaultString(tone, "accent")
		if clickable.Pressed() && !disclosureChevron {
			fill = r.palette.subtle
		}
		if (clickable.Hovered() || gtx.Focused(clickable)) && !disclosureChevron {
			border = r.palette.accent
		}
		if disclosureChevron {
			fill, border, inkTone = color.NRGBA{}, color.NRGBA{}, "muted"
			if clickable.Hovered() || gtx.Focused(clickable) {
				inkTone = "accent"
			}
		}
		if role == "tailing-toggle" {
			if toned, ok := r.toneColor(tone); ok {
				fill, border = mixColorSRGB(r.palette.surface, toned, .12), toned
			}
		}
		if !options.Enabled {
			fill, border, inkTone = r.palette.subtle, r.palette.border, "muted"
		}
		var fade paint.OpacityStack
		if !options.Enabled {
			fade = paint.PushOpacity(gtx.Ops, .65)
			defer fade.Pop()
		}
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Min
			paintDOMSurface(gtx, size, fill, border, gtx.Dp(1), gtx.Dp(r.metrics.controlRadius))
			return layout.Dimensions{Size: size}
		}, func(gtx layout.Context) layout.Dimensions {
			// The minimum height belongs to the control surface, not each child.
			// If it reaches Label.Layout, Gio constrains the label to that full
			// height while painting its glyphs from the top, which defeats the
			// Flex middle alignment. Let the foreground keep its intrinsic height;
			// Background.Layout centers it inside the minimum-sized surface.
			gtx.Constraints.Min.Y = 0
			paddingX := unit.Dp(buttonMetrics.PaddingX.Native)
			paddingY := unit.Dp(buttonMetrics.PaddingY.Native)
			if role == "select" {
				paddingX, paddingY = r.metrics.controlPaddingX, r.metrics.controlPaddingY
			}
			if iconOnly {
				inset := max(float32(0), (buttonMetrics.IconOnlySize.Native-iconSize)/2)
				paddingX, paddingY = unit.Dp(inset), unit.Dp(inset)
			}
			if disclosureChevron {
				paddingX, paddingY = 2, 2
			}
			return layout.Inset{Top: paddingY, Right: paddingX, Bottom: paddingY, Left: paddingX}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if iconOnly {
					return r.layoutGlyph(gtx, iconName, inkTone, unit.Dp(iconSize))
				}
				reservedWidth := r.domWidestControlLabel(gtx, options.ReservedLabels)
				iconWidth := 0
				if iconName != "" {
					iconWidth = gtx.Dp(unit.Dp(iconSize + iconGap))
				}
				gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, max(gtx.Constraints.Min.X, reservedWidth+iconWidth))
				labelWidget := func(gtx layout.Context) layout.Dimensions {
					labelStyle := r.materialTextLabel(label, "control", false)
					if role == "select" {
						labelStyle.Color = r.palette.text
					} else if !options.Enabled {
						labelStyle.Color = r.palette.muted
					} else {
						labelStyle.Color = r.palette.accentStrong
					}
					labelStyle.MaxLines = 1
					return labelStyle.Layout(gtx)
				}
				labelSlot := func(gtx layout.Context) layout.Dimensions {
					width := min(gtx.Constraints.Max.X, reservedWidth)
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					if role == "select" {
						return layout.W.Layout(gtx, labelWidget)
					}
					return layout.Center.Layout(gtx, labelWidget)
				}
				glyph := func(gtx layout.Context) layout.Dimensions {
					return r.layoutGlyph(gtx, iconName, inkTone, unit.Dp(iconSize))
				}
				gap := func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{Width: unit.Dp(iconGap)}.Layout(gtx)
				}
				children := make([]layout.FlexChild, 0, 3)
				if iconName != "" && !trailingIcon {
					children = append(children, layout.Rigid(glyph), layout.Rigid(gap))
				}
				if trailingIcon && options.FillWidth {
					children = append(children, layout.Flexed(1, labelWidget))
				} else {
					children = append(children, layout.Rigid(labelSlot))
				}
				if iconName != "" && trailingIcon {
					children = append(children, layout.Rigid(gap), layout.Rigid(glyph))
				}
				spacing := layout.SpaceStart
				if role != "select" {
					spacing = layout.SpaceSides
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: spacing}.Layout(gtx, children...)
			})
		})
	})
}

func (r *Renderer) domWidestControlLabel(gtx layout.Context, labels []string) int {
	widest := 0
	for _, value := range labels {
		measure := gtx
		measure.Constraints.Min = image.Point{}
		macro := op.Record(gtx.Ops)
		label := r.materialTextLabel(value, "control", false)
		label.MaxLines = 1
		dimensions := label.Layout(measure)
		_ = macro.Stop()
		widest = max(widest, dimensions.Size.X)
	}
	return widest
}

func (r *Renderer) domButtonReservedLabels(node uidsl.Node, data any, displayed string) []string {
	labels := []string{displayed}
	if node.Text != nil {
		if ordinary, err := uidsl.RenderText(data, *node.Text); err == nil {
			labels = append(labels, ordinary)
		}
	}
	if len(node.Actions) > 0 {
		r.mu.RLock()
		catalog := r.actionCatalog
		r.mu.RUnlock()
		if spec, ok := catalog.Spec(node.Actions[0].Command); ok && strings.TrimSpace(spec.Pending) != "" {
			labels = append(labels, spec.Pending)
		}
	}
	return labels
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
			minimum := gtx.Dp(unit.Dp(r.controls.Input.MinimumHeight.Native))
			if node.Input.Multiline {
				gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, minimum))
			} else {
				height := min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, minimum))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			}
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				border := r.palette.border
				if gtx.Focused(&state.editor) {
					border = r.palette.accent
				}
				paintDOMSurface(gtx, size, r.palette.surface, border, gtx.Dp(1), gtx.Dp(r.metrics.controlRadius))
				return layout.Dimensions{Size: size}
			}, func(gtx layout.Context) layout.Dimensions {
				paddingX := unit.Dp(r.controls.Input.PaddingX.Native)
				paddingY := unit.Dp(r.controls.Input.PaddingY.Native)
				return layout.Inset{
					Top: paddingY, Right: paddingX,
					Bottom: paddingY, Left: paddingX,
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
					if !node.Input.Multiline {
						// Keep the outer input at its shared control height, but allow the
						// editor to report its intrinsic line height. Background.Layout then
						// centers that line within the complete input surface.
						gtx.Constraints.Min.Y = 0
					}
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
		labels := make([]string, 0, len(options)+1)
		labels = append(labels, selectedLabel)
		for _, option := range options {
			labels = append(labels, option.label)
		}
		return r.layoutDOMControlWithOptions(gtx, &state.toggle, selectedLabel, icon, "select", "muted", domControlOptions{
			Enabled: enabled, FillWidth: node.Layout.Grow,
			MinimumWidth: r.domLayoutDimension(node.Layout.MinWidth), MinimumHeight: r.domLayoutDimension(node.Layout.MinHeight),
			TrailingIcon: true, ReservedLabels: labels,
		})
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
					border := color.NRGBA{}
					if button.Hovered() || gtx.Focused(button) {
						fill, border = r.palette.subtle, r.palette.accent
					}
					gtx.Constraints.Min.Y = min(gtx.Constraints.Max.Y, max(gtx.Constraints.Min.Y, gtx.Dp(40)))
					return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						size := gtx.Constraints.Min
						paintDOMSurface(gtx, size, fill, border, gtx.Dp(1), gtx.Dp(r.metrics.controlRadius))
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
			} else if node.Style.Role == "execution-row-image" {
				width, height = 28, 28
			}
			return r.layoutImageSource(gtx, source, node.Image.Description, width, height)
		},
	})
}

func (r *Renderer) compileDOMIcon(node uidsl.Node, data any, path string) giodom.Element {
	return giodom.Native(domNodeKey(node, path), giodom.NativeProps{
		Layout: func(gtx layout.Context, _ any) layout.Dimensions {
			if node.Style.Role == "execution-row-status" {
				return r.layoutGlyph(gtx, node.Icon, node.Style.Tone, 20)
			}
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

func domPassiveDisclosureSummary(element giodom.Element) giodom.Element {
	if element.Kind == giodom.KindText {
		element.Text.Selectable = false
		return element
	}
	// Native leaves include explicit actions as well as passive images and
	// icons. Keep their event behavior intact so an action nested in a summary
	// remains the foremost hit target.
	if element.Kind == giodom.KindButton || element.Kind == giodom.KindEditor || element.Kind == giodom.KindNative {
		return element
	}
	if element.Responsive.Compact != nil {
		compact := domPassiveDisclosureSummary(*element.Responsive.Compact)
		element.Responsive.Compact = &compact
	}
	if element.Responsive.Wide != nil {
		wide := domPassiveDisclosureSummary(*element.Responsive.Wide)
		element.Responsive.Wide = &wide
	}
	if element.Children == nil {
		return element
	}
	children := make([]giodom.Element, 0, element.Children.Len())
	for index := 0; index < element.Children.Len(); index++ {
		children = append(children, domPassiveDisclosureSummary(element.Children.At(index)))
	}
	if element.Children.Dynamic() {
		element.Children = giodom.Keyed(element.Children.Revision(), children...)
	} else {
		element.Children = giodom.Static(children...)
	}
	return element
}

func (r *Renderer) compileDOMDisclosure(node uidsl.Node, data any, path string, childStyle domStyleContext) giodom.Element {
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
	toggleNode := uidsl.Node{
		Component: "button", Text: &uidsl.Text{Literal: label}, Icon: icon,
		Style: uidsl.Style{Role: "disclosure-chevron", Tone: node.Style.Tone},
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
				return r.layoutDOMControl(gtx, &state.clickable, label, icon, "disclosure-chevron", node.Style.Tone)
			}
		}(header.Native.Layout)
	}
	summary := []giodom.Element{}
	if node.Style.Role == "execution-row" {
		if node.Image != nil {
			imageNode := uidsl.Node{Component: "image", Image: node.Image, Style: uidsl.Style{Role: "execution-row-image"}}
			summary = append(summary, r.compileDOMImage(imageNode, data, path+"/summary/image"))
		}
		statusIcon, statusTone := "status-waiting", "warning"
		switch node.Style.Tone {
		case "success":
			statusIcon, statusTone = "status-success", "success"
		case "danger":
			statusIcon, statusTone = "status-danger", "danger"
		case "accent":
			statusIcon, statusTone = "loader-2", "warning"
		}
		statusNode := uidsl.Node{Component: "icon", Icon: statusIcon, Style: uidsl.Style{Role: "execution-row-status", Tone: statusTone}}
		summary = append(summary,
			r.compileDOMIcon(statusNode, data, path+"/summary/status"),
			r.domText(giodom.Key(path+"/summary/label"), label, "control", false, "", true),
		)
	}
	if node.Disclosure != nil {
		summary = append(summary, r.compileDOMChildren(node.Disclosure.Summary, data, path+"/summary", childStyle)...)
	}
	if !iconOnlyToggle {
		role, tone := "body", "accent"
		if node.Style.Role == "output-group" {
			role, tone = "output-summary", "console-accent"
		}
		labelElement := r.domText(giodom.Key(path+"/summary/label"), label, role, true, tone, false)
		summary = append([]giodom.Element{labelElement}, summary...)
	}
	if r.controls.Disclosure.ChevronPosition == "leading" {
		summary = append([]giodom.Element{header}, summary...)
	} else {
		summary = append(summary, header)
	}
	headerRow := giodom.Element{
		Kind: giodom.KindFlex, Key: giodom.Key(path + "/header"),
		Flex: giodom.FlexProps{
			Axis: layout.Horizontal, Alignment: layout.Middle, Gap: unit.Dp(r.controls.Disclosure.ChevronGap),
			Wrap: node.Style.Role == "project-row",
		},
		Children: giodom.Static(summary...),
	}
	activateHeader := func(header giodom.Element) giodom.Element {
		header = domPassiveDisclosureSummary(header)
		return giodom.Control(giodom.Key(path+"/summary-activate"), giodom.ButtonProps{
			Enabled: true, Description: label,
			OnClick: func() {
				if navigatePresentation && hasNavigateAction {
					r.dispatch(navigateAction, data)
					return
				}
				r.setDisclosureState(stateKey, !expanded, persistent)
			},
		}, header)
	}
	bodyChildren := []giodom.Element{}
	if expanded && !navigatePresentation {
		bodyChildren = r.compileDOMChildren(node.Children, data, path+"/body", childStyle)
	}
	if progress := r.domProgressProps(node, data); progress != nil {
		padding := r.spacing(node.Layout.Padding)
		if node.Layout.Padding == "" {
			padding = r.metrics.sectionPadding
		}
		headerContent := giodom.Inset(giodom.Key(path+"/progress-header-inset"), giodom.UniformInsets(padding), headerRow)
		headerProgress := *progress
		headerProgress.Track = color.NRGBA{}
		header := giodom.Progress(giodom.Key(path+"/progress-header"), headerProgress, headerContent)
		header = activateHeader(header)
		content := []giodom.Element{header}
		if len(bodyChildren) > 0 {
			body := giodom.Element{
				Kind: giodom.KindFlex, Key: giodom.Key(path + "/body"),
				Flex:     giodom.FlexProps{Axis: layout.Vertical, Alignment: layout.Start, Gap: r.spacing(node.Layout.Gap)},
				Children: giodom.Static(bodyChildren...),
			}
			body = giodom.Inset(giodom.Key(path+"/body-inset"), giodom.Insets{Top: padding, Right: padding, Bottom: padding, Left: padding}, body)
			content = append(content, body)
		}
		return giodom.Element{
			Kind: giodom.KindFlex, Key: domNodeKey(node, path),
			Flex:     giodom.FlexProps{Axis: layout.Vertical, Alignment: layout.Start},
			Children: giodom.Static(content...),
		}
	}
	content := append([]giodom.Element{activateHeader(headerRow)}, bodyChildren...)
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

func (r *Renderer) compileDOMTree(node uidsl.Node, data any, path string, inherited domStyleContext) giodom.Element {
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
		compiled := r.compileDOMNodeWithStyle(entry, itemData, fmt.Sprintf("%s/%d:%s", path, index, key), inherited)
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

func (r *Renderer) compileDOMScroller(node uidsl.Node, data any, path string, inherited domStyleContext) *giodom.Element {
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
		compiled := r.compileDOMContainer(container, itemData, path+"/"+string(key), inherited)
		compiled.Key = key
		return compiled
	}
	children := giodom.Lazy(revision, len(items), keyAt, build)
	viewport := unit.Dp(0)
	if axis == layout.Vertical && node.Layout.MaxHeight != "" {
		parsed, _ := strconv.ParseFloat(node.Layout.MaxHeight, 32)
		viewport = unit.Dp(parsed)
	}
	if node.Layout.MaxWidth != "" && axis == layout.Horizontal {
		parsed, _ := strconv.ParseFloat(node.Layout.MaxWidth, 32)
		viewport = unit.Dp(parsed)
	}
	if node.ID == "job-output-groups" && viewport > 2*r.metrics.spaceSmall && !r.compact {
		viewport -= 2 * r.metrics.spaceSmall
	}
	scrollTarget := giodom.Key("")
	scrollRevision := uint64(0)
	if node.ID == "job-output-groups" && r.pendingOutputScroll != "" {
		scrollTarget = giodom.Key(r.pendingOutputScroll)
		scrollRevision = r.outputScrollRevision
		r.pendingOutputScroll = ""
	}
	result := giodom.VirtualList(domNodeKey(node, path), giodom.ListProps{
		Axis: axis, Gap: r.spacing(node.Layout.Gap), Viewport: viewport, ShrinkCross: axis == layout.Horizontal,
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
