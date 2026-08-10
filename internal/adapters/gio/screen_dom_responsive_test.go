//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestExecutionDisclosureHeaderAdaptsWithoutExcessiveHeight(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	leading := []giodom.Element{
		giodom.Spacer("logo", 28, 28),
		giodom.Spacer("status", 20, 20),
	}
	copyElements := []giodom.Element{
		renderer.domText("summary", "1/15 successful, 7 in progress, 7 waiting", "control", true, "warning", false),
	}
	header := renderer.domExecutionDisclosureHeader(
		"execution", "VMPC2000XL Full cross-platform release Mon 10 Aug, 21:15:00 v0.9.17",
		leading, copyElements, nil, giodom.Spacer("toggle", 28, 28),
	)

	portrait := layoutResponsiveElement(renderer, header, 375, 667)
	landscape := layoutResponsiveElement(renderer, header, 667, 375)
	if portrait.Size.X != 375 || portrait.Size.Y <= 0 || portrait.Size.Y > 140 {
		t.Fatalf("portrait header dimensions = %v", portrait.Size)
	}
	if landscape.Size.X != 667 || landscape.Size.Y <= 0 || landscape.Size.Y > 100 {
		t.Fatalf("landscape header dimensions = %v", landscape.Size)
	}
	if landscape.Size.Y > portrait.Size.Y {
		t.Fatalf("landscape height %d exceeds portrait height %d", landscape.Size.Y, portrait.Size.Y)
	}
}

func TestOutputGroupsViewportTracksWindowHeight(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.metrics.spaceSmall = 8
	for _, test := range []struct {
		name    string
		height  unit.Dp
		minimum unit.Dp
		maximum unit.Dp
	}{
		{name: "portrait phone", height: 667, minimum: 450, maximum: 452},
		{name: "landscape phone", height: 375, minimum: 246, maximum: 248},
		{name: "desktop cap", height: 1000, minimum: 644, maximum: 644},
	} {
		t.Run(test.name, func(t *testing.T) {
			renderer.viewportHeight = test.height
			got := renderer.domOutputGroupsViewport(660)
			if got < test.minimum || got > test.maximum {
				t.Fatalf("viewport = %v, want %v..%v", got, test.minimum, test.maximum)
			}
		})
	}
}

func TestJobOutputStartsAtTailOnlyForActiveStatuses(t *testing.T) {
	for _, status := range []string{"queued", "leased", "running", "waiting", "in progress", "active"} {
		if !jobOutputStartsAtTail(status) {
			t.Errorf("%q did not start at tail", status)
		}
	}
	for _, status := range []string{"succeeded", "failed", "cancelled", ""} {
		if jobOutputStartsAtTail(status) {
			t.Errorf("%q unexpectedly started at tail", status)
		}
	}
}

func TestOutputGroupsUseSharedPinnedCollapseControl(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	screen, err := sharedui.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	scroller, ok := findResponsiveTestNode(screen.Screen.Root, "job-output-groups")
	if !ok {
		t.Fatal("job output scroller not found")
	}
	data := map[string]any{"jobDetails": map[string]any{"output_groups": []any{map[string]any{
		"id": "step-1", "title": "Long step", "state_key": "job-output:step-1", "default_expanded": true,
	}}}}
	compiled := renderer.compileDOMNode(scroller, data, "job-output")
	viewport := findResponsiveTestElement(compiled, giodom.KindVirtualList)
	if viewport == nil || viewport.List.PinnedOverlay == nil {
		t.Fatalf("compiled output viewport = %#v, want pinned overlay builder", viewport)
	}
	renderer.disclosures["job-output:step-1"] = true
	long := viewport.List.PinnedOverlay(giodom.ListViewportItem{Key: "step-1", Index: 0, Extent: 500, Viewport: 300})
	if long == nil {
		t.Fatal("long expanded group did not produce a pinned Collapse control")
	}
	if short := viewport.List.PinnedOverlay(giodom.ListViewportItem{Key: "step-1", Index: 0, Extent: 250, Viewport: 300}); short != nil {
		t.Fatal("short group unexpectedly produced a pinned Collapse control")
	}
	renderer.disclosures["job-output:step-1"] = false
	if collapsed := viewport.List.PinnedOverlay(giodom.ListViewportItem{Key: "step-1", Index: 0, Extent: 500, Viewport: 300}); collapsed != nil {
		t.Fatal("collapsed group unexpectedly produced a pinned Collapse control")
	}
}

