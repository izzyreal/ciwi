//go:build darwin || linux || windows

package gio

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type ActionHandler func(uidsl.Action, map[string]string)

type Renderer struct {
	mu                     sync.RWMutex
	screen                 *uidsl.ScreenDocument
	data                   any
	status                 string
	statusExpires          time.Time
	theme                  *material.Theme
	palette                palette
	themeName              string
	pendingTheme           *material.Theme
	pendingPalette         *palette
	pendingThemeName       string
	list                   layout.List
	buttons                map[string]*widget.Clickable
	disclosures            map[string]bool
	persistentDisclosures  map[string]bool
	onDisclosureChange     func(map[string]bool)
	selectables            map[string]*widget.Selectable
	textEditors            map[string]*widget.Editor
	inputEditors           map[string]*widget.Editor
	selectOpen             map[string]bool
	scrollers              map[string]*layout.List
	icons                  map[string]*widget.Icon
	images                 map[string]paint.ImageOp
	statusText             widget.Editor
	shownStatus            string
	onAction               ActionHandler
	invalidate             func()
	pending                *pendingConfirmation
	resetScroll            bool
	outputTailing          bool
	outputSearch           string
	outputMatch            int
	outputEditors          map[string]*widget.Editor
	outputScroller         *layout.List
	pendingOutputSelection *outputSelection
	renderedJobID          string
}

type outputSelection struct {
	itemID string
	start  int
	end    int
}

type jobOutputSnapshot struct {
	System    string
	Outputs   map[string]string
	Errors    map[string]string
	ExitCodes map[string]string
}

type pendingConfirmation struct {
	action    uidsl.Action
	arguments map[string]string
	title     string
	message   string
}

type palette struct {
	background, backgroundEnd, heroStart, heroEnd, surface, subtle, text, muted, accent, border, success, warning, danger, focus color.NRGBA
}

func NewRenderer(screen *uidsl.ScreenDocument, theme *uidsl.ThemeDocument, onAction ActionHandler) (*Renderer, error) {
	if screen == nil || theme == nil {
		return nil, fmt.Errorf("screen and theme are required")
	}
	materialTheme, colors, err := rendererTheme(theme)
	if err != nil {
		return nil, err
	}
	iconSet, err := materialIcons()
	if err != nil {
		return nil, err
	}
	imageSet, err := embeddedImages()
	if err != nil {
		return nil, err
	}
	renderer := &Renderer{
		screen: screen, theme: materialTheme, palette: colors, themeName: theme.Metadata.Name, onAction: onAction,
		list: layout.List{Axis: layout.Vertical}, buttons: map[string]*widget.Clickable{}, disclosures: map[string]bool{},
		persistentDisclosures: map[string]bool{},
		selectables:           map[string]*widget.Selectable{}, textEditors: map[string]*widget.Editor{}, inputEditors: map[string]*widget.Editor{},
		selectOpen: map[string]bool{}, scrollers: map[string]*layout.List{}, outputEditors: map[string]*widget.Editor{},
		icons: iconSet, images: imageSet,
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
	if screen != nil && screen.Metadata.Name == "job-details" {
		preserveJobUIState(r.data, data)
	}
	if screen != nil && screen.Metadata.Name == "settings" {
		preserveSettingsUIState(r.data, data)
	}
	if r.screen == nil || screen == nil || r.screen.Metadata.Name != screen.Metadata.Name {
		r.resetScroll = true
	}
	r.screen = screen
	r.data = data
	r.mu.Unlock()
}

func (r *Renderer) SetRepeatedItemBinding(root, collection, keyField, keyValue, field string, value any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	rootData, ok := data[root].(map[string]any)
	if !ok {
		return false
	}
	items, ok := rootData[collection].([]any)
	if !ok {
		return false
	}
	nextItems := append([]any(nil), items...)
	found := false
	for index, raw := range items {
		item, itemOK := raw.(map[string]any)
		if !itemOK || fmt.Sprint(item[keyField]) != keyValue {
			continue
		}
		nextItem := make(map[string]any, len(item)+1)
		for existingKey, existingValue := range item {
			nextItem[existingKey] = existingValue
		}
		nextItem[field] = value
		nextItems[index] = nextItem
		found = true
		break
	}
	if !found {
		return false
	}
	nextRoot := make(map[string]any, len(rootData))
	for existingKey, existingValue := range rootData {
		nextRoot[existingKey] = existingValue
	}
	nextRoot[collection] = nextItems
	nextData := make(map[string]any, len(data))
	for existingKey, existingValue := range data {
		nextData[existingKey] = existingValue
	}
	nextData[root] = nextRoot
	r.data = nextData
	return true
}

func (r *Renderer) ApplyJobOutput(snapshot jobOutputSnapshot) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	rootData, ok := data["jobDetails"].(map[string]any)
	if !ok {
		return false
	}
	groups, ok := rootData["output_groups"].([]any)
	if !ok {
		return false
	}
	nextGroups := make([]any, 0, len(groups))
	for _, raw := range groups {
		group, groupOK := raw.(map[string]any)
		if !groupOK {
			nextGroups = append(nextGroups, raw)
			continue
		}
		nextGroup := make(map[string]any, len(group)+3)
		for key, value := range group {
			nextGroup[key] = value
		}
		itemID := fmt.Sprint(group["id"])
		nextGroup["output"] = snapshot.Outputs[itemID]
		if value := snapshot.Errors[itemID]; value != "" {
			nextGroup["error"] = value
			nextGroup["status"] = "failed"
			nextGroup["status_label"] = "Failed"
		}
		if value := snapshot.ExitCodes[itemID]; value != "" {
			nextGroup["exit_code"] = value
		}
		nextGroups = append(nextGroups, nextGroup)
	}
	nextRoot := make(map[string]any, len(rootData)+3)
	for key, value := range rootData {
		nextRoot[key] = value
	}
	nextRoot["system_output"] = snapshot.System
	nextRoot["output_groups"] = nextGroups
	nextRoot["output"] = structuredOutputPlainText(nextRoot, nextGroups, snapshot.System)
	nextData := make(map[string]any, len(data))
	for key, value := range data {
		nextData[key] = value
	}
	nextData["jobDetails"] = nextRoot
	if r.outputSearch != "" {
		matches := groupedOutputMatches(nextData, r.outputSearch)
		if len(matches) == 0 {
			r.outputMatch = 0
			nextRoot["output_search_count"] = "0/0"
		} else {
			if r.outputMatch >= len(matches) {
				r.outputMatch = 0
			}
			nextRoot["output_search_count"] = fmt.Sprintf("%d/%d", r.outputMatch+1, len(matches))
		}
	}
	r.data = nextData
	return true
}

