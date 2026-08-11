//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

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
	iconOnlyToggle := node.Style.Role == "project-row" || node.Style.Role == "execution-row"
	icon := "chevron-right"
	if expanded {
		icon = "chevron-down"
	}
	chevron := r.domDisclosureChevron(path, icon)
	summary := []giodom.Element{}
	executionLeading := []giodom.Element{}
	executionCopy := []giodom.Element{}
	executionActions := []giodom.Element{}
	if node.Style.Role == "execution-row" {
		if node.Image != nil {
			imageNode := uidsl.Node{Component: "image", Image: node.Image, Style: uidsl.Style{Role: "execution-row-image"}}
			executionLeading = append(executionLeading, r.compileDOMImage(imageNode, data, path+"/summary/image"))
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
		executionLeading = append(executionLeading, r.compileDOMIcon(statusNode, data, path+"/summary/status"))
	}
	if node.Disclosure != nil {
		if node.Style.Role == "execution-row" {
			for index := range node.Disclosure.Summary {
				summaryNode := node.Disclosure.Summary[index]
				if summaryNode.Component == "spacer" {
					continue
				}
				compiled := r.compileDOMNodeWithStyle(summaryNode, data, fmt.Sprintf("%s/summary/%d", path, index), childStyle)
				if compiled == nil {
					continue
				}
				if summaryNode.Component == "button" {
					executionActions = append(executionActions, *compiled)
				} else {
					executionCopy = append(executionCopy, *compiled)
				}
			}
		} else {
			summary = append(summary, r.compileDOMChildren(node.Disclosure.Summary, data, path+"/summary", childStyle)...)
		}
	}
	if !iconOnlyToggle {
		role, tone := "body", "accent"
		if node.Style.Role == "output-group" {
			role, tone = "output-summary", "console-accent"
		}
		labelElement := r.domText(giodom.Key(path+"/summary/label"), label, role, true, tone, false)
		summary = append([]giodom.Element{labelElement}, summary...)
	}
	var headerRow giodom.Element
	if node.Style.Role == "execution-row" {
		headerRow = r.domExecutionDisclosureHeader(path, label, executionLeading, executionCopy, executionActions, chevron)
	} else {
		summaryContent := giodom.Element{
			Kind: giodom.KindFlex, Key: giodom.Key(path + "/header/content"), Grow: true,
			Flex: giodom.FlexProps{
				Axis: layout.Horizontal, Alignment: layout.Middle, Gap: unit.Dp(r.controls.Disclosure.ChevronGap),
				Wrap: node.Style.Role == "project-row",
			},
			Children: giodom.Static(summary...),
		}
		headerChildren := []giodom.Element{summaryContent, chevron}
		if r.controls.Disclosure.ChevronPosition == "leading" {
			headerChildren = []giodom.Element{chevron, summaryContent}
		}
		headerRow = giodom.Element{
			Kind: giodom.KindFlex, Key: giodom.Key(path + "/header"),
			Flex: giodom.FlexProps{
				Axis: layout.Horizontal, Alignment: layout.Middle, Gap: unit.Dp(r.controls.Disclosure.ChevronGap),
			},
			Children: giodom.Static(headerChildren...),
		}
	}
	activateHeader := func(header giodom.Element) giodom.Element {
		header = domPassiveDisclosureSummary(header)
		return giodom.Control(giodom.Key(path+"/summary-activate"), giodom.ButtonProps{
			Enabled: true, Description: label,
			OnClick: func() {
				r.setDisclosureState(stateKey, !expanded, persistent)
			},
		}, header)
	}
	bodyChildren := []giodom.Element{}
	if expanded {
		if node.Style.Role == "output-group" {
			bodyChildren = r.compileDOMChildrenOmittingRole(node.Children, data, path+"/body", childStyle, "floating-collapse")
		} else {
			bodyChildren = r.compileDOMChildren(node.Children, data, path+"/body", childStyle)
		}
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

func (r *Renderer) domDisclosureChevron(path, icon string) giodom.Element {
	return giodom.Native(giodom.Key(path+"/chevron"), giodom.NativeProps{
		Layout: func(gtx layout.Context, _ any) layout.Dimensions {
			return r.layoutGlyph(gtx, icon, "muted", unit.Dp(r.controls.Disclosure.ChevronSize))
		},
	})
}

func (r *Renderer) domExecutionDisclosureHeader(path, label string, leading, copyElements, actions []giodom.Element, chevron giodom.Element) giodom.Element {
	gap := unit.Dp(r.controls.Disclosure.ChevronGap)
	title := r.domText(giodom.Key(path+"/summary/label"), label, "control", false, "", false)

	narrowCopyChildren := make([]giodom.Element, 0, len(copyElements)+1)
	narrowCopyChildren = append(narrowCopyChildren, title)
	narrowCopyChildren = append(narrowCopyChildren, copyElements...)
	narrowCopy := giodom.Column(giodom.Key(path+"/header/narrow/copy"), r.metrics.spaceSmall/2, narrowCopyChildren...)
	narrowCopy.Grow = true
	narrowChildren := make([]giodom.Element, 0, len(leading)+len(actions)+2)
	narrowChildren = append(narrowChildren, leading...)
	narrowChildren = append(narrowChildren, narrowCopy)
	narrowChildren = append(narrowChildren, actions...)
	narrowChildren = append(narrowChildren, chevron)
	narrow := giodom.Element{
		Kind: giodom.KindFlex, Key: giodom.Key(path + "/header/narrow"),
		Flex:     giodom.FlexProps{Axis: layout.Horizontal, Alignment: layout.Start, Gap: gap},
		Children: giodom.Static(narrowChildren...),
	}

	wideTitle := title
	wideTitle.Grow = true
	wideChildren := make([]giodom.Element, 0, len(leading)+len(copyElements)+len(actions)+2)
	wideChildren = append(wideChildren, leading...)
	wideChildren = append(wideChildren, wideTitle)
	if len(copyElements) > 0 {
		wideCopy := giodom.Column(giodom.Key(path+"/header/wide/copy"), r.metrics.spaceSmall/2, copyElements...)
		wideCopy = giodom.Constrain(giodom.Key(path+"/header/wide/copy-width"), giodom.ConstraintProps{MinWidth: 140, MaxWidth: 220}, wideCopy)
		wideChildren = append(wideChildren, wideCopy)
	}
	wideChildren = append(wideChildren, actions...)
	wideChildren = append(wideChildren, chevron)
	wide := giodom.Element{
		Kind: giodom.KindFlex, Key: giodom.Key(path + "/header/wide"),
		Flex:     giodom.FlexProps{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gap},
		Children: giodom.Static(wideChildren...),
	}
	return giodom.Responsive(
		giodom.Key(path+"/header"), unit.Dp(r.controls.Viewport.CondensedDisclosureMaximumWidth), narrow, wide,
	)
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

func domOutputCollapseDeclaration(scroller uidsl.Node) (uidsl.Node, uidsl.Node, bool) {
	for _, child := range scroller.Children {
		if child.Component != "disclosure" || child.Style.Role != "output-group" {
			continue
		}
		for _, body := range child.Children {
			if body.Style.Role == "floating-collapse" {
				return child, body, true
			}
		}
	}
	return uidsl.Node{}, uidsl.Node{}, false
}

func (r *Renderer) compileDOMChildrenOmittingRole(nodes []uidsl.Node, data any, path string, inherited domStyleContext, role string) []giodom.Element {
	children := make([]giodom.Element, 0, len(nodes))
	for index := range nodes {
		if nodes[index].Style.Role == role {
			continue
		}
		compiled := r.compileDOMNodeWithStyle(nodes[index], data, fmt.Sprintf("%s/%d", path, index), inherited)
		if compiled == nil {
			continue
		}
		compiled.Grow = nodes[index].Layout.Grow
		children = append(children, *compiled)
	}
	return children
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
	isOutputGroups := node.ID == "job-output-groups"
	if isOutputGroups {
		viewport = r.domOutputGroupsViewport(viewport)
	}
	scrollTarget := giodom.Key("")
	scrollRevision := uint64(0)
	if node.ID == "job-output-groups" && r.pendingOutputScroll != "" {
		scrollTarget = giodom.Key(r.pendingOutputScroll)
		scrollRevision = r.outputScrollRevision
		r.pendingOutputScroll = ""
	}
	var onLeaveEnd func()
	if isOutputGroups {
		onLeaveEnd = func() {
			if !r.outputTailing {
				return
			}
			r.outputTailing = false
			r.SetRootBinding("jobDetails", "tailing_label", "Tailing: Off")
			r.SetRootBinding("jobDetails", "tailing_tone", "warning")
			r.requestFrame()
		}
	}
	var pinnedOverlay func(giodom.ListViewportItem) *giodom.Element
	if disclosureNode, collapseNode, ok := domOutputCollapseDeclaration(node); isOutputGroups && ok {
		pinnedOverlay = func(position giodom.ListViewportItem) *giodom.Element {
			if position.Index < 0 || position.Index >= len(items) || position.Extent <= position.Viewport {
				return nil
			}
			itemData := mergeData(data, repeat.As, items[position.Index])
			fallback := fmt.Sprintf("%s/%s/disclosure", path, position.Key)
			stateKey, _ := r.disclosureStateKey(disclosureNode, itemData, fallback)
			expanded, exists := r.disclosures[stateKey]
			if !exists {
				expanded = disclosureDefaultExpanded(disclosureNode.Disclosure, itemData)
			}
			if !expanded {
				return nil
			}
			return r.compileDOMNodeWithStyle(collapseNode, itemData, fallback+"/pinned-collapse", inherited)
		}
	}
	result := giodom.VirtualList(domNodeKey(node, path), giodom.ListProps{
		Axis: axis, Gap: r.spacing(node.Layout.Gap), Viewport: viewport, ShrinkCross: axis == layout.Horizontal,
		NestedScroll: isOutputGroups, Estimate: 100, Overscan: 2, MaxMeasured: 512,
		ScrollToEnd: isOutputGroups && r.outputTailing, ForceEndRevision: r.outputTailRevision, ResetRevision: r.outputResetRevision,
		ScrollTo: scrollTarget, ScrollRevision: scrollRevision, OnLeaveEnd: onLeaveEnd,
		PinnedOverlay: pinnedOverlay, PinnedAlignment: layout.NE,
		PinnedInsets: giodom.Insets{Top: r.metrics.spaceSmall, Right: r.metrics.spaceSmall},
	}, children)
	return r.decorateDOMNode(result, node, data, path)
}

func (r *Renderer) domOutputGroupsViewport(declared unit.Dp) unit.Dp {
	viewport := declared
	if r.viewportHeight > 0 {
		responsive := max(unit.Dp(240), min(unit.Dp(660), r.viewportHeight*0.7))
		if viewport == 0 || responsive < viewport {
			viewport = responsive
		}
	}
	consoleInset := r.metrics.spaceSmall + 1
	if viewport > 2*consoleInset {
		viewport -= 2 * consoleInset
	}
	return viewport
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
