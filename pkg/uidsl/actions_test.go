package uidsl

import "testing"

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
