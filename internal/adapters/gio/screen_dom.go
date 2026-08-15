//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

const domSemanticStateLimit = 4096

type screenDOMRenderer struct {
	theme               *material.Theme
	runtime             *giodom.Runtime
	selectDismiss       domSelectDismiss
	selectInsidePresses map[pointer.ID]struct{}
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
	dismiss domSelectDismiss
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
	// A select can live inside a later virtual-list item, whose local hit region
	// cannot reach earlier items such as the screen header. Observe presses once
	// at the screen root, then let the active select mark its own trigger/menu
	// presses during DOM layout before deciding whether the press was outside.
	rootPresses := r.dom.selectDismiss.Presses(gtx.Source)
	r.dom.selectInsidePresses = make(map[pointer.ID]struct{}, len(rootPresses))
	document := r.buildScreenDOM(screen, data, pendingScrollSection)
	document = r.decorateDOMOverlays(document, notice, alert)
	dimensions := r.dom.runtime.Layout(gtx, document)
	if r.openSelectKey != "" {
		addDOMSelectPressArea(gtx.Ops, &r.dom.selectDismiss, image.Rectangle{Max: gtx.Constraints.Max})
	}
	for _, pointerID := range rootPresses {
		if _, inside := r.dom.selectInsidePresses[pointerID]; inside {
			continue
		}
		r.openSelectKey = ""
		r.requestFrame()
		break
	}
	return dimensions
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
		giodom.Element{Kind: giodom.KindText, Key: "notice-message", Grow: true, Text: giodom.TextProps{Value: notice.message, Size: r.metrics.textBody, Color: r.palette.noticeText}},
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
		r.domText("alert-title", defaultString(alert.title, "Something went wrong"), "heading", true, "danger"),
		r.domText("alert-message", alert.message, "body", false, ""),
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
		r.domText("confirmation-title", pending.title, "heading", true, ""),
		r.domText("confirmation-message", pending.message, "body", false, ""),
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
	for index := range children {
		key := children[index].Key
		insets := giodom.Insets{Right: pageInset, Left: pageInset}
		if index == 0 {
			insets.Top = pageInset
		}
		if index == len(children)-1 {
			insets.Bottom = pageInset
		}
		row := giodom.Inset(giodom.Key("page-row-inset:"+string(key)), insets, children[index])
		if r.metrics.pageWidth > 0 {
			row = giodom.Constrain(giodom.Key("page-row-width:"+string(key)), giodom.ConstraintProps{MaxWidth: r.metrics.pageWidth}, row)
			row = giodom.Align(key, layout.N, row)
		}
		row.Key = key
		children[index] = row
	}
	props.PassThroughScroll = true
	return giodom.VirtualList(giodom.Key("page-list:"+screen.Metadata.Name), props, giodom.Keyed(domElementsRevision(children), children...))
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
	case "log-view":
		element = r.compileDOMLogView(node, data, path)
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
		label := r.domText(giodom.Key(fmt.Sprintf("%s/label/%d", path, index)), labels[index], "table-header", false, "muted")
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