func TestNativeExecutionJobRowActionFillsAvailableWidth(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	row := uidsl.Node{
		Component: "row", Style: uidsl.Style{Role: "queued-execution-job-row"},
		Actions:  []uidsl.Action{{On: "activate", Command: "navigate", Arguments: map[string]string{"route": "/jobs/job-1"}}},
		Children: []uidsl.Node{{Component: "text", Text: &uidsl.Text{Literal: "job-1"}}},
	}
	compiled := renderer.compileDOMNode(row, map[string]any{}, "job-row")
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1_800_000_000, 0),
		Constraints: layout.Constraints{Min: image.Point{}, Max: image.Pt(320, 200)},
	}
	dimensions := giodom.NewRuntime(renderer.theme, giodom.Options{}).Layout(gtx, *compiled)
	if dimensions.Size.X != 320 {
		t.Fatalf("actionable row width = %d, want full 320", dimensions.Size.X)
	}
}

func TestNativeExecutionJobRowNestedActionSuppressesNavigation(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	commands := []string{}
	renderer.onAction = func(action uidsl.Action, _ map[string]string) {
		commands = append(commands, action.Command)
	}
	row := uidsl.Node{
		Component: "row", Style: uidsl.Style{Role: "queued-execution-job-row"},
		Actions: []uidsl.Action{{On: "activate", Command: "navigate", Arguments: map[string]string{"route": "/jobs/job-1"}}},
		Children: []uidsl.Node{
			{Component: "spacer", Layout: uidsl.Layout{MinWidth: "100", MinHeight: "40"}},
			{Component: "button", Text: &uidsl.Text{Literal: "Cancel"}, Actions: []uidsl.Action{{On: "activate", Command: "cancel-execution"}}},
		},
	}
	compiled := renderer.compileDOMNode(row, map[string]any{}, "job-row")
	runtime := giodom.NewRuntime(renderer.theme, giodom.Options{})
	router := new(input.Router)
	layoutResponsiveInteractiveFrame(runtime, router, *compiled, nil)
	layoutResponsiveInteractiveFrame(runtime, router, *compiled, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(140, 20),
	}})
	layoutResponsiveInteractiveFrame(runtime, router, *compiled, []pointer.Event{{
		Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(140, 20),
	}})
	if len(commands) != 1 || commands[0] != "cancel-execution" {
		t.Fatalf("nested action commands = %v, want only cancel-execution", commands)
	}

	commands = nil
	layoutResponsiveInteractiveFrame(runtime, router, *compiled, []pointer.Event{{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 2, Buttons: pointer.ButtonPrimary, Position: f32.Pt(40, 20),
	}})
	layoutResponsiveInteractiveFrame(runtime, router, *compiled, []pointer.Event{{
		Kind: pointer.Release, Source: pointer.Mouse, PointerID: 2, Position: f32.Pt(40, 20),
	}})
	if len(commands) != 1 || commands[0] != "navigate" {
		t.Fatalf("passive row commands = %v, want navigate", commands)
	}
}

func findResponsiveTestNode(node uidsl.Node, id string) (uidsl.Node, bool) {
	if node.ID == id {
		return node, true
	}
	for _, child := range node.Children {
		if found, ok := findResponsiveTestNode(child, id); ok {
			return found, true
		}
	}
	return uidsl.Node{}, false
}

func findResponsiveTestElement(element *giodom.Element, kind giodom.Kind) *giodom.Element {
	if element == nil {
		return nil
	}
	if element.Kind == kind {
		return element
	}
	if element.Children != nil {
		for index := 0; index < element.Children.Len(); index++ {
			child := element.Children.At(index)
			if found := findResponsiveTestElement(&child, kind); found != nil {
				return found
			}
		}
	}
	return nil
}

func responsiveTestRenderer(t *testing.T) *Renderer {
	t.Helper()
	screen, err := sharedui.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	themes, err := sharedui.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) == 0 {
		t.Fatal("no themes")
	}
	renderer, err := NewRenderer(screen, themes[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	return renderer
}

func layoutResponsiveElement(renderer *Renderer, element giodom.Element, width, height int) layout.Dimensions {
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1_800_000_000, 0),
		Constraints: layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, height)},
	}
	return giodom.NewRuntime(renderer.theme, giodom.Options{}).Layout(gtx, element)
}

func layoutResponsiveInteractiveFrame(runtime *giodom.Runtime, router *input.Router, element giodom.Element, events []pointer.Event) {
	for _, event := range events {
		router.Queue(event)
	}
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1_800_000_000, 0),
		Constraints: layout.Constraints{Min: image.Point{}, Max: image.Pt(320, 80)},
	}
	runtime.Layout(gtx, element)
	router.Frame(operations)
}
