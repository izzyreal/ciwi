//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func componentHandlesOwnActions(component string) bool {
	switch component {
	case "button", "select", "input", "graph-view", "tree-view":
		return true
	default:
		return false
	}
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

func treeEntryVisible(data any, tree *uidsl.TreeView, filter string) bool {
	if tree == nil || filter == "" || filter == "all" {
		return true
	}
	if tree.FilterValues != "" {
		if raw, err := uidsl.Resolve(data, tree.FilterValues); err == nil {
			if values, ok := raw.([]any); ok && len(values) > 0 {
				matched := false
				for _, value := range values {
					matched = matched || fmt.Sprint(value) == filter
				}
				if !matched {
					return false
				}
			}
		}
	}
	children, err := resolveItems(data, tree.Children)
	if err != nil || len(children) == 0 {
		return true
	}
	for _, child := range children {
		if treeEntryVisible(mergeData(data, tree.As, child), tree, filter) {
			return true
		}
	}
	return false
}

func treeEntryNode(node uidsl.Node, data any, key string) (uidsl.Node, error) {
	tree := node.TreeView
	label, err := uidsl.RenderText(data, tree.NodeLabel)
	if err != nil {
		return uidsl.Node{}, err
	}
	detail := ""
	if tree.NodeDetail != (uidsl.Text{}) {
		detail, err = uidsl.RenderText(data, tree.NodeDetail)
		if err != nil {
			return uidsl.Node{}, err
		}
	}
	tone := ""
	if tree.NodeTone != "" {
		if value, resolveErr := uidsl.Resolve(data, tree.NodeTone); resolveErr == nil {
			tone = semanticTone(fmt.Sprint(value))
		}
	}
	link := ""
	if tree.NodeLink != "" {
		if value, resolveErr := uidsl.Resolve(data, tree.NodeLink); resolveErr == nil {
			link = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	actionLabel := ""
	if tree.ActionLabel != (uidsl.Text{}) {
		actionLabel, err = uidsl.RenderText(data, tree.ActionLabel)
		if err != nil {
			return uidsl.Node{}, err
		}
	}
	labelNode := uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: label}, Layout: uidsl.Layout{Grow: true}, Style: uidsl.Style{Role: "code-inline", Emphasis: "strong"}}
	if link != "" {
		labelNode.Style.Tone = "accent"
		labelNode.Actions = []uidsl.Action{{On: "activate", Command: "open-url", Arguments: map[string]string{"url": link}}}
	}
	summary := []uidsl.Node{labelNode}
	if detail != "" {
		summary = append(summary, uidsl.Node{Component: "text", Text: &uidsl.Text{Literal: detail}, Style: uidsl.Style{Role: "detail-small", Tone: tone}})
	}
	if actionLabel != "" && len(node.Actions) > 0 {
		summary = append(summary, uidsl.Node{Component: "button", Text: &uidsl.Text{Literal: actionLabel}, Style: uidsl.Style{Role: "tree-action"}, Actions: node.Actions})
	}
	children, _ := resolveItems(data, tree.Children)
	if len(children) == 0 {
		return uidsl.Node{Component: "row", Style: uidsl.Style{Role: "tree-row"}, Layout: uidsl.Layout{Direction: "horizontal", Gap: "small", Align: "center"}, Children: summary}, nil
	}
	defaultExpanded := false
	if tree.DefaultExpanded != "" {
		if value, resolveErr := uidsl.Resolve(data, tree.DefaultExpanded); resolveErr == nil {
			defaultExpanded, _ = value.(bool)
		}
	}
	childTree := *tree
	childTree.Nodes = tree.Children
	childNode := uidsl.Node{Component: "tree-view", TreeView: &childTree, Actions: node.Actions}
	return uidsl.Node{
		Component: "disclosure", Text: &uidsl.Text{Literal: label}, Style: uidsl.Style{Role: "tree-branch", Tone: tone},
		Disclosure: &uidsl.Disclosure{StateKey: tree.StateKey + ":" + key, DefaultExpanded: defaultExpanded, Summary: summary[1:]},
		Children:   []uidsl.Node{childNode}, Layout: uidsl.Layout{Direction: "vertical", Gap: "small", Padding: "small"},
	}, nil
}

func disclosureNavigationAction(disclosure *uidsl.Disclosure) (uidsl.Action, bool) {
	if disclosure == nil {
		return uidsl.Action{}, false
	}
	for _, summaryNode := range disclosure.Summary {
		for _, action := range summaryNode.Actions {
			if action.On == "activate" && action.Command == "navigate" {
				return action, true
			}
		}
	}
	return uidsl.Action{}, false
}