func structuredOutputPlainText(root map[string]any, groups []any, systemOutput string) string {
	var out strings.Builder
	out.WriteString("ciwi job log\n")
	out.WriteString("Job execution ID: " + fmt.Sprint(root["id"]) + "\n")
	out.WriteString("Status: " + fmt.Sprint(root["status"]) + "\n\n")
	if strings.TrimSpace(systemOutput) != "" {
		out.WriteString(strings.TrimRight(systemOutput, "\n") + "\n\n")
	}
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out.WriteString("--------------------------------------------------------------------------------\n")
		out.WriteString(fmt.Sprint(group["title"]) + "\n")
		out.WriteString("--------------------------------------------------------------------------------\n")
		if fmt.Sprint(group["reached"]) != "true" {
			out.WriteString("Status: Not reached\n")
		}
		for _, field := range []struct{ key, label string }{{"started", "Started"}, {"duration", "Duration"}, {"exit_code", "Exit code"}, {"error", "Error"}} {
			if value := strings.TrimSpace(fmt.Sprint(group[field.key])); value != "" {
				out.WriteString(field.label + ": " + value + "\n")
			}
		}
		out.WriteString("\n")
		if fmt.Sprint(group["kind"]) == "phase" {
			out.WriteString("Details:\n" + fmt.Sprint(group["details"]) + "\n")
		} else {
			out.WriteString("YAML literal:\n'''\n" + fmt.Sprint(group["yaml_literal"]) + "\n'''\n\n")
			out.WriteString("Expanded command:\n'''\n" + fmt.Sprint(group["expanded_command"]) + "\n'''\n")
		}
		out.WriteString("\nOutput:\n'''\n")
		output := fmt.Sprint(group["output"])
		if output == "" && fmt.Sprint(group["reached"]) != "true" {
			output = "(step was not reached)"
		}
		out.WriteString(output + "\n'''\n\n")
	}
	return out.String()
}

func (r *Renderer) SetStatus(status string) {
	r.mu.Lock()
	r.status = status
	r.statusExpires = time.Time{}
	r.mu.Unlock()
}

func (r *Renderer) SetTransientStatus(status string, duration time.Duration) {
	r.mu.Lock()
	r.status = status
	if duration > 0 {
		r.statusExpires = time.Now().Add(duration)
	} else {
		r.statusExpires = time.Time{}
	}
	r.mu.Unlock()
}

func (r *Renderer) StatusExpiry() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.statusExpires
}

func (r *Renderer) ClearExpiredStatus(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.statusExpires.IsZero() || now.Before(r.statusExpires) {
		return false
	}
	r.status = ""
	r.statusExpires = time.Time{}
	return true
}

func (r *Renderer) SetTheme(theme *uidsl.ThemeDocument) error {
	materialTheme, colors, err := rendererTheme(theme)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.pendingTheme = materialTheme
	r.pendingPalette = &colors
	r.pendingThemeName = theme.Metadata.Name
	r.mu.Unlock()
	return nil
}

func (r *Renderer) ThemeName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.pendingThemeName != "" {
		return r.pendingThemeName
	}
	return r.themeName
}

func (r *Renderer) SetRootBinding(root, key string, value any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.data.(map[string]any)
	if !ok {
		return false
	}
	rootData, ok := data[root].(map[string]any)
	if !ok {
		return false
	}
	nextRoot := make(map[string]any, len(rootData)+1)
	for existingKey, existingValue := range rootData {
		nextRoot[existingKey] = existingValue
	}
	nextRoot[key] = value
	nextData := make(map[string]any, len(data))
	for existingKey, existingValue := range data {
		nextData[existingKey] = existingValue
	}
	nextData[root] = nextRoot
	r.data = nextData
	return true
}

func (r *Renderer) SetInvalidate(invalidate func()) {
	r.invalidate = invalidate
}

func (r *Renderer) SetDisclosureStates(states map[string]bool) {
	for key, expanded := range states {
		if strings.TrimSpace(key) != "" {
			r.disclosures[key] = expanded
		}
	}
}

func (r *Renderer) SetDisclosureChange(handler func(map[string]bool)) {
	r.onDisclosureChange = handler
}

func (r *Renderer) Layout(gtx layout.Context) layout.Dimensions {
	r.mu.Lock()
	if r.pendingTheme != nil && r.pendingPalette != nil {
		r.theme = r.pendingTheme
		r.palette = *r.pendingPalette
		r.themeName = r.pendingThemeName
		r.pendingTheme = nil
		r.pendingPalette = nil
		r.pendingThemeName = ""
	}
	screen, data, status, resetScroll := r.screen, r.data, r.status, r.resetScroll
	r.resetScroll = false
	r.mu.Unlock()
	if resetScroll {
		r.list.Position = layout.Position{}
	}
	if screen != nil && screen.Metadata.Name == "job-details" {
		jobID := bindingString(data, "jobDetails.id")
		if jobID != r.renderedJobID {
			r.renderedJobID = jobID
			r.outputTailing = false
			r.outputSearch = ""
			r.outputMatch = 0
			r.outputEditors = map[string]*widget.Editor{}
			r.outputScroller = nil
			r.pendingOutputSelection = nil
		}
	}
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
	return layoutModalOverlay(gtx, body, r.layoutConfirmation)
}

