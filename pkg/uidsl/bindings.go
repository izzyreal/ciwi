package uidsl

import (
	"fmt"
	"reflect"
	"strings"
)

// ValidateBindings verifies that a renderer's view model satisfies every
// binding reachable in a screen for the selected platform. Repeat and select
// item aliases are checked against every supplied item.
func ValidateBindings(document *ScreenDocument, data map[string]any, platform string) error {
	if document == nil {
		return fmt.Errorf("screen document is nil")
	}
	if data == nil {
		return fmt.Errorf("screen %q binding data is nil", document.Metadata.Name)
	}
	if err := validateNodeBindings(document.Screen.Root, data, map[string]any{}, platform, "screen.root"); err != nil {
		return fmt.Errorf("screen %q: %w", document.Metadata.Name, err)
	}
	return nil
}

func validateNodeBindings(node Node, data, locals map[string]any, platform, location string) error {
	if override, ok := node.Overrides[platform]; ok && override.Hidden {
		return nil
	}
	instances := []map[string]any{locals}
	if node.Repeat != nil {
		value, err := resolveBindingValue(data, locals, node.Repeat.Source)
		if err != nil {
			return fmt.Errorf("%s.repeat.source: %w", location, err)
		}
		items, err := bindingSlice(value)
		if err != nil {
			return fmt.Errorf("%s.repeat.source %q: %w", location, node.Repeat.Source, err)
		}
		instances = make([]map[string]any, 0, len(items))
		for index, item := range items {
			next := cloneBindingLocals(locals)
			next[node.Repeat.As] = item
			if _, err := resolveBindingValue(data, next, node.Repeat.Key); err != nil {
				return fmt.Errorf("%s.repeat[%d].key: %w", location, index, err)
			}
			instances = append(instances, next)
		}
	}
	for _, scope := range instances {
		if err := validateNodeInstanceBindings(node, data, scope, platform, location); err != nil {
			return err
		}
	}
	return nil
}

