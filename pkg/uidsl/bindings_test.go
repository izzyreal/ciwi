package uidsl

import (
	"strings"
	"testing"
)

func TestValidateBindingsChecksRepeatedItems(t *testing.T) {
	document := &ScreenDocument{Metadata: Metadata{Name: "example"}, Screen: Screen{Root: Node{
		Component: "list", Repeat: &Repeat{Source: "view.items", As: "item", Key: "item.id"},
		Children: []Node{{Component: "text", Text: &Text{Binding: "item.label"}}},
	}}}
	data := map[string]any{"view": map[string]any{"items": []any{map[string]any{"id": "1"}}}}
	err := ValidateBindings(document, data, "web")
	if err == nil || !strings.Contains(err.Error(), "item.label") {
		t.Fatalf("error = %v, want missing repeated item binding", err)
	}
	data["view"].(map[string]any)["items"] = []any{map[string]any{"id": "1", "label": "One"}}
	if err := ValidateBindings(document, data, "web"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateBindingsAllowsEventArgumentsAndHiddenOverrides(t *testing.T) {
	document := &ScreenDocument{Metadata: Metadata{Name: "example"}, Screen: Screen{Root: Node{
		Component: "column", Children: []Node{
			{Component: "input", Input: &Input{Value: "view.value"}, Actions: []Action{{Arguments: map[string]string{"value": "{{input.value}}"}}}},
			{Component: "text", Text: &Text{Binding: "view.native_only"}, Overrides: map[string]Override{"web": {Hidden: true}}},
		},
	}}}
	if err := ValidateBindings(document, map[string]any{"view": map[string]any{"value": ""}}, "web"); err != nil {
		t.Fatal(err)
	}
}
