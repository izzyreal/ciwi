package uidsl

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Resolve evaluates a dot-separated binding against renderer view data. It
// supports string-keyed maps, structs through their JSON names, and slices.
func Resolve(root any, binding string) (any, error) {
	original := binding
	current := root
	for {
		part, rest, more := strings.Cut(binding, ".")
		if part == "" {
			return nil, fmt.Errorf("invalid binding %q", original)
		}
		var err error
		current, err = resolveBindingPart(current, part, original)
		if err != nil {
			return nil, err
		}
		if !more {
			return current, nil
		}
		binding = rest
	}
}

func resolveBindingPart(current any, part, binding string) (any, error) {
	if current == nil {
		return nil, fmt.Errorf("binding %q cannot descend through <nil>", binding)
	}
	switch value := current.(type) {
	case map[string]any:
		item, ok := value[part]
		if !ok {
			return nil, fmt.Errorf("binding %q does not exist", binding)
		}
		return item, nil
	case []any:
		index, err := strconv.Atoi(part)
		if err != nil || index < 0 || index >= len(value) {
			return nil, fmt.Errorf("binding %q has invalid list index %q", binding, part)
		}
		return value[index], nil
	}

	value := reflect.ValueOf(current)
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("binding %q cannot descend through <nil>", binding)
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			break
		}
		key := reflect.ValueOf(part).Convert(value.Type().Key())
		item := value.MapIndex(key)
		if !item.IsValid() {
			return nil, fmt.Errorf("binding %q does not exist", binding)
		}
		return item.Interface(), nil
	case reflect.Slice, reflect.Array:
		index, err := strconv.Atoi(part)
		if err != nil || index < 0 || index >= value.Len() {
			return nil, fmt.Errorf("binding %q has invalid list index %q", binding, part)
		}
		return value.Index(index).Interface(), nil
	case reflect.Struct:
		valueType := value.Type()
		for index := 0; index < valueType.NumField(); index++ {
			field := valueType.Field(index)
			if field.PkgPath != "" { // Unexported.
				continue
			}
			name := field.Name
			if tagName := strings.Split(field.Tag.Get("json"), ",")[0]; tagName != "" {
				if tagName == "-" {
					continue
				}
				name = tagName
			}
			if name == part {
				return value.Field(index).Interface(), nil
			}
		}
		return nil, fmt.Errorf("binding %q does not exist", binding)
	}

	return nil, fmt.Errorf("binding %q cannot descend through %s", binding, reflect.TypeOf(current))
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
	var rendered strings.Builder
	rendered.Grow(len(template))
	for {
		start := strings.Index(template, "{{")
		if start < 0 {
			rendered.WriteString(template)
			return rendered.String(), nil
		}
		rendered.WriteString(template[:start])
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
		rendered.WriteString(fmt.Sprint(value))
		template = template[end+2:]
	}
}