func validateNodeInstanceBindings(node Node, data, locals map[string]any, platform, location string) error {
	check := func(label, binding string) error {
		if strings.TrimSpace(binding) == "" {
			return nil
		}
		if _, err := resolveBindingValue(data, locals, binding); err != nil {
			return fmt.Errorf("%s.%s: %w", location, label, err)
		}
		return nil
	}
	checkTemplate := func(label, template string, eventLocals bool) error {
		for _, match := range templateBindingPattern.FindAllStringSubmatch(template, -1) {
			binding := strings.TrimSpace(match[1])
			root := strings.SplitN(binding, ".", 2)[0]
			if eventLocals && (root == "input" || root == "selection") {
				continue
			}
			if err := check(label, binding); err != nil {
				return err
			}
		}
		return nil
	}
	if node.Text != nil {
		if err := check("text.binding", node.Text.Binding); err != nil {
			return err
		}
		if err := checkTemplate("text.template", node.Text.Template, false); err != nil {
			return err
		}
	}
	if node.Image != nil {
		if err := check("image.binding", node.Image.Binding); err != nil {
			return err
		}
	}
	for label, binding := range map[string]string{
		"visible.binding": nodeBinding(node.Visible), "enabled.binding": nodeBinding(node.Enabled),
		"style.toneBinding": node.Style.ToneBinding,
	} {
		if err := check(label, binding); err != nil {
			return err
		}
	}
	if node.Input != nil {
		if err := check("input.value", node.Input.Value); err != nil {
			return err
		}
	}
	if node.Progress != nil {
		if err := check("progress.binding", node.Progress.Binding); err != nil {
			return err
		}
	}
	if node.Pulse != nil {
		if err := check("pulse.binding", node.Pulse.Binding); err != nil {
			return err
		}
	}
	if node.Disclosure != nil {
		if err := checkTemplate("disclosure.stateKey", node.Disclosure.StateKey, false); err != nil {
			return err
		}
		for index, child := range node.Disclosure.Summary {
			if err := validateNodeBindings(child, data, locals, platform, fmt.Sprintf("%s.disclosure.summary[%d]", location, index)); err != nil {
				return err
			}
		}
	}
	if node.Select != nil {
		if err := check("select.value", node.Select.Value); err != nil {
			return err
		}
		value, err := resolveBindingValue(data, locals, node.Select.Options)
		if err != nil {
			return fmt.Errorf("%s.select.options: %w", location, err)
		}
		items, err := bindingSlice(value)
		if err != nil {
			return fmt.Errorf("%s.select.options: %w", location, err)
		}
		for index, item := range items {
			next := cloneBindingLocals(locals)
			next[node.Select.As] = item
			if _, err := resolveBindingValue(data, next, node.Select.OptionValue); err != nil {
				return fmt.Errorf("%s.select.options[%d].value: %w", location, index, err)
			}
			if _, err := resolveBindingValue(data, next, node.Select.OptionLabel); err != nil {
				return fmt.Errorf("%s.select.options[%d].label: %w", location, index, err)
			}
		}
	}
	if node.GraphView != nil {
		if err := checkTemplate("graphView.stateKey", node.GraphView.StateKey, false); err != nil {
			return err
		}
		value, err := resolveBindingValue(data, locals, node.GraphView.Nodes)
		if err != nil {
			return fmt.Errorf("%s.graphView.nodes: %w", location, err)
		}
		items, err := bindingSlice(value)
		if err != nil {
			return fmt.Errorf("%s.graphView.nodes: %w", location, err)
		}
		for index, item := range items {
			next := cloneBindingLocals(locals)
			next[node.GraphView.As] = item
			for label, binding := range map[string]string{"key": node.GraphView.NodeKey, "dependencies": node.GraphView.Dependencies} {
				if _, err := resolveBindingValue(data, next, binding); err != nil {
					return fmt.Errorf("%s.graphView.nodes[%d].%s: %w", location, index, label, err)
				}
			}
			for label, text := range map[string]Text{"label": node.GraphView.NodeLabel, "meta": node.GraphView.NodeMeta} {
				if text.Binding != "" {
					if _, err := resolveBindingValue(data, next, text.Binding); err != nil {
						return fmt.Errorf("%s.graphView.nodes[%d].%s: %w", location, index, label, err)
					}
				}
				for _, match := range templateBindingPattern.FindAllStringSubmatch(text.Template, -1) {
					if _, err := resolveBindingValue(data, next, strings.TrimSpace(match[1])); err != nil {
						return fmt.Errorf("%s.graphView.nodes[%d].%s: %w", location, index, label, err)
					}
				}
			}
			for detailIndex, detail := range node.GraphView.Details {
				if err := validateNodeBindings(detail, data, next, platform, fmt.Sprintf("%s.graphView.details[%d]", location, detailIndex)); err != nil {
					return err
				}
			}
			for actionIndex, action := range node.Actions {
				for name, value := range action.Arguments {
					for _, match := range templateBindingPattern.FindAllStringSubmatch(value, -1) {
						binding := strings.TrimSpace(match[1])
						root := strings.SplitN(binding, ".", 2)[0]
						if root == "input" || root == "selection" {
							continue
						}
						if _, err := resolveBindingValue(data, next, binding); err != nil {
							return fmt.Errorf("%s.graphView.nodes[%d].actions[%d].arguments.%s: %w", location, index, actionIndex, name, err)
						}
					}
				}
			}
		}
	}
	if node.GraphView == nil {
		for actionIndex, action := range node.Actions {
			for name, value := range action.Arguments {
				if err := checkTemplate(fmt.Sprintf("actions[%d].arguments.%s", actionIndex, name), value, true); err != nil {
					return err
				}
			}
		}
	}
	for index, child := range node.Children {
		if err := validateNodeBindings(child, data, locals, platform, fmt.Sprintf("%s.children[%d]", location, index)); err != nil {
			return err
		}
	}
	return nil
}

func nodeBinding(condition *Condition) string {
	if condition == nil {
		return ""
	}
	return condition.Binding
}

func resolveBindingValue(data, locals map[string]any, path string) (any, error) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("binding is empty")
	}
	var current any
	if value, ok := locals[parts[0]]; ok {
		current = value
	} else if value, ok := data[parts[0]]; ok {
		current = value
	} else {
		return nil, fmt.Errorf("binding not found: %s", path)
	}
	for _, part := range parts[1:] {
		value := reflect.ValueOf(current)
		for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
			if value.IsNil() {
				return nil, fmt.Errorf("binding not found: %s", path)
			}
			value = value.Elem()
		}
		if !value.IsValid() || value.Kind() != reflect.Map || value.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("binding not found: %s", path)
		}
		next := value.MapIndex(reflect.ValueOf(part).Convert(value.Type().Key()))
		if !next.IsValid() {
			return nil, fmt.Errorf("binding not found: %s", path)
		}
		current = next.Interface()
	}
	return current, nil
}

func bindingSlice(value any) ([]any, error) {
	reflected := reflect.ValueOf(value)
	for reflected.IsValid() && (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface) {
		if reflected.IsNil() {
			return nil, nil
		}
		reflected = reflected.Elem()
	}
	if !reflected.IsValid() {
		return nil, nil
	}
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return nil, fmt.Errorf("must be a list")
	}
	result := make([]any, reflected.Len())
	for index := range result {
		result[index] = reflected.Index(index).Interface()
	}
	return result, nil
}

func cloneBindingLocals(input map[string]any) map[string]any {
	result := make(map[string]any, len(input)+1)
	for key, value := range input {
		result[key] = value
	}
	return result
}