func layoutModalOverlay(gtx layout.Context, body, confirmation layout.Widget) layout.Dimensions {
	viewportConstraints := gtx.Constraints
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			// Stack removes the minimum constraints from stacked children. Restore
			// the viewport constraints so showing a modal cannot reflow the page
			// underneath it.
			gtx.Constraints = viewportConstraints
			return body(gtx)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.Fill(gtx.Ops, color.NRGBA{A: 0x70})
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(confirmation),
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
		equal := conditionEqual(node.Visible, value)
		if (!node.Visible.Not && !equal) || (node.Visible.Not && equal) {
			return layout.Dimensions{}
		}
	}
	if node.Component == "scroller" {
		return r.constrainNode(gtx, node, func(gtx layout.Context) layout.Dimensions {
			return r.layoutScroller(gtx, node, data, path)
		})
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
	if node.Style.ToneBinding != "" {
		value, err := uidsl.Resolve(data, node.Style.ToneBinding)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		node.Style.Tone = semanticTone(fmt.Sprint(value))
	}

	content := func(gtx layout.Context) layout.Dimensions {
		if node.Component == "disclosure" {
			return r.layoutDisclosure(gtx, node, data, path)
		}
		if node.Component == "text" {
			return r.layoutText(gtx, node, data, path)
		}
		if node.Component == "badge" {
			return r.layoutBadge(gtx, node, data, path)
		}
		if node.Component == "image" {
			return r.layoutImage(gtx, node)
		}
		if node.Component == "button" {
			return r.layoutButton(gtx, node, data, path)
		}
		if node.Component == "select" {
			return r.layoutSelect(gtx, node, data, path)
		}
		if node.Component == "input" {
			return r.layoutInput(gtx, node, data, path)
		}
		return r.layoutChildren(gtx, node, data, path)
	}
	if node.Component == "card" || node.Component == "disclosure" || node.Component == "section" || node.Style.Role == "hero" {
		padding := unit.Dp(0)
		if node.Component == "section" || node.Component == "card" {
			padding = 14
			if node.Layout.Padding != "" {
				padding = 0
			}
		}
		if node.Component == "disclosure" {
			padding = 10
			if node.Layout.Padding != "" {
				padding = spacing(node.Layout.Padding)
			}
		}
		if node.Style.Role == "hero" {
			padding = 22
		}
		content = r.surface(content, padding, node.Style.Role == "hero")
	}
	widgetFn := content
	if len(node.Actions) > 0 && !componentHandlesOwnActions(node.Component) {
		button := r.button(path)
		for button.Clicked(gtx) {
			r.dispatchFromLayout(gtx, node.Actions[0], data)
		}
		widgetFn = func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, content)
		}
	}
	return r.constrainNode(gtx, node, widgetFn)
}

func componentHandlesOwnActions(component string) bool {
	switch component {
	case "button", "select", "input":
		return true
	default:
		return false
	}
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
	stateKey, persistent := r.disclosureStateKey(node, data, path)
	expanded, exists := r.disclosures[stateKey]
	if !exists {
		expanded = node.Disclosure != nil && node.Disclosure.DefaultExpanded
		r.disclosures[stateKey] = expanded
	}
	if persistent {
		r.persistentDisclosures[stateKey] = true
	}
	iconName := "chevron-right"
	if expanded {
		iconName = "chevron-down"
	}
	toggle := r.button(path + "/disclosure-toggle")
	for toggle.Clicked(gtx) {
		expanded = !expanded
		r.setDisclosureState(stateKey, expanded, persistent)
	}
	header := func(gtx layout.Context) layout.Dimensions {
		toggleWidget := func(gtx layout.Context) layout.Dimensions {
			description := "Expand " + label
			if expanded {
				description = "Collapse " + label
			}
			return r.layoutIconButton(gtx, toggle, iconName, description)
		}
		labelWidget := func(gtx layout.Context) layout.Dimensions {
			labelPath := path + "/label"
			labelClick := r.button(path + "/disclosure-label")
			for labelClick.Clicked(gtx) {
				if r.selectable(labelPath).SelectionLen() == 0 {
					expanded = !expanded
					r.setDisclosureState(stateKey, expanded, persistent)
				}
			}
			return labelClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					defer pointer.PassOp{}.Push(gtx.Ops).Pop()
					textNode := node
					textNode.Component = "text"
					textNode.Text = &uidsl.Text{Literal: label}
					if textNode.Style.Role == "" {
						textNode.Style.Role = "heading"
					}
					if textNode.Style.Role == "execution-row" {
						textNode.Style.Tone = ""
					}
					return r.layoutText(gtx, textNode, data, labelPath)
				})
			})
		}
		if node.Style.Role != "execution-row" {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(toggleWidget), layout.Flexed(1, labelWidget),
			)
		}
		children := make([]layout.FlexChild, 0, 4)
		if node.Image != nil {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return r.layoutImageSized(gtx, node.Image, 28, 28)
			}))
		}
		statusIcon := map[string]string{"success": "status-success", "danger": "status-danger", "warning": "status-waiting", "accent": "status-running"}[node.Style.Tone]
		if statusIcon != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 9}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return r.layoutGlyph(gtx, statusIcon, node.Style.Tone, 18)
				})
			}))
		}
		children = append(children, layout.Flexed(1, labelWidget), layout.Rigid(toggleWidget))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	}
	if !expanded {
		return header(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				contentNode := node
				contentNode.Layout.Padding = ""
				return r.layoutChildren(gtx, contentNode, data, path+"/content")
			})
		}),
	)
}

func (r *Renderer) disclosureStateKey(node uidsl.Node, data any, fallback string) (string, bool) {
	if node.Disclosure == nil || strings.TrimSpace(node.Disclosure.StateKey) == "" {
		return fallback, false
	}
	key, err := uidsl.RenderText(data, uidsl.Text{Template: node.Disclosure.StateKey})
	if err != nil || strings.TrimSpace(key) == "" {
		return fallback, false
	}
	return key, true
}

func (r *Renderer) setDisclosureState(key string, expanded, persistent bool) {
	r.disclosures[key] = expanded
	if persistent {
		r.persistentDisclosures[key] = true
		r.notifyDisclosureChange()
	}
	r.requestFrame()
}

