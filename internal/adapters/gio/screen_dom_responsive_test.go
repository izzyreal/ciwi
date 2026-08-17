//go:build darwin || ios || linux || windows

package gio

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedui "github.com/izzyreal/ciwi/ui"
	"golang.org/x/image/math/fixed"
)

func TestExecutionDisclosureHeaderAdaptsWithoutExcessiveHeight(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	leading := []giodom.Element{
		giodom.Spacer("logo", 28, 28),
		giodom.Spacer("status", 20, 20),
	}
	copyElements := []giodom.Element{
		renderer.domText("summary", "1/15 successful, 7 in progress, 7 waiting", "control", true, "warning"),
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

func TestCompactLayoutUsesSharedWidthBoundary(t *testing.T) {
	context := layout.Context{
		Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
		Constraints: layout.Constraints{Max: image.Pt(1520, 4000)},
	}
	if !compactLayoutForWidth(context, 760) {
		t.Fatal("760dp viewport was not compact")
	}
	context.Constraints.Max.X = 1522
	if compactLayoutForWidth(context, 760) {
		t.Fatal("761dp viewport was compact")
	}
}

func TestNativeProjectDisclosureUsesTrailingPassiveChevronAndInlineExpansion(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	node := uidsl.Node{
		Component: "disclosure", Text: &uidsl.Text{Literal: "Project"}, Style: uidsl.Style{Role: "project-row"},
		Disclosure: &uidsl.Disclosure{
			StateKey: "front-project:1",
			Summary: []uidsl.Node{
				{
					Component: "text", Text: &uidsl.Text{Literal: "Project"},
					Actions: []uidsl.Action{{On: "activate", Command: "navigate", Arguments: map[string]string{"route": "/projects/1"}}},
				},
				{Component: "spacer", Layout: uidsl.Layout{Grow: true}},
				{Component: "badge", Text: &uidsl.Text{Literal: "2 pipelines"}},
			},
		},
		Children: []uidsl.Node{{Component: "text", Text: &uidsl.Text{Literal: "Project details"}}},
	}
	compiled := renderer.compileDOMNode(node, map[string]any{}, "project")
	header := findResponsiveTestElementByKey(compiled, "project/header")
	if header == nil || header.Kind != giodom.KindFlex || header.Children == nil || header.Children.Len() != 2 {
		t.Fatalf("project disclosure header = %#v", header)
	}
	content, chevron := header.Children.At(0), header.Children.At(1)
	if !content.Grow {
		t.Fatal("project disclosure summary does not grow to the trailing edge")
	}
	if chevron.Kind != giodom.KindNative || chevron.Key != "project/chevron" || chevron.Native.NewState != nil {
		t.Fatalf("project disclosure chevron = %#v, want passive native leaf", chevron)
	}
	link := findResponsiveTestElementByKey(compiled, "project/summary/0")
	if link == nil || link.Kind != giodom.KindButton || link.Children == nil || link.Children.Len() != 1 {
		t.Fatalf("project name link = %#v, want retained nested navigation action", link)
	}
	linkLeaf := link.Children.At(0)
	if linkLeaf.Kind != giodom.KindNative || linkLeaf.Native.InteractionRevision == nil {
		t.Fatalf("project name link leaf = %#v, want retained native navigation action", linkLeaf)
	}
	activation := findResponsiveTestElementByKey(compiled, "project/summary-activate")
	if activation == nil || activation.Kind != giodom.KindButton || activation.Button.OnClick == nil {
		t.Fatalf("project disclosure activation = %#v", activation)
	}
	activation.Button.OnClick()
	if !renderer.disclosures["front-project:1"] {
		t.Fatal("project disclosure did not expand inline")
	}
	recompiled := renderer.compileDOMNode(node, map[string]any{}, "project")
	if body := findResponsiveTestElementByKey(recompiled, "project/body/0"); body == nil {
		t.Fatal("expanded project disclosure did not render its inline body")
	}
}

func TestNativeSharedTimelineAndCollapsedDisclosureGeometry(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	if renderer.inputPlaceholder != (color.NRGBA{R: 0x75, G: 0x75, B: 0x75, A: 0xff}) {
		t.Fatalf("native input placeholder = %#v, want #757575", renderer.inputPlaceholder)
	}
	jobScreen, err := sharedui.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	timelineCard, ok := findResponsiveTestNodeByRepeatSource(jobScreen.Screen.Root, "jobDetails.timeline")
	if !ok || len(timelineCard.Children) == 0 {
		t.Fatal("job timeline card declaration not found")
	}
	cardData := map[string]any{"item": map[string]any{
		"id": "phase-1", "title": "Ciwi phase 1/4: Prepare workspace", "status_label": "Succeeded",
		"progress": map[string]any{"state": "complete", "fraction": 1},
	}}
	card := renderer.compileDOMNode(timelineCard.Children[0], cardData, "timeline/card")
	if card == nil {
		t.Fatal("timeline card did not compile")
	}
	if dimensions := layoutResponsiveElement(renderer, *card, 1000, 500); dimensions.Size != image.Pt(235, 86) {
		t.Fatalf("timeline card dimensions = %v, want (235,86)", dimensions.Size)
	}

	chevron := renderer.domDisclosureChevron("geometry", "chevron-right")
	if dimensions := layoutResponsiveLooseElement(renderer, chevron, 100, 100); dimensions.Size != image.Pt(20, 20) {
		t.Fatalf("passive disclosure chevron dimensions = %v, want (20,20)", dimensions.Size)
	}

	output := uidsl.Node{
		Component: "disclosure", Text: &uidsl.Text{Literal: "Ciwi phase 1/4: Prepare workspace"},
		Style:      uidsl.Style{Role: "output-group"},
		Layout:     uidsl.Layout{Direction: "vertical", Gap: "0", Padding: "section-padding"},
		Disclosure: &uidsl.Disclosure{StateKey: "output:geometry"},
	}
	compiledOutput := renderer.compileDOMNode(output, map[string]any{}, "output-geometry")
	if dimensions := layoutResponsiveElement(renderer, *compiledOutput, 1000, 500); dimensions.Size.Y != 50 {
		t.Fatalf("collapsed output row height = %d, want 50", dimensions.Size.Y)
	}
}

func TestNativeSelectUsesSharedOptionHeight(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	node := uidsl.Node{
		Component: "select", ID: "geometry-select",
		Select: &uidsl.Select{
			Value: "form.selected", Options: "form.options", As: "option",
			OptionValue: "option.value", OptionLabel: "option.label",
		},
	}
	data := map[string]any{"form": map[string]any{
		"selected": "one", "options": []any{map[string]any{"value": "one", "label": "One"}},
	}}
	compiled := renderer.compileDOMNode(node, data, "geometry-select")
	if dimensions := layoutResponsiveLooseElement(renderer, *compiled, 300, 100); dimensions.Size.Y != int(renderer.controls.Select.MinimumHeight) || dimensions.Size.Y != 44 {
		t.Fatalf("native select height = %d, shared minimum = %.0f", dimensions.Size.Y, renderer.controls.Select.MinimumHeight)
	}

	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1_800_000_000, 0),
		Constraints: layout.Constraints{Max: image.Pt(300, 100)},
	}
	dimensions := renderer.layoutDOMSelectOption(gtx, new(widget.Clickable), nativeSelectOption{value: "one", label: "One"}, true)
	if dimensions.Size.Y != int(renderer.controls.Select.OptionMinimumHeight) || dimensions.Size.Y != 40 {
		t.Fatalf("native select option height = %d, shared minimum = %.0f", dimensions.Size.Y, renderer.controls.Select.OptionMinimumHeight)
	}

	shortWidth := renderer.domSelectMenuWidth(gtx, 0, []nativeSelectOption{{value: "one", label: "One"}})
	longWidth := renderer.domSelectMenuWidth(gtx, 0, []nativeSelectOption{{value: "long", label: "A considerably longer option"}})
	if shortWidth != int(renderer.controls.Select.MenuMinimumWidth) {
		t.Fatalf("short native select menu width = %d, shared minimum = %.0f", shortWidth, renderer.controls.Select.MenuMinimumWidth)
	}
	if longWidth <= shortWidth || longWidth >= gtx.Constraints.Max.X {
		t.Fatalf("long native select menu width = %d, want intrinsic width between %d and %d", longWidth, shortWidth, gtx.Constraints.Max.X)
	}
}

