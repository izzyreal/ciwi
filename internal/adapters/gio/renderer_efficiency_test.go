//go:build darwin || linux || windows

package gio

import (
	"testing"

	presentationOperations "github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestButtonNodeStateMatchesActiveOperation(t *testing.T) {
	action := uidsl.Action{Command: "run-job", Arguments: map[string]string{"jobId": "{{job.id}}"}}
	data := map[string]any{"job": map[string]any{"id": "job-1"}}
	fingerprint, err := presentationOperations.Fingerprint(action.Command, map[string]string{"jobId": "job-1"})
	if err != nil {
		t.Fatal(err)
	}
	renderer := &Renderer{activeOperations: map[string]presentationOperations.Operation{
		fingerprint: {ID: "operation-1", Fingerprint: fingerprint, PendingLabel: "Starting…", State: presentationOperations.StateRunning},
	}}
	node := uidsl.Node{Text: &uidsl.Text{Literal: "Run"}, Actions: []uidsl.Action{action}}
	label, enabled := renderer.buttonNodeState(&node, data)
	if enabled || label != "Starting…" || node.Icon != "loader-2" {
		t.Fatalf("pending button = label %q, enabled %v, icon %q", label, enabled, node.Icon)
	}
}

func TestButtonNodeStateSkipsActionResolutionWithoutActiveOperations(t *testing.T) {
	node := uidsl.Node{
		Text:    &uidsl.Text{Literal: "Run"},
		Actions: []uidsl.Action{{Command: "run-job", Arguments: map[string]string{"jobId": "{{job.id}}"}}},
	}
	data := map[string]any{"job": map[string]any{"id": "job-1"}}
	inactive := &Renderer{activeOperations: map[string]presentationOperations.Operation{}}
	active := &Renderer{activeOperations: map[string]presentationOperations.Operation{"unrelated": {ID: "operation-1"}}}
	inactiveAllocs := testing.AllocsPerRun(100, func() {
		copy := node
		inactive.buttonNodeState(&copy, data)
	})
	activeAllocs := testing.AllocsPerRun(100, func() {
		copy := node
		active.buttonNodeState(&copy, data)
	})
	if inactiveAllocs >= activeAllocs {
		t.Fatalf("inactive allocations = %.1f, active allocations = %.1f; action resolution was not skipped", inactiveAllocs, activeAllocs)
	}
}