func (r *Renderer) notifyDisclosureChange() {
	if r.onDisclosureChange == nil {
		return
	}
	states := make(map[string]bool, len(r.persistentDisclosures))
	for key := range r.persistentDisclosures {
		states[key] = r.disclosures[key]
	}
	r.onDisclosureChange(states)
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

func (r *Renderer) constrainNode(gtx layout.Context, node uidsl.Node, content layout.Widget) layout.Dimensions {
	constraints := gtx.Constraints
	if value, ok := layoutDimension(node.Layout.MinWidth, gtx); ok {
		constraints.Min.X = min(max(value, constraints.Min.X), constraints.Max.X)
	}
	if value, ok := layoutDimension(node.Layout.MaxWidth, gtx); ok {
		constraints.Max.X = max(constraints.Min.X, min(value, constraints.Max.X))
	}
	if value, ok := layoutDimension(node.Layout.MinHeight, gtx); ok {
		constraints.Min.Y = min(max(value, constraints.Min.Y), constraints.Max.Y)
	}
	if value, ok := layoutDimension(node.Layout.MaxHeight, gtx); ok {
		constraints.Max.Y = max(constraints.Min.Y, min(value, constraints.Max.Y))
	}
	gtx.Constraints = constraints
	return content(gtx)
}

func layoutDimension(value string, gtx layout.Context) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "page" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return gtx.Dp(unit.Dp(parsed)), true
}

func (r *Renderer) layoutText(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	text := "ciwi"
	if node.Text != nil {
		resolved, err := uidsl.RenderText(data, *node.Text)
		if err != nil {
			return r.errorLabel(gtx, err)
		}
		text = resolved
	}
	if node.Style.Role == "code" {
		editor := r.textEditors[path]
		if editor == nil {
			editor = &widget.Editor{ReadOnly: true}
			r.textEditors[path] = editor
		}
		outputChanged := editor.Text() != text
		if outputChanged {
			editor.SetText(text)
			if node.ID == "job-output-text" && r.outputTailing {
				runeCount := utf8.RuneCountInString(text)
				editor.SetCaret(runeCount, runeCount)
			}
		}
		outputItemID := ""
		if node.ID == "job-output-group-text" {
			if value, resolveErr := uidsl.Resolve(data, "outputGroup.id"); resolveErr == nil {
				outputItemID = fmt.Sprint(value)
				r.outputEditors[outputItemID] = editor
				if outputChanged && r.outputTailing {
					runeCount := utf8.RuneCountInString(text)
					editor.SetCaret(runeCount, runeCount)
				}
			}
		} else if node.ID == "job-output-system-text" {
			r.outputEditors[""] = editor
		}
		if pending := r.pendingOutputSelection; pending != nil && pending.itemID == outputItemID && (node.ID == "job-output-group-text" || node.ID == "job-output-system-text") {
			editor.SetCaret(pending.start, pending.end)
			r.pendingOutputSelection = nil
		}
		style := material.Editor(r.theme, editor, "")
		style.Font.Typeface = font.Typeface("Go Mono")
		style.TextSize = unit.Sp(13)
		style.Color = r.palette.text
		style.SelectionColor = r.palette.focus
		style.SelectionColor.A = 0xc0
		return layout.UniformInset(12).Layout(gtx, style.Layout)
	}
	var label material.LabelStyle
	switch node.Style.Role {
	case "title":
		label = material.H4(r.theme, text)
		label.TextSize = unit.Sp(28)
	case "heading":
		label = material.H6(r.theme, text)
		label.TextSize = unit.Sp(18)
	case "subtitle":
		label = material.Subtitle1(r.theme, text)
		label.TextSize = unit.Sp(16)
	case "badge":
		label = material.Body2(r.theme, text)
		label.TextSize = unit.Sp(12)
	case "execution-row":
		label = material.Body1(r.theme, text)
		label.TextSize = unit.Sp(14)
	default:
		label = material.Body1(r.theme, text)
		label.TextSize = unit.Sp(14)
	}
	if tone, ok := r.toneColor(node.Style.Tone); ok {
		label.Color = tone
	}
	if node.Style.Emphasis == "strong" {
		label.Font.Weight = font.Bold
	}
	if node.Style.Truncate {
		label.MaxLines = 1
	}
	label.State = r.selectable(path)
	return label.Layout(gtx)
}

func (r *Renderer) layoutBadge(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	tone, ok := r.toneColor(node.Style.Tone)
	if !ok {
		tone = r.palette.accent
	}
	background := tone
	background.A = 0x24
	border := tone
	border.A = 0x90
	const badgeRadius unit.Dp = 12
	node.Style.Role = "badge"
	return widget.Border{Color: border, CornerRadius: badgeRadius, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(badgeRadius)).Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 4, Right: 9, Bottom: 4, Left: 9}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return r.layoutText(gtx, node, data, path+"/text")
			})
		})
	})
}

func (r *Renderer) toneColor(tone string) (color.NRGBA, bool) {
	switch tone {
	case "muted":
		return r.palette.muted, true
	case "accent":
		return r.palette.accent, true
	case "success":
		return r.palette.success, true
	case "warning":
		return r.palette.warning, true
	case "danger":
		return r.palette.danger, true
	case "focus":
		return r.palette.focus, true
	default:
		return color.NRGBA{}, false
	}
}

func (r *Renderer) layoutImage(gtx layout.Context, node uidsl.Node) layout.Dimensions {
	if node.Image == nil {
		return r.errorLabel(gtx, fmt.Errorf("image source is missing"))
	}
	return r.layoutImageSized(gtx, node.Image, 132, 110)
}

func (r *Renderer) layoutImageSized(gtx layout.Context, description *uidsl.Image, width, height unit.Dp) layout.Dimensions {
	if description == nil {
		return r.errorLabel(gtx, fmt.Errorf("image source is missing"))
	}
	source, ok := r.images[description.Asset]
	if !ok {
		return r.errorLabel(gtx, fmt.Errorf("image asset %q is unavailable", description.Asset))
	}
	semantic.DescriptionOp(description.Description).Add(gtx.Ops)
	size := gtx.Constraints.Constrain(image.Pt(gtx.Dp(width), gtx.Dp(height)))
	gtx.Constraints = layout.Exact(size)
	return widget.Image{Src: source, Fit: widget.Contain, Position: layout.Center, Scale: 1}.Layout(gtx)
}

func (r *Renderer) layoutGlyph(gtx layout.Context, iconName, tone string, size unit.Dp) layout.Dimensions {
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
	}
	iconColor, ok := r.toneColor(tone)
	if !ok {
		iconColor = r.palette.accent
	}
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(size), gtx.Dp(size)))
	return icon.Layout(gtx, iconColor)
}

