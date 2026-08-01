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
	Component string              `yaml:"component" json:"component"`
	ID        string              `yaml:"id,omitempty" json:"id,omitempty"`
	Text      *Text               `yaml:"text,omitempty" json:"text,omitempty"`
	Icon      string              `yaml:"icon,omitempty" json:"icon,omitempty"`
	Image     *Image              `yaml:"image,omitempty" json:"image,omitempty"`
	Layout    Layout              `yaml:"layout,omitempty" json:"layout,omitempty"`
	Style     Style               `yaml:"style,omitempty" json:"style,omitempty"`
	Repeat    *Repeat             `yaml:"repeat,omitempty" json:"repeat,omitempty"`
	Visible   *Condition          `yaml:"visible,omitempty" json:"visible,omitempty"`
	Actions   []Action            `yaml:"actions,omitempty" json:"actions,omitempty"`
	Children  []Node              `yaml:"children,omitempty" json:"children,omitempty"`
	Overrides map[string]Override `yaml:"overrides,omitempty" json:"overrides,omitempty"`
}

type Text struct {
	Literal  string `yaml:"literal,omitempty" json:"literal,omitempty"`
	Binding  string `yaml:"binding,omitempty" json:"binding,omitempty"`
	Template string `yaml:"template,omitempty" json:"template,omitempty"`
}

type Image struct {
	Asset       string `yaml:"asset" json:"asset"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
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
}

type Style struct {
	Role     string `yaml:"role,omitempty" json:"role,omitempty"`
	Emphasis string `yaml:"emphasis,omitempty" json:"emphasis,omitempty"`
	Tone     string `yaml:"tone,omitempty" json:"tone,omitempty"`
	Truncate bool   `yaml:"truncate,omitempty" json:"truncate,omitempty"`
}

type Repeat struct {
	Source string `yaml:"source" json:"source"`
	As     string `yaml:"as" json:"as"`
	Key    string `yaml:"key" json:"key"`
}

type Condition struct {
	Binding string `yaml:"binding" json:"binding"`
	Equals  string `yaml:"equals,omitempty" json:"equals,omitempty"`
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
	"button": true, "list": true, "badge": true, "spacer": true,
	"divider": true,
}

var commands = map[string]bool{
	"navigate": true, "run-pipeline": true, "run-chain": true,
	"toggle": true, "refresh": true, "clear-queue": true,
	"flush-history": true, "delete-execution": true,
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
		if source.Query != "get-front-page-view" {
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
		if platform != "web" && platform != "gio" {
			return fmt.Errorf("screen.overrides contains unsupported platform %q", platform)
		}
	}
	ids := map[string]struct{}{}
	return validateNode(d.Screen.Root, "screen.root", ids, sources)
}

var changeTopics = map[string]bool{
	"server": true, "projects": true, "agents": true, "queue": true,
	"history": true, "updates": true, "vault": true,
}

func validateNode(node Node, path string, ids map[string]struct{}, inheritedScope map[string]struct{}) error {
	scope := cloneScope(inheritedScope)
	if !components[node.Component] {
		return fmt.Errorf("%s.component %q is not supported", path, node.Component)
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
	if node.Visible != nil {
		if err := validateBinding(node.Visible.Binding, scope); err != nil {
			return fmt.Errorf("%s.visible.binding: %w", path, err)
		}
	}
	for i, action := range node.Actions {
		if action.On != "activate" && action.On != "change" {
			return fmt.Errorf("%s.actions[%d].on must be activate or change", path, i)
		}
		if !commands[action.Command] {
			return fmt.Errorf("%s.actions[%d].command %q is not supported", path, i, action.Command)
		}
		for name, value := range action.Arguments {
			if err := validateTemplate(value, scope); err != nil {
				return fmt.Errorf("%s.actions[%d].arguments[%s]: %w", path, i, name, err)
			}
		}
	}
	for platform := range node.Overrides {
		if platform != "web" && platform != "gio" {
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
