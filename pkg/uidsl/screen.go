// Package uidsl defines ciwi's renderer-neutral, declarative UI contract.
// It deliberately describes ciwi screens rather than exposing HTML, CSS,
// JavaScript, Gio widgets, or transport-specific messages.
package uidsl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const APIVersion = "ciwi.ui/v1"

type Metadata struct {
	Name        string `yaml:"name" json:"name"`
	Title       string `yaml:"title,omitempty" json:"title,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type ScreenDocument struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Screen     Screen   `yaml:"screen" json:"screen"`
}

type Screen struct {
	DataSources []DataSource        `yaml:"dataSources,omitempty" json:"dataSources,omitempty"`
	Persistence []Persistence       `yaml:"persistence,omitempty" json:"persistence,omitempty"`
	Root        Node                `yaml:"root" json:"root"`
	Overrides   map[string]Override `yaml:"overrides,omitempty" json:"overrides,omitempty"`
}

type DataSource struct {
	Name        string   `yaml:"name" json:"name"`
	Query       string   `yaml:"query" json:"query"`
	WatchTopics []string `yaml:"watchTopics,omitempty" json:"watchTopics,omitempty"`
}

type Persistence struct {
	Name         string `yaml:"name" json:"name"`
	StorageKey   string `yaml:"storageKey" json:"storageKey"`
	DefaultValue string `yaml:"defaultValue,omitempty" json:"defaultValue,omitempty"`
	Scope        string `yaml:"scope,omitempty" json:"scope,omitempty"`
}

type Node struct {
	Component  string              `yaml:"component" json:"component"`
	ID         string              `yaml:"id,omitempty" json:"id,omitempty"`
	Text       *Text               `yaml:"text,omitempty" json:"text,omitempty"`
	Icon       string              `yaml:"icon,omitempty" json:"icon,omitempty"`
	Image      *Image              `yaml:"image,omitempty" json:"image,omitempty"`
	Select     *Select             `yaml:"select,omitempty" json:"select,omitempty"`
	Input      *Input              `yaml:"input,omitempty" json:"input,omitempty"`
	Disclosure *Disclosure         `yaml:"disclosure,omitempty" json:"disclosure,omitempty"`
	GraphView  *GraphView          `yaml:"graphView,omitempty" json:"graphView,omitempty"`
	TreeView   *TreeView           `yaml:"treeView,omitempty" json:"treeView,omitempty"`
	Progress   *Progress           `yaml:"progress,omitempty" json:"progress,omitempty"`
	Pulse      *Pulse              `yaml:"pulse,omitempty" json:"pulse,omitempty"`
	Layout     Layout              `yaml:"layout,omitempty" json:"layout,omitempty"`
	Style      Style               `yaml:"style,omitempty" json:"style,omitempty"`
	Repeat     *Repeat             `yaml:"repeat,omitempty" json:"repeat,omitempty"`
	Visible    *Condition          `yaml:"visible,omitempty" json:"visible,omitempty"`
	Enabled    *Condition          `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Actions    []Action            `yaml:"actions,omitempty" json:"actions,omitempty"`
	Children   []Node              `yaml:"children,omitempty" json:"children,omitempty"`
	Overrides  map[string]Override `yaml:"overrides,omitempty" json:"overrides,omitempty"`
}

type Text struct {
	Literal  string `yaml:"literal,omitempty" json:"literal,omitempty"`
	Binding  string `yaml:"binding,omitempty" json:"binding,omitempty"`
	Template string `yaml:"template,omitempty" json:"template,omitempty"`
}

type Image struct {
	Asset       string `yaml:"asset,omitempty" json:"asset,omitempty"`
	Binding     string `yaml:"binding,omitempty" json:"binding,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type Select struct {
	Value       string `yaml:"value" json:"value"`
	Options     string `yaml:"options" json:"options"`
	As          string `yaml:"as" json:"as"`
	OptionValue string `yaml:"optionValue" json:"optionValue"`
	OptionLabel string `yaml:"optionLabel" json:"optionLabel"`
}

type Input struct {
	Value       string `yaml:"value" json:"value"`
	Placeholder string `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	Multiline   bool   `yaml:"multiline,omitempty" json:"multiline,omitempty"`
	MinLines    int    `yaml:"minLines,omitempty" json:"minLines,omitempty"`
}