func (r *Renderer) layoutButton(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	label := "Run"
	if node.Text != nil {
		if resolved, err := uidsl.RenderText(data, *node.Text); err == nil {
			label = resolved
		}
	}
	enabled := conditionEnabled(node.Enabled, data)
	button := r.button(path)
	for button.Clicked(gtx) {
		if enabled && len(node.Actions) > 0 {
			r.dispatchFromLayout(gtx, node.Actions[0], data)
		}
	}
	return r.layoutControlButton(gtx, button, label, node.Icon, enabled)
}

func (r *Renderer) layoutInput(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if node.Input == nil {
		return r.errorLabel(gtx, fmt.Errorf("input configuration is missing"))
	}
	value, err := uidsl.Resolve(data, node.Input.Value)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	editor := r.inputEditors[path]
	if editor == nil {
		editor = &widget.Editor{SingleLine: true, Submit: true}
		editor.SetText(fmt.Sprint(value))
		r.inputEditors[path] = editor
	} else if !gtx.Focused(editor) && editor.Text() != fmt.Sprint(value) {
		editor.SetText(fmt.Sprint(value))
	}
	changed := false
	submitted := false
	for {
		event, ok := editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.ChangeEvent); ok {
			changed = true
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	if (changed || submitted) && len(node.Actions) > 0 {
		inputData := mergeData(data, "input", map[string]any{"value": editor.Text()})
		r.dispatchFromLayout(gtx, node.Actions[0], inputData)
	}
	return widget.Border{Color: r.palette.border, CornerRadius: 9, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 8, Right: 11, Bottom: 8, Left: 11}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := material.Editor(r.theme, editor, node.Input.Placeholder)
			style.TextSize = unit.Sp(14)
			style.Color = r.palette.text
			style.HintColor = r.palette.muted
			return style.Layout(gtx)
		})
	})
}

func (r *Renderer) layoutScroller(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if node.Repeat == nil {
		return r.errorLabel(gtx, fmt.Errorf("scroller repeat configuration is missing"))
	}
	items, err := resolveItems(data, node.Repeat.Source)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	list := r.scrollers[path]
	if list == nil {
		axis := layout.Horizontal
		if node.Layout.Direction == "vertical" {
			axis = layout.Vertical
		}
		list = &layout.List{Axis: axis}
		r.scrollers[path] = list
	}
	if node.ID == "job-output-groups" {
		r.outputScroller = list
		list.ScrollToEnd = r.outputTailing
	}
	return list.Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
		itemData := mergeData(data, node.Repeat.As, items[index])
		itemPath := fmt.Sprintf("%s/%d", path, index)
		if key, resolveErr := uidsl.Resolve(itemData, node.Repeat.Key); resolveErr == nil {
			itemPath = path + "/" + fmt.Sprint(key)
		}
		inset := layout.Inset{Right: spacing(node.Layout.Gap)}
		component := "row"
		if list.Axis == layout.Vertical {
			inset = layout.Inset{Bottom: spacing(node.Layout.Gap)}
			component = "column"
		}
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			container := node
			container.Component = component
			container.Repeat = nil
			container.Actions = nil
			return r.layoutChildren(gtx, container, itemData, itemPath)
		})
	})
}

func (r *Renderer) layoutSelect(gtx layout.Context, node uidsl.Node, data any, path string) layout.Dimensions {
	if node.Select == nil {
		return r.errorLabel(gtx, fmt.Errorf("select configuration is missing"))
	}
	enabled := conditionEnabled(node.Enabled, data)
	value, err := uidsl.Resolve(data, node.Select.Value)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	items, err := resolveItems(data, node.Select.Options)
	if err != nil {
		return r.errorLabel(gtx, err)
	}
	type option struct{ value, label string }
	options := make([]option, 0, len(items))
	selectedValue := fmt.Sprint(value)
	selectedLabel := selectedValue
	for _, item := range items {
		itemData := mergeData(data, node.Select.As, item)
		optionValue, valueErr := uidsl.Resolve(itemData, node.Select.OptionValue)
		optionLabel, labelErr := uidsl.Resolve(itemData, node.Select.OptionLabel)
		if valueErr != nil {
			return r.errorLabel(gtx, valueErr)
		}
		if labelErr != nil {
			return r.errorLabel(gtx, labelErr)
		}
		entry := option{value: fmt.Sprint(optionValue), label: fmt.Sprint(optionLabel)}
		options = append(options, entry)
		if entry.value == selectedValue {
			selectedLabel = entry.label
		}
	}
	toggle := r.button(path + "/select-toggle")
	for toggle.Clicked(gtx) {
		if enabled {
			r.selectOpen[path] = !r.selectOpen[path]
			r.requestFrame()
		}
	}
	if !enabled {
		r.selectOpen[path] = false
	}
	header := func(gtx layout.Context) layout.Dimensions {
		icon := "chevron-down"
		if r.selectOpen[path] {
			icon = "chevron-up"
		}
		return r.layoutControlButton(gtx, toggle, selectedLabel, icon, enabled)
	}
	if !r.selectOpen[path] {
		return header(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8}.Layout(gtx, r.surface(func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(options))
				for optionIndex := range options {
					entry := options[optionIndex]
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						choice := r.button(path + "/option/" + entry.value)
						for choice.Clicked(gtx) {
							r.selectOpen[path] = false
							if len(node.Actions) > 0 && entry.value != selectedValue {
								selectionData := mergeData(data, "selection", map[string]any{"value": entry.value, "label": entry.label})
								r.dispatch(node.Actions[0], selectionData)
							}
							r.requestFrame()
						}
						icon := ""
						if entry.value == selectedValue {
							icon = "check"
						}
						return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return r.layoutControlButton(gtx, choice, entry.label, icon, true)
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			}, 10, false))
		}),
	)
}

