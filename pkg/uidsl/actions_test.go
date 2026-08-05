package uidsl

import (
	"sort"
	"strings"
	"testing"
)

func TestActionCatalogRequiresEverySupportedCommand(t *testing.T) {
	payload := []byte("apiVersion: ciwi.ui/v1\nkind: ActionCatalog\nactions:\n  - command: navigate\n    class: local\n")
	if _, err := ParseActionCatalog(payload); err == nil {
		t.Fatal("expected incomplete catalog to fail")
	}
}

func TestActionSpecResolveScope(t *testing.T) {
	spec := ActionSpec{Command: "run-pipeline", Scope: "pipeline:{{pipelineDbId}}"}
	if got := spec.ResolveScope(map[string]string{"pipelineDbId": "42"}); got != "pipeline:42" {
		t.Fatalf("scope = %q", got)
	}
}

func TestActionCatalogValidationDefaultsAndLookup(t *testing.T) {
	document := completeActionCatalog()
	for index := range document.Actions {
		spec := &document.Actions[index]
		switch spec.Command {
		case "refresh":
			spec.Class = ActionClassQuery
			spec.Scope = "screen"
		case "run-pipeline":
			spec.Class = ActionClassMutation
			spec.Scope = "pipeline:{{pipelineDbId}}"
		}
	}
	if err := document.Validate(); err != nil {
		t.Fatal(err)
	}
	query, ok := document.Spec("refresh")
	if !ok || query.Navigation != ActionNavigationCancel || query.Persistence != ActionPersistenceNone {
		t.Fatalf("query defaults = %#v, %v", query, ok)
	}
	mutation, ok := document.Spec("run-pipeline")
	if !ok || mutation.Navigation != ActionNavigationContinue || mutation.Persistence != ActionPersistenceSafe {
		t.Fatalf("mutation defaults = %#v, %v", mutation, ok)
	}
	if _, ok := document.Spec("does-not-exist"); ok {
		t.Fatal("unknown command was found")
	}
	if _, ok := (*ActionCatalogDocument)(nil).Spec("refresh"); ok {
		t.Fatal("nil catalog returned a command")
	}
}

func TestActionCatalogValidationRejectsInvalidSemantics(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ActionCatalogDocument)
		want string
	}{
		{name: "api version", edit: func(d *ActionCatalogDocument) { d.APIVersion = "v0" }, want: "apiVersion"},
		{name: "kind", edit: func(d *ActionCatalogDocument) { d.Kind = "Screen" }, want: "kind"},
		{name: "unsupported command", edit: func(d *ActionCatalogDocument) { d.Actions[0].Command = "unknown" }, want: "not supported"},
		{name: "duplicate", edit: func(d *ActionCatalogDocument) { d.Actions[1].Command = d.Actions[0].Command }, want: "duplicate"},
		{name: "unsupported class", edit: func(d *ActionCatalogDocument) { d.Actions[0].Class = "background" }, want: "class"},
		{name: "mutation scope", edit: func(d *ActionCatalogDocument) { d.Actions[0].Class = ActionClassMutation }, want: "scope"},
		{name: "persistence", edit: func(d *ActionCatalogDocument) { d.Actions[0].Persistence = "forever" }, want: "persistence"},
		{name: "navigation", edit: func(d *ActionCatalogDocument) { d.Actions[0].Navigation = "detach" }, want: "navigation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := completeActionCatalog()
			test.edit(document)
			if err := document.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
	if err := (*ActionCatalogDocument)(nil).Validate(); err == nil {
		t.Fatal("nil catalog unexpectedly validated")
	}
}

func TestActionSpecResolveScopeFallsBackToCommand(t *testing.T) {
	spec := ActionSpec{Command: "run-pipeline", Scope: "{{pipelineDbId}}"}
	if got := spec.ResolveScope(nil); got != "run-pipeline" {
		t.Fatalf("scope = %q", got)
	}
}

func completeActionCatalog() *ActionCatalogDocument {
	names := make([]string, 0, len(commands))
	for command := range commands {
		names = append(names, command)
	}
	sort.Strings(names)
	actions := make([]ActionSpec, 0, len(names))
	for _, command := range names {
		actions = append(actions, ActionSpec{Command: command, Class: ActionClassLocal})
	}
	return &ActionCatalogDocument{APIVersion: APIVersion, Kind: "ActionCatalog", Actions: actions}
}