// Progress binds a semantic progress snapshot supplied by the presentation
// layer. It is intentionally visual-policy free.
type Progress struct {
	Binding string `yaml:"binding" json:"binding"`
}

// Pulse binds an icon's opacity animation to the Unix-millisecond timestamp
// of the most recent event, such as an agent heartbeat.
type Pulse struct {
	Binding string `yaml:"binding" json:"binding"`
}

type Disclosure struct {
	DefaultExpanded     bool   `yaml:"defaultExpanded,omitempty" json:"defaultExpanded,omitempty"`
	StateKey            string `yaml:"stateKey,omitempty" json:"stateKey,omitempty"`
	CompactPresentation string `yaml:"compactPresentation,omitempty" json:"compactPresentation,omitempty"`
	Summary             []Node `yaml:"summary,omitempty" json:"summary,omitempty"`
}

// GraphView describes one definition graph. The node's children are its list
// representation; Details is rendered for the selected graph node. Renderers
// own layout, selection, zoom, scrolling, and local view persistence while the
// graph's data and actions remain renderer-neutral.
type GraphView struct {
	StateKey     string     `yaml:"stateKey" json:"stateKey"`
	DefaultMode  string     `yaml:"defaultMode,omitempty" json:"defaultMode,omitempty"`
	Nodes        string     `yaml:"nodes" json:"nodes"`
	As           string     `yaml:"as" json:"as"`
	NodeKey      string     `yaml:"nodeKey" json:"nodeKey"`
	NodeLabel    Text       `yaml:"nodeLabel" json:"nodeLabel"`
	NodeMeta     Text       `yaml:"nodeMeta,omitempty" json:"nodeMeta,omitempty"`
	Dependencies string     `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Root         *GraphRoot `yaml:"root,omitempty" json:"root,omitempty"`
	Details      []Node     `yaml:"details,omitempty" json:"details,omitempty"`
}

// GraphRoot describes an optional aggregate node rendered before the ordinary
// dependency nodes. It has its own data scope and actions and never participates
// in ordinary graph selection or details.
type GraphRoot struct {
	Binding       string     `yaml:"binding" json:"binding"`
	As            string     `yaml:"as" json:"as"`
	Key           string     `yaml:"key" json:"key"`
	Label         Text       `yaml:"label" json:"label"`
	Meta          Text       `yaml:"meta,omitempty" json:"meta,omitempty"`
	ActionVisible *Condition `yaml:"actionVisible,omitempty" json:"actionVisible,omitempty"`
	Actions       []Action   `yaml:"actions,omitempty" json:"actions,omitempty"`
}

// TreeView describes recursive hierarchical data. Labels, details, links, and
// actions remain declarative while each renderer owns expansion mechanics.
type TreeView struct {
	StateKey        string `yaml:"stateKey" json:"stateKey"`
	Nodes           string `yaml:"nodes" json:"nodes"`
	As              string `yaml:"as" json:"as"`
	NodeKey         string `yaml:"nodeKey" json:"nodeKey"`
	NodeLabel       Text   `yaml:"nodeLabel" json:"nodeLabel"`
	NodeDetail      Text   `yaml:"nodeDetail,omitempty" json:"nodeDetail,omitempty"`
	NodeTone        string `yaml:"nodeTone,omitempty" json:"nodeTone,omitempty"`
	NodeLink        string `yaml:"nodeLink,omitempty" json:"nodeLink,omitempty"`
	Children        string `yaml:"children" json:"children"`
	DefaultExpanded string `yaml:"defaultExpanded,omitempty" json:"defaultExpanded,omitempty"`
	Filter          string `yaml:"filter,omitempty" json:"filter,omitempty"`
	FilterValues    string `yaml:"filterValues,omitempty" json:"filterValues,omitempty"`
	ActionLabel     Text   `yaml:"actionLabel,omitempty" json:"actionLabel,omitempty"`
}

type Layout struct {
	Direction string `yaml:"direction,omitempty" json:"direction,omitempty"`
	Gap       string `yaml:"gap,omitempty" json:"gap,omitempty"`
	Padding   string `yaml:"padding,omitempty" json:"padding,omitempty"`
	Align     string `yaml:"align,omitempty" json:"align,omitempty"`
	Justify   string `yaml:"justify,omitempty" json:"justify,omitempty"`
	Wrap      bool   `yaml:"wrap,omitempty" json:"wrap,omitempty"`
	Grow      bool   `yaml:"grow,omitempty" json:"grow,omitempty"`
	MinWidth  string `yaml:"minWidth,omitempty" json:"minWidth,omitempty"`
	MaxWidth  string `yaml:"maxWidth,omitempty" json:"maxWidth,omitempty"`
	MinHeight string `yaml:"minHeight,omitempty" json:"minHeight,omitempty"`
	MaxHeight string `yaml:"maxHeight,omitempty" json:"maxHeight,omitempty"`
}

type Style struct {
	Role        string `yaml:"role,omitempty" json:"role,omitempty"`
	Emphasis    string `yaml:"emphasis,omitempty" json:"emphasis,omitempty"`
	Tone        string `yaml:"tone,omitempty" json:"tone,omitempty"`
	ToneBinding string `yaml:"toneBinding,omitempty" json:"toneBinding,omitempty"`
	Truncate    bool   `yaml:"truncate,omitempty" json:"truncate,omitempty"`
}

type Repeat struct {
	Source string `yaml:"source" json:"source"`
	As     string `yaml:"as" json:"as"`
	Key    string `yaml:"key" json:"key"`
}

type Condition struct {
	Binding string `yaml:"binding" json:"binding"`
	Equals  string `yaml:"equals,omitempty" json:"equals,omitempty"`
	Empty   bool   `yaml:"empty,omitempty" json:"empty,omitempty"`
	Not     bool   `yaml:"not,omitempty" json:"not,omitempty"`
}

type Action struct {
	On        string            `yaml:"on" json:"on"`
	Command   string            `yaml:"command" json:"command"`
	Arguments map[string]string `yaml:"arguments,omitempty" json:"arguments,omitempty"`
	Confirm   *Confirmation     `yaml:"confirm,omitempty" json:"confirm,omitempty"`
}

type Confirmation struct {
	Title   string `yaml:"title" json:"title"`
	Message string `yaml:"message" json:"message"`
}

type Override struct {
	Hidden bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`
	Layout Layout `yaml:"layout,omitempty" json:"layout,omitempty"`
	Style  Style  `yaml:"style,omitempty" json:"style,omitempty"`
}

var identifierPattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9.-]*$`)
var templateBindingPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

var components = map[string]bool{
	"page": true, "column": true, "row": true, "section": true,
	"card": true, "text": true, "icon": true, "image": true,
	"disclosure": true, "graph-view": true, "tree-view": true,
	"button": true, "select": true, "input": true, "list": true, "scroller": true, "badge": true, "spacer": true,
	"divider": true,
}

var commands = map[string]bool{
	"navigate": true, "run-pipeline": true, "run-chain": true,
	"toggle": true, "refresh": true, "clear-queue": true,
	"flush-history": true, "delete-execution": true, "remove-execution": true,
	"cancel-execution": true, "rerun-execution": true,
	"change-theme":         true,
	"select-timeline-item": true, "change-output-search": true,
	"find-output": true, "copy-output": true, "toggle-output-tailing": true,
	"set-report-filter": true, "download-artifact": true, "download-job-log": true,
	"set-disclosures":              true,
	"set-run-option":               true,
	"set-agent-script-field":       true,
	"set-project-structure-filter": true,
	"agent-action":                 true,
	"run-agent-script":             true,
	"project-action":               true,
	"set-project-import-field":     true, "import-project": true,
	"set-managed-yaml-field": true, "validate-managed-yaml": true, "save-managed-yaml": true,
	"set-vault-field": true, "save-vault-connection": true, "test-vault-connection": true, "delete-vault-connection": true,
	"set-server-update-option": true, "check-server-updates": true,
	"refresh-rollback-versions": true, "server-update-action": true,
	"set-connection-field": true, "save-connection": true, "retry-connection": true,
	"generate-ssh-device-key": true, "trust-ssh-host-key": true, "reject-ssh-host-key": true, "copy-text": true,
	"open-url": true,
}

func ParseScreen(payload []byte) (*ScreenDocument, error) {
	var document ScreenDocument
	if err := decodeStrict(payload, &document); err != nil {
		return nil, err
	}
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &document, nil
}

func (d *ScreenDocument) Validate() error {
	if d == nil {
		return errors.New("screen document is nil")
	}
	if d.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", d.APIVersion)
	}
	if d.Kind != "Screen" {
		return fmt.Errorf("kind must be Screen, got %q", d.Kind)
	}
	if !identifierPattern.MatchString(d.Metadata.Name) {
		return fmt.Errorf("metadata.name %q is not a valid identifier", d.Metadata.Name)
	}
	sources := map[string]struct{}{}
	for i, source := range d.Screen.DataSources {
		if !identifierPattern.MatchString(source.Name) {
			return fmt.Errorf("dataSources[%d].name %q is not a valid identifier", i, source.Name)
		}
		if source.Query == "" {
			return fmt.Errorf("dataSources[%d].query is required", i)
		}
		if source.Query != "get-front-page-view" && source.Query != "get-project-details" && source.Query != "get-job-details" && source.Query != "get-settings-view" && source.Query != "get-managed-yaml" && source.Query != "get-run-options" && source.Query != "get-agents-view" && source.Query != "get-agent-details" && source.Query != "get-vault-connections" && source.Query != "get-native-connection" && source.Query != "get-native-client-state" {
			return fmt.Errorf("dataSources[%d].query %q is not supported", i, source.Query)
		}
		for _, topic := range source.WatchTopics {
			if !changeTopics[topic] {
				return fmt.Errorf("dataSources[%d].watchTopics contains unsupported topic %q", i, topic)
			}
		}
		if _, exists := sources[source.Name]; exists {
			return fmt.Errorf("duplicate data source %q", source.Name)
		}
		sources[source.Name] = struct{}{}
	}
	state := map[string]struct{}{}
	for i, item := range d.Screen.Persistence {
		if !identifierPattern.MatchString(item.Name) || item.StorageKey == "" {
			return fmt.Errorf("persistence[%d] requires a valid name and storageKey", i)
		}
		if item.Scope != "" && item.Scope != "client" && item.Scope != "session" {
			return fmt.Errorf("persistence[%d].scope must be client or session", i)
		}
		if _, exists := state[item.Name]; exists {
			return fmt.Errorf("duplicate persistence name %q", item.Name)
		}
		state[item.Name] = struct{}{}
		sources[item.Name] = struct{}{}
	}
	for platform := range d.Screen.Overrides {
		if platform != "web" && platform != "gio" && platform != "compact" {
			return fmt.Errorf("screen.overrides contains unsupported platform %q", platform)
		}
	}
	ids := map[string]struct{}{}
	return validateNode(d.Screen.Root, "screen.root", ids, sources)
}

var changeTopics = map[string]bool{
	"server": true, "projects": true, "agents": true, "queue": true,
	"history": true, "updates": true, "vault": true,
	"agent-eligibility": true,
}

func validateNode(node Node, path string, ids map[string]struct{}, inheritedScope map[string]struct{}) error {
	scope := cloneScope(inheritedScope)
	actionScope := scope
	if !components[node.Component] {
		return fmt.Errorf("%s.component %q is not supported", path, node.Component)
	}
	if node.Component == "scroller" && node.Repeat == nil {
		return fmt.Errorf("%s.repeat is required for the scroller component", path)
	}
	if node.ID != "" {
		if !identifierPattern.MatchString(node.ID) {
			return fmt.Errorf("%s.id %q is not a valid identifier", path, node.ID)
		}
		if _, exists := ids[node.ID]; exists {
			return fmt.Errorf("duplicate node id %q", node.ID)
		}
		ids[node.ID] = struct{}{}
	}
	if node.Repeat != nil {
		if node.Repeat.Source == "" || !identifierPattern.MatchString(node.Repeat.As) || node.Repeat.Key == "" {
			return fmt.Errorf("%s.repeat requires source, a valid as name, and key", path)
		}
		if err := validateBinding(node.Repeat.Source, inheritedScope); err != nil {
			return fmt.Errorf("%s.repeat.source: %w", path, err)
		}
		scope[node.Repeat.As] = struct{}{}
		if err := validateBinding(node.Repeat.Key, scope); err != nil {
			return fmt.Errorf("%s.repeat.key: %w", path, err)
		}
	}
	if node.Text != nil {
		choices := 0
		for _, value := range []string{node.Text.Literal, node.Text.Binding, node.Text.Template} {
			if value != "" {
				choices++
			}
		}
		if choices != 1 {
			return fmt.Errorf("%s.text must set exactly one of literal, binding, or template", path)
		}
		if node.Text.Binding != "" {
			if err := validateBinding(node.Text.Binding, scope); err != nil {
				return fmt.Errorf("%s.text.binding: %w", path, err)
			}
		}
		if node.Text.Template != "" {
			if err := validateTemplate(node.Text.Template, scope); err != nil {
				return fmt.Errorf("%s.text.template: %w", path, err)
			}
		}
	}
	if node.Image != nil {
		choices := 0
		if strings.TrimSpace(node.Image.Asset) != "" {
			choices++
		}
		if strings.TrimSpace(node.Image.Binding) != "" {
			choices++
		}
		if choices != 1 {
			return fmt.Errorf("%s.image must set exactly one of asset or binding", path)
		}
		if node.Image.Binding != "" {
			if err := validateBinding(node.Image.Binding, scope); err != nil {
				return fmt.Errorf("%s.image.binding: %w", path, err)
			}
		}
	}
	if node.Select != nil {
		if node.Component != "select" {
			return fmt.Errorf("%s.select is only valid for the select component", path)
		}
		if !identifierPattern.MatchString(node.Select.As) {
			return fmt.Errorf("%s.select.as %q is not a valid identifier", path, node.Select.As)
		}
		if err := validateBinding(node.Select.Value, scope); err != nil {
			return fmt.Errorf("%s.select.value: %w", path, err)
		}
		if err := validateBinding(node.Select.Options, scope); err != nil {
			return fmt.Errorf("%s.select.options: %w", path, err)
		}
		optionScope := cloneScope(scope)
		optionScope[node.Select.As] = struct{}{}
		if err := validateBinding(node.Select.OptionValue, optionScope); err != nil {
			return fmt.Errorf("%s.select.optionValue: %w", path, err)
		}
		if err := validateBinding(node.Select.OptionLabel, optionScope); err != nil {
			return fmt.Errorf("%s.select.optionLabel: %w", path, err)
		}
		actionScope = cloneScope(scope)
		actionScope["selection"] = struct{}{}
	} else if node.Component == "select" {
		return fmt.Errorf("%s.select is required for the select component", path)
	}
	if node.Input != nil {
		if node.Component != "input" {
			return fmt.Errorf("%s.input is only valid for the input component", path)
		}
		if err := validateBinding(node.Input.Value, scope); err != nil {
			return fmt.Errorf("%s.input.value: %w", path, err)
		}
		if node.Input.MinLines < 0 {
			return fmt.Errorf("%s.input.minLines must not be negative", path)
		}
		if node.Input.MinLines > 1 && !node.Input.Multiline {
			return fmt.Errorf("%s.input.minLines requires multiline", path)
		}
		actionScope = cloneScope(scope)
		actionScope["input"] = struct{}{}
	} else if node.Component == "input" {
		return fmt.Errorf("%s.input is required for the input component", path)
	}
	if node.Disclosure != nil {
		if node.Component != "disclosure" {
			return fmt.Errorf("%s.disclosure is only valid for the disclosure component", path)
		}
		if node.Disclosure.StateKey != "" {
			if err := validateTemplate(node.Disclosure.StateKey, scope); err != nil {
				return fmt.Errorf("%s.disclosure.stateKey: %w", path, err)
			}
		}
		if presentation := node.Disclosure.CompactPresentation; presentation != "" && presentation != "inline" && presentation != "sheet" && presentation != "navigate" {
			return fmt.Errorf("%s.disclosure.compactPresentation must be inline, sheet, or navigate", path)
		}
		for i, summaryNode := range node.Disclosure.Summary {
			if err := validateNode(summaryNode, fmt.Sprintf("%s.disclosure.summary[%d]", path, i), ids, scope); err != nil {
				return err
			}
		}
		if node.Disclosure.CompactPresentation == "navigate" {
			navigationActions := 0
			for _, summaryNode := range node.Disclosure.Summary {
				for _, action := range summaryNode.Actions {
					if action.On == "activate" && action.Command == "navigate" {
						navigationActions++
					}
				}
			}
			if navigationActions != 1 {
				return fmt.Errorf("%s.disclosure.compactPresentation navigate requires exactly one activate navigation action in disclosure.summary", path)
			}
		}
	}
	if node.GraphView != nil {
		if node.Component != "graph-view" {
			return fmt.Errorf("%s.graphView is only valid for the graph-view component", path)
		}
		graph := node.GraphView
		if graph.StateKey == "" || graph.Nodes == "" || graph.NodeKey == "" || !textDefined(graph.NodeLabel) || !identifierPattern.MatchString(graph.As) {
			return fmt.Errorf("%s.graphView requires stateKey, nodes, a valid as name, nodeKey, and nodeLabel", path)
		}
		if graph.DefaultMode != "" && graph.DefaultMode != "graph" && graph.DefaultMode != "list" {
			return fmt.Errorf("%s.graphView.defaultMode must be graph or list", path)
		}
		if err := validateTemplate(graph.StateKey, scope); err != nil {
			return fmt.Errorf("%s.graphView.stateKey: %w", path, err)
		}
		if err := validateBinding(graph.Nodes, scope); err != nil {
			return fmt.Errorf("%s.graphView.nodes: %w", path, err)
		}
		graphScope := cloneScope(scope)
		graphScope[graph.As] = struct{}{}
		if err := validateBinding(graph.NodeKey, graphScope); err != nil {
			return fmt.Errorf("%s.graphView.nodeKey: %w", path, err)
		}
		if err := validateText(graph.NodeLabel, graphScope); err != nil {
			return fmt.Errorf("%s.graphView.nodeLabel: %w", path, err)
		}
		if graph.NodeMeta != (Text{}) {
			if err := validateText(graph.NodeMeta, graphScope); err != nil {
				return fmt.Errorf("%s.graphView.nodeMeta: %w", path, err)
			}
		}
		if graph.Dependencies != "" {
			if err := validateBinding(graph.Dependencies, graphScope); err != nil {
				return fmt.Errorf("%s.graphView.dependencies: %w", path, err)
			}
		}
		if graph.Root != nil {
			root := graph.Root
			if root.Binding == "" || root.Key == "" || !textDefined(root.Label) || !identifierPattern.MatchString(root.As) {
				return fmt.Errorf("%s.graphView.root requires binding, a valid as name, key, and label", path)
			}
			if err := validateBinding(root.Binding, scope); err != nil {
				return fmt.Errorf("%s.graphView.root.binding: %w", path, err)
			}
			rootScope := cloneScope(scope)
			rootScope[root.As] = struct{}{}
			if err := validateBinding(root.Key, rootScope); err != nil {
				return fmt.Errorf("%s.graphView.root.key: %w", path, err)
			}
			if err := validateText(root.Label, rootScope); err != nil {
				return fmt.Errorf("%s.graphView.root.label: %w", path, err)
			}
			if root.Meta != (Text{}) {
				if err := validateText(root.Meta, rootScope); err != nil {
					return fmt.Errorf("%s.graphView.root.meta: %w", path, err)
				}
			}
			if root.ActionVisible != nil {
				if err := validateBinding(root.ActionVisible.Binding, rootScope); err != nil {
					return fmt.Errorf("%s.graphView.root.actionVisible.binding: %w", path, err)
				}
			}
			for i, action := range root.Actions {
				if action.On != "activate" {
					return fmt.Errorf("%s.graphView.root.actions[%d].on must be activate", path, i)
				}
				if !commands[action.Command] {
					return fmt.Errorf("%s.graphView.root.actions[%d].command %q is not supported", path, i, action.Command)
				}
				for name, value := range action.Arguments {
					if err := validateTemplate(value, rootScope); err != nil {
						return fmt.Errorf("%s.graphView.root.actions[%d].arguments[%s]: %w", path, i, name, err)
					}
				}
			}
		}
		for i, detailNode := range graph.Details {
			if err := validateNode(detailNode, fmt.Sprintf("%s.graphView.details[%d]", path, i), ids, graphScope); err != nil {
				return err
			}
		}
		actionScope = graphScope
	} else if node.Component == "graph-view" {
		return fmt.Errorf("%s.graphView is required for the graph-view component", path)
	}
	if node.TreeView != nil {
		if node.Component != "tree-view" {
			return fmt.Errorf("%s.treeView is only valid for the tree-view component", path)
		}
		tree := node.TreeView
		if tree.StateKey == "" || tree.Nodes == "" || tree.NodeKey == "" || tree.Children == "" || !textDefined(tree.NodeLabel) || !identifierPattern.MatchString(tree.As) {
			return fmt.Errorf("%s.treeView requires stateKey, nodes, a valid as name, nodeKey, nodeLabel, and children", path)
		}
		if err := validateTemplate(tree.StateKey, scope); err != nil {
			return fmt.Errorf("%s.treeView.stateKey: %w", path, err)
		}
		if err := validateBinding(tree.Nodes, scope); err != nil {
			return fmt.Errorf("%s.treeView.nodes: %w", path, err)
		}
		treeScope := cloneScope(scope)
		treeScope[tree.As] = struct{}{}
		for field, binding := range map[string]string{
			"nodeKey": tree.NodeKey, "nodeTone": tree.NodeTone, "nodeLink": tree.NodeLink,
			"children": tree.Children, "defaultExpanded": tree.DefaultExpanded,
			"filterValues": tree.FilterValues,
		} {
			if binding != "" {
				if err := validateBinding(binding, treeScope); err != nil {
					return fmt.Errorf("%s.treeView.%s: %w", path, field, err)
				}
			}
		}
		if tree.Filter != "" {
			if err := validateBinding(tree.Filter, scope); err != nil {
				return fmt.Errorf("%s.treeView.filter: %w", path, err)
			}
		}
		if err := validateText(tree.NodeLabel, treeScope); err != nil {
			return fmt.Errorf("%s.treeView.nodeLabel: %w", path, err)
		}
		if tree.NodeDetail != (Text{}) {
			if err := validateText(tree.NodeDetail, treeScope); err != nil {
				return fmt.Errorf("%s.treeView.nodeDetail: %w", path, err)
			}
		}
		if tree.ActionLabel != (Text{}) {
			if err := validateText(tree.ActionLabel, treeScope); err != nil {
				return fmt.Errorf("%s.treeView.actionLabel: %w", path, err)
			}
		}
		actionScope = treeScope
	} else if node.Component == "tree-view" {
		return fmt.Errorf("%s.treeView is required for the tree-view component", path)
	}
	if node.Visible != nil {
		if err := validateBinding(node.Visible.Binding, scope); err != nil {
			return fmt.Errorf("%s.visible.binding: %w", path, err)
		}
	}
	if node.Style.ToneBinding != "" {
		if err := validateBinding(node.Style.ToneBinding, scope); err != nil {
			return fmt.Errorf("%s.style.toneBinding: %w", path, err)
		}
	}
	if node.Progress != nil {
		if err := validateBinding(node.Progress.Binding, scope); err != nil {
			return fmt.Errorf("%s.progress.binding: %w", path, err)
		}
	}
	if node.Pulse != nil {
		if node.Component != "icon" {
			return fmt.Errorf("%s.pulse is only valid for the icon component", path)
		}
		if err := validateBinding(node.Pulse.Binding, scope); err != nil {
			return fmt.Errorf("%s.pulse.binding: %w", path, err)
		}
	}
	for i, action := range node.Actions {
		if action.On != "activate" && action.On != "change" {
			return fmt.Errorf("%s.actions[%d].on must be activate or change", path, i)
		}
		if !commands[action.Command] {
			return fmt.Errorf("%s.actions[%d].command %q is not supported", path, i, action.Command)
		}
		if node.Component == "select" && action.On != "change" {
			return fmt.Errorf("%s.actions[%d].on must be change for a select component", path, i)
		}
		if node.Component == "input" && action.On != "change" {
			return fmt.Errorf("%s.actions[%d].on must be change for an input component", path, i)
		}
		for name, value := range action.Arguments {
			if err := validateTemplate(value, actionScope); err != nil {
				return fmt.Errorf("%s.actions[%d].arguments[%s]: %w", path, i, name, err)
			}
		}
	}
	for platform := range node.Overrides {
		if platform != "web" && platform != "gio" && platform != "compact" {
			return fmt.Errorf("%s.overrides contains unsupported platform %q", path, platform)
		}
	}
	for i, child := range node.Children {
		if err := validateNode(child, fmt.Sprintf("%s.children[%d]", path, i), ids, scope); err != nil {
			return err
		}
	}
	return nil
}

func validateBinding(binding string, scope map[string]struct{}) error {
	binding = strings.TrimSpace(binding)
	if binding == "" || strings.ContainsAny(binding, " {}()[]") {
		return fmt.Errorf("binding %q is malformed", binding)
	}
	if _, exists := scope[BindingRoot(binding)]; !exists {
		return fmt.Errorf("binding %q has unknown root %q", binding, BindingRoot(binding))
	}
	return nil
}

func validateText(text Text, scope map[string]struct{}) error {
	choices := 0
	for _, value := range []string{text.Literal, text.Binding, text.Template} {
		if value != "" {
			choices++
		}
	}
	if choices != 1 {
		return fmt.Errorf("must set exactly one of literal, binding, or template")
	}
	if text.Binding != "" {
		return validateBinding(text.Binding, scope)
	}
	if text.Template != "" {
		return validateTemplate(text.Template, scope)
	}
	return nil
}

func textDefined(text Text) bool {
	return text.Literal != "" || text.Binding != "" || text.Template != ""
}

func validateTemplate(template string, scope map[string]struct{}) error {
	matches := templateBindingPattern.FindAllStringSubmatch(template, -1)
	if strings.Count(template, "{{") != len(matches) || strings.Count(template, "}}") != len(matches) {
		return fmt.Errorf("template contains malformed binding markers")
	}
	for _, match := range matches {
		if err := validateBinding(match[1], scope); err != nil {
			return err
		}
	}
	return nil
}

func cloneScope(input map[string]struct{}) map[string]struct{} {
	output := make(map[string]struct{}, len(input)+1)
	for name := range input {
		output[name] = struct{}{}
	}
	return output
}

func decodeStrict(payload []byte, output any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode UI document: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("UI document contains more than one YAML document")
		}
		return fmt.Errorf("decode trailing UI document: %w", err)
	}
	return nil
}

func BindingRoot(path string) string {
	path = strings.TrimSpace(path)
	if index := strings.IndexByte(path, '.'); index >= 0 {
		return path[:index]
	}
	return path
}