func TestNativeSelectMenuWidthIncludesSelectedTypography(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	options := []nativeSelectOption{
		{value: "discover", label: "Automatic discovery"},
		{value: "explicit", label: "Explicit endpoint"},
		{value: "ssh", label: "Remote server (SSH)"},
	}
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1_800_000_000, 0),
		Constraints: layout.Constraints{Max: image.Pt(300, 100)},
	}
	menuWidth := renderer.domSelectMenuWidth(gtx, 0, options)
	metrics := renderer.controls.Select
	labelWidth := menuWidth -
		2*gtx.Dp(unit.Dp(metrics.MenuPadding)) -
		2*gtx.Dp(unit.Dp(metrics.OptionPaddingX)) -
		gtx.Dp(unit.Dp(metrics.SelectionIndicatorWidth)) -
		gtx.Dp(unit.Dp(metrics.OptionGap))

	for _, option := range options {
		measure := gtx
		measure.Constraints.Min = image.Point{}
		macro := op.Record(gtx.Ops)
		selectedLabel := renderer.materialTextLabel(option.label, "control", true)
		selectedLabel.MaxLines = 1
		selectedWidth := selectedLabel.Layout(measure).Size.X
		_ = macro.Stop()
		if selectedWidth > labelWidth {
			t.Errorf("selected option %q width = %d, menu label width = %d", option.label, selectedWidth, labelWidth)
		}
	}
}

func TestNativePresentationTextIsPassiveButCodeRemainsSelectable(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	ordinary := renderer.compileDOMNode(uidsl.Node{
		Component: "text", Text: &uidsl.Text{Literal: "Queued and In Progress Job Executions"},
		Style: uidsl.Style{Role: "heading", Emphasis: "strong"},
	}, map[string]any{}, "heading")
	if ordinary == nil || ordinary.Kind != giodom.KindText || ordinary.Text.Selectable {
		t.Fatalf("ordinary native text = %#v, want passive text", ordinary)
	}
	code := renderer.compileDOMNode(uidsl.Node{
		Component: "text", Text: &uidsl.Text{Literal: "selectable output"}, Style: uidsl.Style{Role: "code"},
	}, map[string]any{}, "code")
	if code == nil || code.Kind != giodom.KindNative {
		t.Fatalf("native code text = %#v, want selectable read-only native editor", code)
	}
}

func TestNativePageScrollRegionsWrapInputsAndSystemMessages(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	input := renderer.compileDOMNode(uidsl.Node{
		Component: "input", Input: &uidsl.Input{Value: "item.query", Placeholder: "Search output"},
	}, map[string]any{"item": map[string]any{"query": ""}}, "search")
	if input == nil || input.Kind != giodom.KindPassThroughScrollRegion {
		t.Fatalf("native input = %#v, want pass-through scroll region", input)
	}

	system := renderer.compileDOMNode(uidsl.Node{
		Component: "card", Style: uidsl.Style{Role: "output-system"},
		Children: []uidsl.Node{{
			Component: "text", Text: &uidsl.Text{Literal: "System messages"}, Style: uidsl.Style{Role: "output-code"},
		}},
	}, map[string]any{}, "system")
	if system == nil || system.Kind != giodom.KindPassThroughScrollRegion {
		t.Fatalf("native system messages = %#v, want pass-through scroll region", system)
	}

	outputCode := renderer.compileDOMNode(uidsl.Node{
		Component: "text", Text: &uidsl.Text{Literal: "nested output"}, Style: uidsl.Style{Role: "output-code"},
	}, map[string]any{}, "output-code")
	if outputCode == nil || outputCode.Kind != giodom.KindNative {
		t.Fatalf("ordinary output code = %#v, want unchanged native editor", outputCode)
	}
}

