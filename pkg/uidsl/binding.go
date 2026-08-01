package uidsl

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Resolve evaluates a dot-separated binding against renderer view data. It
// supports string-keyed maps, structs through their JSON names, and slices.
func Resolve(root any, binding string) (any, error) {
	var normalized any
	encoded, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("normalize binding data: %w", err)
	}
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, fmt.Errorf("normalize binding data: %w", err)
	}
	current := normalized
	for _, part := range strings.Split(binding, ".") {
		if part == "" {
			return nil, fmt.Errorf("invalid binding %q", binding)
		}
		switch value := current.(type) {
		case map[string]any:
			var exists bool
			current, exists = value[part]
			if !exists {
				return nil, fmt.Errorf("binding %q does not exist", binding)
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil, fmt.Errorf("binding %q has invalid list index %q", binding, part)
			}
			current = value[index]
		default:
			return nil, fmt.Errorf("binding %q cannot descend through %s", binding, reflect.TypeOf(current))
		}
	}
	return current, nil
}

// RenderText evaluates a text expression. Templates use {{binding.path}}
// placeholders and intentionally provide no executable expression language.
func RenderText(root any, expression Text) (string, error) {
	if expression.Literal != "" {
		return expression.Literal, nil
	}
	if expression.Binding != "" {
		value, err := Resolve(root, expression.Binding)
		if err != nil {
			return "", err
		}
		return fmt.Sprint(value), nil
	}
	template := expression.Template
	for {
		start := strings.Index(template, "{{")
		if start < 0 {
			return template, nil
		}
		endRelative := strings.Index(template[start+2:], "}}")
		if endRelative < 0 {
			return "", fmt.Errorf("unterminated binding in template %q", expression.Template)
		}
		end := start + 2 + endRelative
		binding := strings.TrimSpace(template[start+2 : end])
		value, err := Resolve(root, binding)
		if err != nil {
			return "", err
		}
		template = template[:start] + fmt.Sprint(value) + template[end+2:]
	}
}