func (r *Renderer) layoutControlButton(gtx layout.Context, button *widget.Clickable, label, iconName string, enabled bool) layout.Dimensions {
	background := r.palette.surface
	borderColor := r.palette.border
	if enabled && button.Hovered() {
		background = r.palette.subtle
		borderColor = r.palette.accent
	}
	if enabled && gtx.Focused(button) {
		borderColor = r.palette.focus
	}
	return widget.Border{Color: borderColor, CornerRadius: 9, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(9)).Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 10, Right: 14, Bottom: 10, Left: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := make([]layout.FlexChild, 0, 2)
					icon := r.icons[iconName]
					if icon != nil {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(19), gtx.Dp(19)))
							iconColor := r.palette.accent
							if !enabled {
								iconColor = r.palette.muted
							}
							return icon.Layout(gtx, iconColor)
						}))
					}
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						inset := layout.Inset{}
						if icon != nil {
							inset.Left = 8
						}
						labelStyle := material.Body1(r.theme, label)
						labelStyle.TextSize = unit.Sp(14)
						labelStyle.Color = r.palette.accent
						if !enabled {
							labelStyle.Color = r.palette.muted
						}
						return inset.Layout(gtx, labelStyle.Layout)
					}))
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
			})
		})
	})
}

func conditionEnabled(condition *uidsl.Condition, data any) bool {
	if condition == nil {
		return true
	}
	value, err := uidsl.Resolve(data, condition.Binding)
	if err != nil {
		return false
	}
	equal := conditionEqual(condition, value)
	if condition.Not {
		return !equal
	}
	return equal
}

func conditionEqual(condition *uidsl.Condition, value any) bool {
	if condition != nil && condition.Empty {
		return fmt.Sprint(value) == ""
	}
	return fmt.Sprint(value) == defaultString(condition.Equals, "true")
}

func (r *Renderer) layoutIconButton(gtx layout.Context, button *widget.Clickable, iconName, description string) layout.Dimensions {
	icon := r.icons[iconName]
	if icon == nil {
		return r.errorLabel(gtx, fmt.Errorf("icon %q is unavailable", iconName))
	}
	background := r.palette.surface
	borderColor := r.palette.border
	if button.Hovered() {
		background = r.palette.subtle
		borderColor = r.palette.accent
	}
	if gtx.Focused(button) {
		borderColor = r.palette.focus
	}
	return widget.Border{Color: borderColor, CornerRadius: 9, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(9)).Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(description).Add(gtx.Ops)
				return layout.UniformInset(9).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(19), gtx.Dp(19)))
					return icon.Layout(gtx, r.palette.accent)
				})
			})
		})
	})
}

func (r *Renderer) surface(content layout.Widget, padding unit.Dp, hero bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return widget.Border{Color: r.palette.border, CornerRadius: 12, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rect := image.Rectangle{Max: gtx.Constraints.Min}
				if hero {
					stack := clip.UniformRRect(rect, gtx.Dp(12)).Push(gtx.Ops)
					paint.LinearGradientOp{
						Stop1: f32.Pt(0, 0), Color1: r.palette.heroStart,
						Stop2: f32.Pt(float32(rect.Dx()), float32(rect.Dy())), Color2: r.palette.heroEnd,
					}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
					stack.Pop()
				} else {
					paint.FillShape(gtx.Ops, r.palette.surface, clip.UniformRRect(rect, gtx.Dp(12)).Op(gtx.Ops))
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(padding).Layout(gtx, content)
			})
		})
	}
}

func (r *Renderer) dispatchFromLayout(gtx layout.Context, action uidsl.Action, data any) {
	arguments, err := actionArguments(action, data)
	if err != nil {
		r.SetStatus(err.Error())
		return
	}
	switch action.Command {
	case "select-timeline-item":
		items, resolveErr := resolveItems(data, "jobDetails.timeline")
		if resolveErr != nil {
			r.SetStatus(resolveErr.Error())
			return
		}
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if ok && fmt.Sprint(itemMap["id"]) == arguments["id"] {
				r.SetRootBinding("jobDetails", "selected_timeline_item", itemMap)
				if groups, groupErr := resolveItems(data, "jobDetails.output_groups"); groupErr == nil {
					for index, rawGroup := range groups {
						group, groupOK := rawGroup.(map[string]any)
						if !groupOK || fmt.Sprint(group["id"]) != arguments["id"] {
							continue
						}
						stateKey := fmt.Sprint(group["state_key"])
						if stateKey != "" {
							r.setDisclosureState(stateKey, true, true)
						}
						if r.outputScroller != nil {
							r.outputScroller.ScrollTo(index)
						}
						break
					}
				}
				r.SetStatus("Selected " + fmt.Sprint(itemMap["title"]))
				r.requestFrame()
				return
			}
		}
	case "change-output-search":
		r.outputSearch = arguments["query"]
		r.outputMatch = 0
		r.SetRootBinding("jobDetails", "output_search", r.outputSearch)
		r.selectGroupedOutputMatch(data, r.outputSearch, 0, true)
		r.requestFrame()
	case "find-output":
		direction := 1
		if arguments["direction"] == "previous" {
			direction = -1
		}
		query := arguments["query"]
		if query == "" {
			query = r.outputSearch
		}
		r.selectGroupedOutputMatch(data, query, direction, true)
		if pending := r.pendingOutputSelection; pending == nil {
			if matches := groupedOutputMatches(data, query); len(matches) > 0 {
				if editor := r.outputEditors[matches[r.outputMatch].itemID]; editor != nil {
					gtx.Execute(key.FocusCmd{Tag: editor})
				}
			}
		}
		r.requestFrame()
	case "copy-output":
		output, resolveErr := uidsl.Resolve(data, "jobDetails.output")
		if resolveErr != nil {
			r.SetStatus(resolveErr.Error())
			return
		}
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(fmt.Sprint(output)))})
		r.SetStatus("Output copied")
	case "toggle-output-tailing":
		r.outputTailing = !r.outputTailing
		label := "Tailing: Off"
		if r.outputTailing {
			label = "Tailing: On"
			if r.outputScroller != nil {
				r.outputScroller.ScrollToEnd = true
			}
		}
		r.SetRootBinding("jobDetails", "tailing_label", label)
		r.requestFrame()
	case "set-disclosures":
		prefix := arguments["prefix"]
		expanded, parseErr := strconv.ParseBool(arguments["expanded"])
		if parseErr != nil || prefix == "" {
			r.SetStatus("Invalid disclosure group")
			return
		}
		for key := range r.persistentDisclosures {
			if strings.HasPrefix(key, prefix) {
				r.disclosures[key] = expanded
			}
		}
		r.notifyDisclosureChange()
		r.requestFrame()
	default:
		r.dispatch(action, data)
	}
}