func TestNativePageViewportOwnsFullWidthAroundCenteredRows(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.compact = false
	renderer.metrics.pageWidth = 200
	renderer.metrics.pageInset = 16
	renderer.metrics.spaceLarge = 24
	screen := &uidsl.ScreenDocument{
		APIVersion: uidsl.APIVersion, Kind: "Screen", Metadata: uidsl.Metadata{Name: "full-width-scroll"},
		Screen: uidsl.Screen{Root: uidsl.Node{
			Component: "page", Children: []uidsl.Node{{
				Component: "card", ID: "centered-content",
				Children: []uidsl.Node{{Component: "text", Text: &uidsl.Text{Literal: "Content"}}},
			}},
		}},
	}
	document := renderer.buildScreenDOM(screen, map[string]any{}, "")
	if document.Kind != giodom.KindVirtualList || !document.List.PassThroughScroll {
		t.Fatalf("page root = %#v, want a full-width pass-through viewport", document)
	}
	if document.Children == nil || document.Children.Len() != 1 {
		t.Fatalf("page children = %#v, want one centered row", document.Children)
	}
	row := document.Children.At(0)
	if row.Kind != giodom.KindAlign || row.Align.Direction != layout.N || row.Key != "centered-content" {
		t.Fatalf("page row = %#v, want keyed centered content inside the viewport", row)
	}
	constraint := findResponsiveTestElement(&row, giodom.KindConstrain)
	if constraint == nil || constraint.Constraint.MaxWidth != 200 {
		t.Fatalf("page row width constraint = %#v, want existing 200dp maximum", constraint)
	}
	inset := findResponsiveTestElement(&row, giodom.KindInset)
	wantInset := renderer.pageInset()
	if inset == nil || inset.Inset.Left != wantInset || inset.Inset.Right != wantInset {
		t.Fatalf("page row inset = %#v, want preserved horizontal inset %v", inset, wantInset)
	}
}

func TestNativeOverlaySpineKeepsPageIdentityStable(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	body := giodom.Column("page-list:settings", 0, giodom.Text("content", "Content", 14, color.NRGBA{}))
	tests := []struct {
		name                  string
		notice                *nativeNotice
		alert                 *nativeAlert
		pending               *pendingConfirmation
		wantNoticeModal       bool
		wantAlertModal        bool
		wantConfirmationModal bool
	}{
		{name: "none"},
		{name: "notice", notice: &nativeNotice{message: "Saved"}, wantNoticeModal: true},
		{name: "alert", alert: &nativeAlert{title: "Failed", message: "Try again"}, wantAlertModal: true},
		{name: "confirmation", pending: &pendingConfirmation{title: "Confirm", message: "Continue?"}, wantConfirmationModal: true},
		{
			name: "all", notice: &nativeNotice{message: "Saved"}, alert: &nativeAlert{title: "Failed", message: "Try again"},
			pending:         &pendingConfirmation{title: "Confirm", message: "Continue?"},
			wantNoticeModal: true, wantAlertModal: true, wantConfirmationModal: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			renderer.pending = test.pending
			document := renderer.decorateDOMOverlays(body, test.notice, test.alert)
			confirmationBody := assertOverlayShell(t, document, "confirmation-overlay", test.wantConfirmationModal)
			alertBody := assertOverlayShell(t, confirmationBody, "alert-overlay", test.wantAlertModal)
			noticeBody := assertOverlayShell(t, alertBody, "notice-overlay", test.wantNoticeModal)
			if noticeBody.Key != body.Key {
				t.Fatalf("overlay body key = %q, want stable page key %q", noticeBody.Key, body.Key)
			}
		})
	}
}

func TestNativeDownloadPanelSurvivesRoutesAndCarriesProgressActions(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.SetDownloads([]nativeDownloadSnapshot{
		{ID: "active", Label: "Artifact", FileName: "large.zip", State: string(downloadDownloading), Downloaded: 40, Total: 80},
		{ID: "staged", Label: "Clean log", FileName: "ciwi-job-clean.log", State: string(downloadReadyToSave), Downloaded: 12, Total: 12},
	})

	for _, route := range []giodom.Key{"page-list:front-page", "page-list:settings"} {
		body := giodom.Column(route, 0, giodom.Text("content", "Content", 14, color.NRGBA{}))
		document := renderer.decorateDOMOverlays(body, nil, nil)
		confirmationBody := assertOverlayShell(t, document, "confirmation-overlay", false)
		alertBody := assertOverlayShell(t, confirmationBody, "alert-overlay", false)
		noticeBody := assertOverlayShell(t, alertBody, "notice-overlay", false)
		page := assertOverlayShell(t, noticeBody, "download-overlay", true)
		if page.Key != route {
			t.Fatalf("download overlay page key = %q, want %q", page.Key, route)
		}
		if findResponsiveTestElementByKey(&document, "download-progress-active") == nil ||
			findResponsiveTestElementByKey(&document, "download-cancel-active") == nil ||
			findResponsiveTestElementByKey(&document, "download-save-staged") == nil {
			t.Fatalf("download panel omitted progress or state actions: %#v", document)
		}
	}
}