func disclosureDefaultExpanded(disclosure *uidsl.Disclosure, data any) bool {
	if disclosure == nil {
		return false
	}
	if binding := strings.TrimSpace(disclosure.DefaultExpandedBinding); binding != "" {
		value, err := uidsl.Resolve(data, binding)
		if err == nil {
			return boolValue(value)
		}
	}
	return disclosure.DefaultExpanded
}

func withDefaultConsoleText(children []uidsl.Node) []uidsl.Node {
	result := make([]uidsl.Node, len(children))
	for index := range children {
		child := children[index]
		if child.Component == "text" && child.Style.Tone == "" && child.Style.ToneBinding == "" {
			child.Style.Tone = "console-text"
		}
		if len(child.Children) > 0 {
			child.Children = withDefaultConsoleText(child.Children)
		}
		result[index] = child
	}
	return result
}

func compactNodeHasContent(node uidsl.Node, data any) bool {
	if node.Visible != nil {
		value, err := uidsl.Resolve(data, node.Visible.Binding)
		if err != nil {
			return false
		}
		equal := conditionEqual(node.Visible, value)
		if (!node.Visible.Not && !equal) || (node.Visible.Not && equal) {
			return false
		}
	}
	if node.Text != nil {
		value, err := uidsl.RenderText(data, *node.Text)
		return err == nil && strings.TrimSpace(value) != ""
	}
	if len(node.Children) > 0 {
		for _, child := range node.Children {
			if compactNodeHasContent(child, data) {
				return true
			}
		}
		return false
	}
	return node.Component != "spacer"
}

func compactRowNeedsStack(children []uidsl.Node) bool {
	buttons := 0
	for _, child := range children {
		if child.Component == "button" {
			buttons++
		}
	}
	return buttons >= 2
}

func flexAlignment(axis layout.Axis, align string, executionGrid bool) layout.Alignment {
	switch strings.ToLower(strings.TrimSpace(align)) {
	case "center", "middle":
		return layout.Middle
	case "end":
		return layout.End
	case "start":
		return layout.Start
	}
	if executionGrid {
		return layout.Start
	}
	if axis == layout.Horizontal {
		return layout.Middle
	}
	return layout.Start
}

func applyGioOverride(node uidsl.Node, compact bool) (uidsl.Node, bool) {
	hidden := false
	apply := func(override uidsl.Override) {
		hidden = hidden || override.Hidden
		if override.Layout != (uidsl.Layout{}) {
			node.Layout = mergeLayout(node.Layout, override.Layout)
		}
		if override.Style != (uidsl.Style{}) {
			node.Style = mergeStyle(node.Style, override.Style)
		}
	}
	if override, ok := node.Overrides["gio"]; ok {
		apply(override)
	}
	if compact {
		if override, ok := node.Overrides["compact"]; ok {
			apply(override)
		}
	}
	return node, hidden
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
	for _, key := range []string{"output_search", "output_search_count", "tailing_label", "tailing_tone"} {
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
		nextRoot["update_result"], nextRoot["update_result_tone"] = "Update successful.", "success"
		nextRoot["rollback_result"], nextRoot["selected_update_version"], nextRoot["selected_rollback_version"] = "", "", ""
		nextRoot["update_versions"] = []any{map[string]any{"value": "", "label": "Check for updates"}}
		nextRoot["rollback_versions"] = []any{map[string]any{"value": "", "label": "Refresh versions"}}
	}
	statuses := map[string]map[string]any{}
	if projects, ok := previousRoot["projects"].([]any); ok {
		for _, raw := range projects {
			project, projectOK := raw.(map[string]any)
			if projectOK && strings.TrimSpace(fmt.Sprint(project["action_status"])) != "" {
				statuses[fmt.Sprint(project["id"])] = map[string]any{"status": project["action_status"], "tone": project["action_tone"]}
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
				project["action_status"], project["action_tone"] = status["status"], status["tone"]
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

func semanticTone(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "succeeded", "success", "passed", "complete", "completed", "online":
		return "success"
	case "failed", "failure", "error", "cancelled", "canceled", "offline":
		return "danger"
	case "warning", "queued", "waiting", "pending", "not reached", "stale", "deactivated":
		return "warning"
	case "accent", "running", "leased", "in progress", "active":
		return "accent"
	default:
		return "muted"
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