type groupedOutputMatch struct {
	itemID string
	index  int
	start  int
	end    int
}

func (r *Renderer) selectGroupedOutputMatch(data any, query string, direction int, selectMatch bool) {
	matches := groupedOutputMatches(data, query)
	if len(matches) == 0 {
		r.outputMatch = 0
		r.SetRootBinding("jobDetails", "output_search_count", "0/0")
		return
	}
	if direction > 0 {
		r.outputMatch = (r.outputMatch + 1) % len(matches)
	} else if direction < 0 {
		r.outputMatch = (r.outputMatch - 1 + len(matches)) % len(matches)
	} else if r.outputMatch >= len(matches) {
		r.outputMatch = 0
	}
	r.SetRootBinding("jobDetails", "output_search_count", fmt.Sprintf("%d/%d", r.outputMatch+1, len(matches)))
	if selectMatch {
		match := matches[r.outputMatch]
		if match.itemID != "" {
			if groups, err := resolveItems(data, "jobDetails.output_groups"); err == nil {
				for index, raw := range groups {
					group, ok := raw.(map[string]any)
					if !ok || fmt.Sprint(group["id"]) != match.itemID {
						continue
					}
					stateKey := fmt.Sprint(group["state_key"])
					if stateKey != "" {
						r.setDisclosureState(stateKey, true, true)
					}
					if r.outputScroller != nil {
						r.outputScroller.ScrollTo(index)
					}
					break
				}
			}
		}
		if editor := r.outputEditors[match.itemID]; editor != nil {
			editor.SetCaret(match.start, match.end)
			r.pendingOutputSelection = nil
		} else {
			r.pendingOutputSelection = &outputSelection{itemID: match.itemID, start: match.start, end: match.end}
		}
	}
}

func groupedOutputMatches(data any, query string) []groupedOutputMatch {
	if query == "" {
		return nil
	}
	sources := []struct{ itemID, text string }{}
	if system, err := uidsl.Resolve(data, "jobDetails.system_output"); err == nil && fmt.Sprint(system) != "" {
		sources = append(sources, struct{ itemID, text string }{"", fmt.Sprint(system)})
	}
	if groups, err := resolveItems(data, "jobDetails.output_groups"); err == nil {
		for _, raw := range groups {
			group, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			sources = append(sources, struct{ itemID, text string }{fmt.Sprint(group["id"]), fmt.Sprint(group["output"])})
		}
	}
	var matches []groupedOutputMatch
	for sourceIndex, source := range sources {
		for _, match := range outputMatches(source.text, query) {
			matches = append(matches, groupedOutputMatch{itemID: source.itemID, index: sourceIndex, start: match[0], end: match[1]})
		}
	}
	return matches
}

func outputMatches(output, query string) [][2]int {
	if query == "" {
		return nil
	}
	lowerOutput := strings.ToLower(output)
	lowerQuery := strings.ToLower(query)
	var matches [][2]int
	for offset := 0; offset <= len(lowerOutput)-len(lowerQuery); {
		index := strings.Index(lowerOutput[offset:], lowerQuery)
		if index < 0 {
			break
		}
		startByte := offset + index
		endByte := startByte + len(lowerQuery)
		matches = append(matches, [2]int{utf8.RuneCountInString(output[:startByte]), utf8.RuneCountInString(output[:endByte])})
		offset = endByte
	}
	return matches
}

func (r *Renderer) dispatch(action uidsl.Action, data any) {
	if r.onAction == nil {
		return
	}
	arguments, err := actionArguments(action, data)
	if err != nil {
		r.SetStatus(err.Error())
		return
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

func actionArguments(action uidsl.Action, data any) (map[string]string, error) {
	arguments := make(map[string]string, len(action.Arguments))
	for name, expression := range action.Arguments {
		value, err := uidsl.RenderText(data, uidsl.Text{Template: expression})
		if err != nil {
			return nil, err
		}
		arguments[name] = value
	}
	return arguments, nil
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
			title := material.H6(r.theme, pending.title)
			title.State = r.selectable("confirmation/title")
			message := material.Body1(r.theme, pending.message)
			message.State = r.selectable("confirmation/message")
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(title.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 12, Bottom: 20}.Layout(gtx, message.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(material.Button(r.theme, cancel, "Cancel").Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 10}.Layout(gtx, material.Button(r.theme, confirm, "Confirm").Layout)
						}),
					)
				}),
			)
		})
	}, 14, false)(gtx)
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

func (r *Renderer) selectable(key string) *widget.Selectable {
	if r.selectables == nil {
		r.selectables = map[string]*widget.Selectable{}
	}
	if selectable := r.selectables[key]; selectable != nil {
		return selectable
	}
	selectable := new(widget.Selectable)
	r.selectables[key] = selectable
	return selectable
}