func assertOverlayShell(t *testing.T, element giodom.Element, key giodom.Key, wantModal bool) giodom.Element {
	t.Helper()
	if element.Kind != giodom.KindOverlay || element.Key != key {
		t.Fatalf("overlay = kind %v key %q, want key %q", element.Kind, element.Key, key)
	}
	wantChildren := 1
	if wantModal {
		wantChildren = 2
	}
	if element.Children == nil || element.Children.Len() != wantChildren {
		t.Fatalf("overlay %q children = %v, want %d", key, element.Children, wantChildren)
	}
	return element.Children.At(0)
}

func TestNativeSettingsCardGapAndPendingButtonWidthContracts(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	catalog, err := sharedui.LoadActionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetActionCatalog(catalog)

	card := uidsl.Node{
		Component: "card", Layout: uidsl.Layout{Direction: "vertical", Gap: "small"},
		Children: []uidsl.Node{
			{Component: "spacer", Layout: uidsl.Layout{MinHeight: "20"}},
			{Component: "spacer", Layout: uidsl.Layout{MinHeight: "20"}},
		},
	}
	withoutGap := card
	withoutGap.Layout.Gap = "0"
	withGapDimensions := layoutResponsiveLooseElement(renderer, *renderer.compileDOMNode(card, map[string]any{}, "card-gap"), 400, 200)
	withoutGapDimensions := layoutResponsiveLooseElement(renderer, *renderer.compileDOMNode(withoutGap, map[string]any{}, "card-no-gap"), 400, 200)
	if got, want := withGapDimensions.Size.Y-withoutGapDimensions.Size.Y, int(renderer.metrics.spaceSmall); got != want {
		t.Fatalf("native card gap contribution = %d, want %d", got, want)
	}

	plain := uidsl.Node{Component: "button", Text: &uidsl.Text{Literal: "Update now"}}
	updating := plain
	updating.Actions = []uidsl.Action{{On: "activate", Command: "server-update-action"}}
	plainDimensions := layoutResponsiveLooseElement(renderer, *renderer.compileDOMNode(plain, map[string]any{}, "plain-update"), 400, 100)
	updatingDimensions := layoutResponsiveLooseElement(renderer, *renderer.compileDOMNode(updating, map[string]any{}, "reserved-update"), 400, 100)
	if updatingDimensions.Size.X <= plainDimensions.Size.X {
		t.Fatalf("pending-label button width = %d, want wider than ordinary width %d", updatingDimensions.Size.X, plainDimensions.Size.X)
	}
}

func TestNativeSelectOwnershipAndOutsideDismissal(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	options := []any{
		map[string]any{"value": "one", "label": "One"},
		map[string]any{"value": "two", "label": "Two"},
	}
	selectNode := func(id, valueBinding string) uidsl.Node {
		return uidsl.Node{
			Component: "select", ID: id,
			Select: &uidsl.Select{
				Value: valueBinding, Options: "form.options", As: "option",
				OptionValue: "option.value", OptionLabel: "option.label",
			},
			Actions: []uidsl.Action{{
				On: "change", Command: "test-select",
				Arguments: map[string]string{"value": "{{selection.value}}"},
			}},
		}
	}
	screen := &uidsl.ScreenDocument{
		APIVersion: uidsl.APIVersion, Kind: "Screen", Metadata: uidsl.Metadata{Name: "select-dismissal"},
		Screen: uidsl.Screen{Root: uidsl.Node{
			Component: "page", Layout: uidsl.Layout{Direction: "vertical", Gap: "small"},
			Children: []uidsl.Node{
				{Component: "button", ID: "screen-header", Text: &uidsl.Text{Literal: "Screen header"}},
				{
					Component: "row", Layout: uidsl.Layout{Direction: "horizontal", Gap: "small"},
					Children: []uidsl.Node{selectNode("first-select", "form.first"), selectNode("second-select", "form.second")},
				},
			},
		}},
	}
	data := map[string]any{"form": map[string]any{"first": "one", "second": "two", "options": options}}
	renderer.metrics.pageInset = 0
	renderer.metrics.spaceLarge = 0
	renderer.metrics.pageWidth = 0
	selectedValue := ""
	renderer.onAction = func(_ uidsl.Action, arguments map[string]string) {
		selectedValue = arguments["value"]
	}
	router := new(input.Router)
	frame := func(events ...pointer.Event) {
		for _, event := range events {
			router.Queue(event)
		}
		operations := new(op.Ops)
		gtx := layout.Context{
			Ops: operations, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now: time.Unix(1_800_000_000, 0), Constraints: layout.Constraints{Max: image.Pt(320, 240)},
		}
		renderer.layoutScreenDOMFrame(gtx, screen, data, "", nil, nil)
		router.Frame(operations)
	}
	tap := func(pointerID pointer.ID, x, y float32) {
		frame(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: pointerID, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)})
		frame(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: pointerID, Position: f32.Pt(x, y)})
	}
	frame()
	tap(1, 20, 74)
	if renderer.openSelectKey != "first-select" {
		t.Fatalf("open select = %q, want first-select", renderer.openSelectKey)
	}
	tap(2, 120, 74)
	if renderer.openSelectKey != "second-select" {
		t.Fatalf("open select after second tap = %q, want second-select", renderer.openSelectKey)
	}
	// If the first state remained independently open, tapping it would close
	// instead of reopening it and claiming global ownership.
	tap(3, 20, 74)
	if renderer.openSelectKey != "first-select" {
		t.Fatalf("reopened select = %q, want first-select", renderer.openSelectKey)
	}
	tap(4, 20, 22)
	if renderer.openSelectKey != "" {
		t.Fatalf("open select after tapping screen header = %q, want closed", renderer.openSelectKey)
	}
	tap(5, 20, 74)
	if renderer.openSelectKey != "first-select" {
		t.Fatalf("reopened select before outside touch = %q, want first-select", renderer.openSelectKey)
	}
	frame(pointer.Event{Kind: pointer.Press, Source: pointer.Touch, PointerID: 6, Position: f32.Pt(300, 220)})
	frame(pointer.Event{Kind: pointer.Release, Source: pointer.Touch, PointerID: 6, Position: f32.Pt(300, 220)})
	if renderer.openSelectKey != "" {
		t.Fatalf("open select after touching blank content = %q, want closed", renderer.openSelectKey)
	}
	tap(7, 20, 74)
	tap(8, 20, 170)
	if selectedValue != "two" {
		t.Fatalf("selected value after tapping popup option = %q, want two", selectedValue)
	}
	if renderer.openSelectKey != "" {
		t.Fatalf("open select after choosing popup option = %q, want closed", renderer.openSelectKey)
	}
}

