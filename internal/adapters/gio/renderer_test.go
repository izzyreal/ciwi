//go:build darwin

package gio

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

func TestRendererLaysOutSharedFrontPage(t *testing.T) {
	screen, err := sharedUI.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("space")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: "v0.2.0"},
		Projects: []*cnpv1.ProjectSummary{{
			Id: 1, Name: "ciwi", RepoUrl: "https://github.com/izzyreal/ciwi",
			Pipelines: []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "build"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	var operations op.Ops
	dimensions := renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if dimensions.Size != image.Pt(1100, 760) {
		t.Fatalf("dimensions = %v", dimensions.Size)
	}
}

func TestRendererHonorsActionConfirmation(t *testing.T) {
	var dispatched bool
	renderer := &Renderer{onAction: func(_ uidsl.Action, arguments map[string]string) {
		dispatched = arguments["pipelineDbId"] == "7"
	}}
	renderer.dispatch(uidsl.Action{
		Command:   "run-pipeline",
		Arguments: map[string]string{"pipelineDbId": "{{pipeline.id}}"},
		Confirm:   &uidsl.Confirmation{Title: "Run pipeline", Message: "Queue another execution."},
	}, map[string]any{"pipeline": map[string]any{"id": float64(7)}})
	if renderer.pending == nil {
		t.Fatal("confirmation was not requested")
	}
	if dispatched {
		t.Fatal("action dispatched before confirmation")
	}
	pending := renderer.pending
	renderer.pending = nil
	renderer.onAction(pending.action, pending.arguments)
	if !dispatched {
		t.Fatal("confirmed action was not dispatched")
	}
}

func TestNativeAddressNormalizesListenStyleAddress(t *testing.T) {
	address, err := nativeAddress(t.Context(), ":8113")
	if err != nil {
		t.Fatal(err)
	}
	if address != "127.0.0.1:8113" {
		t.Fatalf("address = %q", address)
	}
}