func (r *Renderer) errorLabel(gtx layout.Context, err error) layout.Dimensions {
	label := material.Body2(r.theme, err.Error())
	label.Color = r.palette.danger
	label.State = r.selectable("error/" + err.Error())
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
	if override.MinHeight != "" {
		base.MinHeight = override.MinHeight
	}
	if override.MaxHeight != "" {
		base.MaxHeight = override.MaxHeight
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
	if override.ToneBinding != "" {
		base.ToneBinding = override.ToneBinding
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

func preserveJobUIState(previous, next any) {
	if bindingString(previous, "jobDetails.id") == "" || bindingString(previous, "jobDetails.id") != bindingString(next, "jobDetails.id") {
		return
	}
	previousData, previousOK := previous.(map[string]any)
	nextData, nextOK := next.(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	previousRoot, previousOK := previousData["jobDetails"].(map[string]any)
	nextRoot, nextOK := nextData["jobDetails"].(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	for _, key := range []string{"output_search", "output_search_count", "tailing_label"} {
		if value, exists := previousRoot[key]; exists {
			nextRoot[key] = value
		}
	}
	selected, ok := previousRoot["selected_timeline_item"].(map[string]any)
	if !ok {
		return
	}
	selectedID := fmt.Sprint(selected["id"])
	if timeline, ok := nextRoot["timeline"].([]any); ok {
		for _, item := range timeline {
			entry, entryOK := item.(map[string]any)
			if entryOK && fmt.Sprint(entry["id"]) == selectedID {
				nextRoot["selected_timeline_item"] = entry
				return
			}
		}
	}
}

func preserveSettingsUIState(previous, next any) {
	previousData, previousOK := previous.(map[string]any)
	nextData, nextOK := next.(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	previousRoot, previousOK := previousData["settings"].(map[string]any)
	nextRoot, nextOK := nextData["settings"].(map[string]any)
	if !previousOK || !nextOK {
		return
	}
	for _, field := range []string{
		"import_repo_url", "import_repo_ref", "import_config_file",
		"update_versions", "selected_update_version", "rollback_versions", "selected_rollback_version",
		"update_result", "update_result_tone", "rollback_result", "rollback_result_tone",
	} {
		if value, exists := previousRoot[field]; exists {
			nextRoot[field] = value
		}
	}
	selectedTarget := strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(previousRoot["selected_update_version"])), "v")
	if selectedTarget == "" {
		selectedTarget = strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(previousRoot["selected_rollback_version"])), "v")
	}
	currentVersion := strings.TrimPrefix(strings.TrimSpace(fmt.Sprint(nextRoot["update_current_version"])), "v")
	if selectedTarget != "" && currentVersion == selectedTarget {
		nextRoot["update_result"] = "Update successful."
		nextRoot["update_result_tone"] = "success"
		nextRoot["rollback_result"] = ""
		nextRoot["selected_update_version"] = ""
		nextRoot["selected_rollback_version"] = ""
		nextRoot["update_versions"] = []any{map[string]any{"value": "", "label": "Check for updates"}}
		nextRoot["rollback_versions"] = []any{map[string]any{"value": "", "label": "Refresh versions"}}
	}
	statuses := map[string]map[string]any{}
	if projects, ok := previousRoot["projects"].([]any); ok {
		for _, raw := range projects {
			project, projectOK := raw.(map[string]any)
			if !projectOK || strings.TrimSpace(fmt.Sprint(project["action_status"])) == "" {
				continue
			}
			statuses[fmt.Sprint(project["id"])] = map[string]any{
				"status": project["action_status"], "tone": project["action_tone"],
			}
		}
	}
	if projects, ok := nextRoot["projects"].([]any); ok {
		for _, raw := range projects {
			project, projectOK := raw.(map[string]any)
			if !projectOK {
				continue
			}
			if status, exists := statuses[fmt.Sprint(project["id"])]; exists {
				project["action_status"] = status["status"]
				project["action_tone"] = status["tone"]
			}
		}
	}
}

func bindingString(data any, path string) string {
	value, err := uidsl.Resolve(data, path)
	if err != nil {
		return ""
	}
	return fmt.Sprint(value)
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
		"border": &p.border, "success": &p.success, "warning": &p.warning,
		"danger": &p.danger, "focus": &p.focus,
	} {
		*target, err = get(name)
		if err != nil {
			return palette{}, fmt.Errorf("theme color %s: %w", name, err)
		}
	}
	p.backgroundEnd = p.background
	p.heroStart = p.surface
	p.heroEnd = p.subtle
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
	if gradient, ok := theme.Gradients["hero"]; ok && len(gradient.Stops) >= 2 {
		p.heroStart, err = parseColor(gradient.Stops[0].Color)
		if err != nil {
			return palette{}, fmt.Errorf("hero gradient start: %w", err)
		}
		p.heroEnd, err = parseColor(gradient.Stops[len(gradient.Stops)-1].Color)
		if err != nil {
			return palette{}, fmt.Errorf("hero gradient end: %w", err)
		}
	}
	return p, nil
}

func rendererTheme(document *uidsl.ThemeDocument) (*material.Theme, palette, error) {
	if document == nil {
		return nil, palette{}, fmt.Errorf("theme is required")
	}
	colors, err := paletteFromTheme(document.Theme)
	if err != nil {
		return nil, palette{}, err
	}
	theme := material.NewTheme()
	theme.Palette.Fg = colors.text
	theme.Palette.Bg = colors.background
	theme.Palette.ContrastBg = colors.accent
	theme.Palette.ContrastFg = colors.surface
	return theme, colors, nil
}

func semanticTone(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "succeeded", "success", "passed", "complete", "completed", "online":
		return "success"
	case "failed", "failure", "error", "cancelled", "canceled", "offline":
		return "danger"
	case "queued", "waiting", "pending", "not reached", "stale":
		return "warning"
	case "running", "leased", "in progress", "active":
		return "accent"
	default:
		return "muted"
	}
}

func materialIcons() (map[string]*widget.Icon, error) {
	sources := map[string][]byte{
		"settings": icons.ActionSettings, "arrow-left": icons.NavigationArrowBack,
		"server":      icons.HardwareComputer,
		"adjustments": icons.ImageTune,
		"player-play": icons.AVPlayArrow, "chevron-right": icons.NavigationChevronRight,
		"chevron-down": icons.NavigationExpandMore, "chevron-up": icons.NavigationExpandLess,
		"check": icons.NavigationCheck, "copy": icons.ContentContentCopy,
		"trash": icons.ActionDelete, "refresh": icons.NavigationRefresh, "circle-x": icons.NavigationCancel,
		"chevrons-up": icons.NavigationUnfoldLess, "chevrons-down": icons.NavigationUnfoldMore,
		"status-success": icons.ActionCheckCircle, "status-danger": icons.AlertErrorOutline,
		"status-waiting": icons.ActionHourglassEmpty, "status-running": icons.AVPlayCircleOutline,
	}
	out := make(map[string]*widget.Icon, len(sources))
	for name, source := range sources {
		icon, err := widget.NewIcon(source)
		if err != nil {
			return nil, fmt.Errorf("load icon %q: %w", name, err)
		}
		out[name] = icon
	}
	return out, nil
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
	return map[string]paint.ImageOp{"ciwi-logo": paint.NewImageOp(decoded)}, nil
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