func TestNativeOutputDisclosureHeaderTogglesAcrossNestedViewport(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.outputTailing = false
	screen, err := sharedui.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	scroller, ok := findResponsiveTestNode(screen.Screen.Root, "job-output-groups")
	if !ok {
		t.Fatal("job output scroller not found")
	}
	data := map[string]any{"jobDetails": map[string]any{"output_groups": []any{map[string]any{
		"id": "step-1", "title": "Ciwi phase 1/4: Prepare workspace", "state_key": "job-output:step-1",
		"default_expanded": false, "progress": map[string]any{"state": "complete", "fraction": 1},
		"reached": true, "started": "now", "status_label": "Succeeded", "duration": "1s",
		"command_label": "", "output": "ok", "error": "", "exit_code": "0", "details": "",
		"yaml_literal": "", "expanded_command": "",
	}}}}
	runtime := giodom.NewRuntime(renderer.theme, giodom.Options{})
	router := new(input.Router)
	frame := func(events ...pointer.Event) {
		for _, event := range events {
			router.Queue(event)
		}
		compiled := renderer.compileDOMNode(scroller, data, "job-output")
		operations := new(op.Ops)
		gtx := layout.Context{
			Ops: operations, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now: time.Unix(1_800_000_000, 0), Constraints: layout.Constraints{Min: image.Pt(320, 0), Max: image.Pt(320, 200)},
		}
		runtime.Layout(gtx, *compiled)
		router.Frame(operations)
	}
	frame()
	frame(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(40, 30),
	})
	frame(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(40, 30),
	})
	if !renderer.disclosures["job-output:step-1"] {
		t.Fatal("mouse tap on output label did not expand the disclosure")
	}

	frame()
	frame(pointer.Event{
		Kind: pointer.Press, Source: pointer.Touch, PointerID: 2, Position: f32.Pt(280, 30),
	})
	frame(pointer.Event{
		Kind: pointer.Release, Source: pointer.Touch, PointerID: 2, Position: f32.Pt(280, 30),
	})
	if renderer.disclosures["job-output:step-1"] {
		t.Fatal("touch tap on the trailing output row area did not collapse the disclosure")
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
		{name: "portrait phone", height: 667, minimum: 448, maximum: 450},
		{name: "landscape phone", height: 375, minimum: 244, maximum: 246},
		{name: "desktop cap", height: 1000, minimum: 642, maximum: 642},
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

func TestNativeOutputGroupsMaxHeightRemainsACap(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	screen, err := sharedui.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	scroller, ok := findResponsiveTestNode(screen.Screen.Root, "job-output-groups")
	if !ok {
		t.Fatal("job output scroller not found")
	}
	groups := func(count int) []any {
		result := make([]any, 0, count)
		for index := 0; index < count; index++ {
			result = append(result, map[string]any{
				"id": fmt.Sprintf("phase-%d", index), "title": fmt.Sprintf("Phase %d", index+1),
				"state_key": fmt.Sprintf("job-output:phase-%d", index), "status": "succeeded",
				"progress": map[string]any{"state": "complete", "fraction": 1},
			})
		}
		return result
	}
	compile := func(count int) giodom.Element {
		compiled := renderer.compileDOMNode(scroller, map[string]any{
			"jobDetails": map[string]any{"output_groups": groups(count)},
		}, fmt.Sprintf("output-groups-%d", count))
		return *compiled
	}
	if dimensions := layoutResponsiveLooseElement(renderer, compile(1), 800, 1000); dimensions.Size.Y >= 200 {
		t.Fatalf("one collapsed output group height = %d, want intrinsic content height", dimensions.Size.Y)
	}
	if dimensions := layoutResponsiveLooseElement(renderer, compile(20), 800, 1000); dimensions.Size.Y != 660 {
		t.Fatalf("many output groups height = %d, want declared cap 660", dimensions.Size.Y)
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

func TestNativeJobLogDescriptorStillTriggersInitialPageLoad(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.outputTailing = false
	requests := make([]map[string]string, 0, 1)
	renderer.onAction = func(action uidsl.Action, arguments map[string]string) {
		if action.Command == "load-job-log-page" {
			requests = append(requests, cloneStringMap(arguments))
		}
	}
	renderer.ApplyJobLogDescriptor(jobLogDescriptorSnapshot{
		JobID: "job-1", Streams: map[string]int64{"step:1": 7},
	})

	node := uidsl.Node{Component: "log-view", LogView: &uidsl.LogView{
		JobExecutionID: "jobDetails.id", ItemID: "outputGroup.id",
	}}
	data := map[string]any{
		"jobDetails":  map[string]any{"id": "job-1"},
		"outputGroup": map[string]any{"id": "step:1"},
	}
	renderer.compileDOMLogView(node, data, "log")
	renderer.compileDOMLogView(node, data, "log")

	if len(requests) != 1 {
		t.Fatalf("page requests = %d, want one deduplicated request", len(requests))
	}
	if requests[0]["mode"] != "head" || requests[0]["cursor"] != "0" {
		t.Fatalf("page request = %+v, want initial head page", requests[0])
	}
	stream := renderer.jobLogStreams[nativeJobLogKey("job-1", "step:1")]
	if stream.PageLoaded {
		t.Fatal("descriptor incorrectly marked the log page loaded")
	}
}

func TestNativeEmptyJobLogPageReloadsWhenChunksArrive(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.outputTailing = true
	requests := make([]map[string]string, 0, 1)
	renderer.onAction = func(action uidsl.Action, arguments map[string]string) {
		if action.Command == "load-job-log-page" {
			requests = append(requests, cloneStringMap(arguments))
		}
	}
	key := nativeJobLogKey("job-2", "step:1")
	renderer.ApplyJobLogPage(jobLogStreamSnapshot{JobID: "job-2", ItemID: "step:1"})
	renderer.ApplyJobLogDescriptor(jobLogDescriptorSnapshot{
		JobID: "job-2", Streams: map[string]int64{"step:1": 11},
	})

	node := uidsl.Node{Component: "log-view", LogView: &uidsl.LogView{
		JobExecutionID: "jobDetails.id", ItemID: "outputGroup.id",
	}}
	data := map[string]any{
		"jobDetails":  map[string]any{"id": "job-2"},
		"outputGroup": map[string]any{"id": "step:1"},
	}
	renderer.compileDOMLogView(node, data, "log")

	if len(requests) != 1 || requests[0]["mode"] != "tail" {
		t.Fatalf("page requests = %+v, want one tail reload", requests)
	}
	if stream := renderer.jobLogStreams[key]; !stream.PageLoaded || !stream.HasAfter || stream.LatestChunkID != 11 {
		t.Fatalf("empty stream after descriptor = %+v", stream)
	}
}

func TestNativeShortJobLogUsesIntrinsicHeightInsteadOfViewportCap(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.ApplyJobLogPage(jobLogStreamSnapshot{
		JobID: "job-short", ItemID: "", Terminal: true,
		Chunks: []jobLogChunkSnapshot{{ID: 1, Text: "one short line\n"}},
	})
	node := uidsl.Node{Component: "log-view", LogView: &uidsl.LogView{JobExecutionID: "jobDetails.id"}}
	compiled := renderer.compileDOMNode(node, map[string]any{
		"jobDetails": map[string]any{"id": "job-short"},
	}, "short-log")
	dimensions := layoutResponsiveLooseElement(renderer, *compiled, 800, 800)
	minimum, maximum := int(renderer.controls.LogView.MinimumHeight), int(renderer.controls.LogView.MaximumHeight)
	if dimensions.Size.Y < minimum || dimensions.Size.Y >= maximum {
		t.Fatalf("short native log height = %d, want intrinsic height in [%d,%d)", dimensions.Size.Y, minimum, maximum)
	}
}

func TestNativeLongJobLogStopsAtSharedViewportCap(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.ApplyJobLogPage(jobLogStreamSnapshot{
		JobID: "job-long", Terminal: true,
		Chunks: []jobLogChunkSnapshot{{ID: 1, Text: strings.Repeat("a long output line\n", 80)}},
	})
	node := uidsl.Node{Component: "log-view", LogView: &uidsl.LogView{JobExecutionID: "jobDetails.id"}}
	compiled := renderer.compileDOMNode(node, map[string]any{
		"jobDetails": map[string]any{"id": "job-long"},
	}, "long-log")
	dimensions := layoutResponsiveLooseElement(renderer, *compiled, 800, 800)
	if want := int(renderer.controls.LogView.MaximumHeight); dimensions.Size.Y != want {
		t.Fatalf("long native log height = %d, want shared cap %d", dimensions.Size.Y, want)
	}
}

func TestNativeJobLogPagePreservesNewerDescriptorState(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.ApplyJobLogDescriptor(jobLogDescriptorSnapshot{
		JobID: "job-3", Terminal: true, Streams: map[string]int64{"step:1": 9},
	})
	renderer.ApplyJobLogPage(jobLogStreamSnapshot{
		JobID: "job-3", ItemID: "step:1", Chunks: []jobLogChunkSnapshot{{ID: 5, Text: "older"}},
	})

	stream := renderer.jobLogStreams[nativeJobLogKey("job-3", "step:1")]
	if !stream.PageLoaded || !stream.Terminal || !stream.HasAfter || stream.LatestChunkID != 9 {
		t.Fatalf("merged stream = %+v, want terminal page with newer data pending", stream)
	}
	renderer.ApplyJobLogPage(jobLogStreamSnapshot{
		JobID: "job-3", ItemID: "step:1", LoadedMode: "after",
		Chunks: []jobLogChunkSnapshot{{ID: 9, Text: "newest"}},
	})
	if stream = renderer.jobLogStreams[nativeJobLogKey("job-3", "step:1")]; stream.HasAfter {
		t.Fatalf("caught-up stream still reports newer data: %+v", stream)
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

func TestInteractiveLogOwnsTailingInsteadOfOutputGroupScroller(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.outputTailing = true
	screen, err := sharedui.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	scroller, ok := findResponsiveTestNode(screen.Screen.Root, "job-output-groups")
	if !ok {
		t.Fatal("job output scroller not found")
	}
	data := func(interactive bool) map[string]any {
		return map[string]any{"jobDetails": map[string]any{
			"interactive_log_available": interactive,
			"output_groups": []any{map[string]any{
				"id": "step:1", "title": "Step", "state_key": "job-output:step:1",
			}},
		}}
	}
	interactive := findResponsiveTestElement(renderer.compileDOMNode(scroller, data(true), "interactive-output"), giodom.KindVirtualList)
	if interactive == nil || interactive.List.ScrollToEnd || interactive.List.OnLeaveEnd != nil {
		t.Fatalf("interactive output list tail props = %#v, want no outer follow ownership", interactive)
	}
	legacy := findResponsiveTestElement(renderer.compileDOMNode(scroller, data(false), "legacy-output"), giodom.KindVirtualList)
	if legacy == nil || !legacy.List.ScrollToEnd || legacy.List.OnLeaveEnd == nil {
		t.Fatalf("legacy output list tail props = %#v, want retained outer follow behavior", legacy)
	}

	renderer.ApplyJobLogPage(jobLogStreamSnapshot{
		JobID: "job-1", ItemID: "step:1", Chunks: []jobLogChunkSnapshot{{ID: 1, Text: "output\n"}},
	})
	log := renderer.compileDOMLogView(uidsl.Node{Component: "log-view", LogView: &uidsl.LogView{
		JobExecutionID: "jobDetails.id", ItemID: "outputGroup.id",
	}}, map[string]any{
		"jobDetails":  map[string]any{"id": "job-1"},
		"outputGroup": map[string]any{"id": "step:1"},
	}, "running-log")
	if !log.List.ScrollToEnd || log.List.OnLeaveEnd == nil {
		t.Fatalf("interactive log tail props = %#v, want nested follow ownership", log.List)
	}
}

func TestInteractiveOutputGroupScrollsMetadataAndLogUnderFixedHeader(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.outputTailing = true
	renderer.viewportHeight = 400
	renderer.ApplyJobLogPage(jobLogStreamSnapshot{
		JobID: "job-1", ItemID: "step:1", PageLoaded: true,
		Chunks: []jobLogChunkSnapshot{{ID: 7, Text: strings.Repeat("output line\n", 80)}},
	})
	screen, err := sharedui.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	scroller, ok := findResponsiveTestNode(screen.Screen.Root, "job-output-groups")
	if !ok {
		t.Fatal("job output scroller not found")
	}
	compiled := renderer.compileDOMNode(scroller, map[string]any{"jobDetails": map[string]any{
		"id": "job-1", "interactive_log_available": true,
		"output_groups": []any{map[string]any{
			"id": "step:1", "title": "Job step 1/1: Build", "state_key": "job-output:step:1",
			"default_expanded": true, "reached": true, "started": "now", "duration": "1s",
			"kind": "step", "yaml_literal": "run: build", "expanded_command": "build",
		}},
	}}, "interactive-output")
	body := findResponsiveTestListByLabel(compiled, "Execution output")
	if body == nil || !body.List.ScrollToEnd || body.List.Viewport != renderer.domJobLogViewport(true) || body.List.Viewport >= unit.Dp(renderer.controls.LogView.MaximumHeight) {
		t.Fatalf("interactive step body = %#v, want bounded tail-owning viewport", body)
	}
	if body.Children == nil || body.Children.Len() < 2 {
		t.Fatalf("interactive step body children = %#v, want metadata preamble plus log blocks", body.Children)
	}
	preamble := body.Children.At(0)
	if !responsiveTestContainsText(&preamble, "Started: now") || responsiveTestContainsText(&preamble, "Job step 1/1: Build") {
		t.Fatalf("interactive preamble = %#v, want metadata below a fixed disclosure header", preamble)
	}
}

func TestNativeLogSearchHighlightsReturnedRuneSpan(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	renderer.outputTailing = false
	renderer.jobLogStreams[nativeJobLogKey("job-1", "step:1")] = jobLogStreamSnapshot{
		JobID: "job-1", ItemID: "step:1", PageLoaded: true,
		Chunks:            []jobLogChunkSnapshot{{ID: 7, Text: "prefix needle suffix"}},
		SelectedChunkID:   7,
		SelectedStartRune: 7,
		SelectedEndRune:   13,
	}
	log := renderer.compileDOMLogView(uidsl.Node{Component: "log-view", LogView: &uidsl.LogView{
		JobExecutionID: "jobDetails.id", ItemID: "outputGroup.id",
	}}, map[string]any{
		"jobDetails":  map[string]any{"id": "job-1"},
		"outputGroup": map[string]any{"id": "step:1"},
	}, "searched-log")
	if log.Children == nil || log.Children.Len() != 1 {
		t.Fatalf("compiled searched log children = %#v", log.Children)
	}
	wantTarget := giodom.Key("searched-log/job-log-chunk:7:block:0")
	if log.List.ScrollTo != wantTarget {
		t.Fatalf("searched log target = %q, want bounded match block %q", log.List.ScrollTo, wantTarget)
	}
	chunk := log.Children.At(0)
	if chunk.Kind != giodom.KindNative || chunk.Native.NewState == nil || chunk.Native.Layout == nil {
		t.Fatalf("compiled searched chunk = %#v", chunk)
	}
	state := chunk.Native.NewState()
	chunk.Native.Layout(layout.Context{
		Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(800, 2000)},
	}, state)
	editor := &state.(*domEditorState).editor
	if editor.Text() != "prefix needle suffix" {
		t.Fatalf("target display block text = %q", editor.Text())
	}
	if start, end := editor.Selection(); start != end {
		t.Fatalf("search highlight stole editor selection focus state: %d:%d", start, end)
	}
}

func TestDOMTextHighlightRegionsDoNotDependOnEditorFocus(t *testing.T) {
	glyphs := make([]text.Glyph, 4)
	for index := range glyphs {
		glyphs[index] = text.Glyph{
			X: fixed.I(index * 10), Y: 12, Advance: fixed.I(10),
			Ascent: fixed.I(10), Descent: fixed.I(3), Runes: 1, Flags: text.FlagClusterBreak,
		}
	}
	regions := domTextHighlightRegions(glyphs, 1, 3)
	if len(regions) != 1 || regions[0] != image.Rect(10, 2, 30, 15) {
		t.Fatalf("highlight regions = %#v, want one focus-independent rectangle", regions)
	}
	clustered := []text.Glyph{
		{X: 0, Y: 12, Advance: fixed.I(12), Ascent: fixed.I(10), Descent: fixed.I(3), Runes: 2, Flags: text.FlagClusterBreak | text.FlagLineBreak},
		{X: 0, Y: 32, Advance: fixed.I(10), Ascent: fixed.I(10), Descent: fixed.I(3), Runes: 1, Flags: text.FlagClusterBreak},
	}
	regions = domTextHighlightRegions(clustered, 1, 3)
	if len(regions) != 2 || regions[0] != image.Rect(0, 2, 12, 15) || regions[1] != image.Rect(0, 22, 10, 35) {
		t.Fatalf("clustered wrapped highlight regions = %#v", regions)
	}
}

func TestNativeLogSearchHighlightsCrossChunkRuneSpan(t *testing.T) {
	stream := jobLogStreamSnapshot{
		Chunks: []jobLogChunkSnapshot{
			{ID: 4, Text: "abc"},
			{ID: 5, Text: "def"},
		},
		SelectedChunkID: 5, SelectedStartRune: -2, SelectedEndRune: 2,
	}
	ranges := jobLogSelectionRanges(stream)
	if ranges[4] != [2]int{1, 3} || ranges[5] != [2]int{0, 2} {
		t.Fatalf("cross-chunk highlight ranges = %#v", ranges)
	}
	blocks, target := nativeJobLogDisplayBlocks(stream, "cross-chunk")
	if target != "cross-chunk/job-log-chunk:4:block:0" || len(blocks) != 2 {
		t.Fatalf("cross-chunk display blocks = %#v target %q", blocks, target)
	}
	if blocks[0].Text != "abc" || blocks[0].HighlightStart != 1 || blocks[0].HighlightEnd != 3 ||
		blocks[1].Text != "def" || blocks[1].HighlightStart != 0 || blocks[1].HighlightEnd != 2 {
		t.Fatalf("cross-chunk highlighted blocks = %#v", blocks)
	}
}

func TestNativeLogSearchTargetsExactBlockInsideMaximumStorageChunk(t *testing.T) {
	prefix := strings.Repeat("x", 1200)
	stream := jobLogStreamSnapshot{
		Chunks:          []jobLogChunkSnapshot{{ID: 9, Text: prefix + "needle" + strings.Repeat("y", 600)}},
		SelectedChunkID: 9, SelectedStartRune: len(prefix), SelectedEndRune: len(prefix) + len("needle"),
	}
	blocks, target := nativeJobLogDisplayBlocks(stream, "deep-search")
	if target != "deep-search/job-log-chunk:9:block:1024" {
		t.Fatalf("deep search target = %q, want bounded block containing the exact match", target)
	}
	for _, block := range blocks {
		if utf8.RuneCountInString(block.Text) > nativeJobLogDisplayRunesMax {
			t.Fatalf("display block at %d contains %d runes", block.StartRune, utf8.RuneCountInString(block.Text))
		}
		if block.Key == target && (block.HighlightStart != 176 || block.HighlightEnd != 176+len("needle")) {
			t.Fatalf("deep search target block = %#v", block)
		}
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

func findResponsiveTestNodeByRepeatSource(node uidsl.Node, source string) (uidsl.Node, bool) {
	if node.Repeat != nil && node.Repeat.Source == source {
		return node, true
	}
	for _, child := range node.Children {
		if found, ok := findResponsiveTestNodeByRepeatSource(child, source); ok {
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

func findResponsiveTestListByLabel(element *giodom.Element, label string) *giodom.Element {
	if element == nil {
		return nil
	}
	if element.Kind == giodom.KindVirtualList && element.List.SemanticLabel == label {
		return element
	}
	if element.Children != nil {
		for index := 0; index < element.Children.Len(); index++ {
			child := element.Children.At(index)
			if found := findResponsiveTestListByLabel(&child, label); found != nil {
				return found
			}
		}
	}
	return nil
}

func responsiveTestContainsText(element *giodom.Element, value string) bool {
	if element == nil {
		return false
	}
	if element.Kind == giodom.KindText && element.Text.Value == value {
		return true
	}
	if element.Children != nil {
		for index := 0; index < element.Children.Len(); index++ {
			child := element.Children.At(index)
			if responsiveTestContainsText(&child, value) {
				return true
			}
		}
	}
	return false
}

func findResponsiveTestElementByKey(element *giodom.Element, key giodom.Key) *giodom.Element {
	if element == nil {
		return nil
	}
	if element.Key == key {
		return element
	}
	if element.Responsive.Compact != nil {
		if found := findResponsiveTestElementByKey(element.Responsive.Compact, key); found != nil {
			return found
		}
	}
	if element.Responsive.Wide != nil {
		if found := findResponsiveTestElementByKey(element.Responsive.Wide, key); found != nil {
			return found
		}
	}
	if element.Children != nil {
		for index := 0; index < element.Children.Len(); index++ {
			child := element.Children.At(index)
			if found := findResponsiveTestElementByKey(&child, key); found != nil {
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

func layoutResponsiveLooseElement(renderer *Renderer, element giodom.Element, width, height int) layout.Dimensions {
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1_800_000_000, 0),
		Constraints: layout.Constraints{Min: image.Point{}, Max: image.Pt(width, height)},
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
