//go:build darwin || linux || windows

package gio

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

func TestCiwiFontCollectionContainsExplicitMonospaceFaces(t *testing.T) {
	faces, err := ciwiFontCollection()
	if err != nil {
		t.Fatal(err)
	}
	want := map[font.Weight]bool{font.Normal: false, font.Medium: false, font.Bold: false}
	for _, face := range faces {
		if face.Font.Typeface != font.Typeface("Ciwi Mono") {
			continue
		}
		if _, ok := want[face.Font.Weight]; ok {
			want[face.Font.Weight] = true
		}
	}
	for weight, found := range want {
		if !found {
			t.Errorf("Ciwi Mono weight %v is not embedded in the native font collection", weight)
		}
	}
}

func TestAwaitingPaletteTextContrastAcrossThemes(t *testing.T) {
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range themes {
		background, parseErr := parseColor(theme.Theme.Colors["awaiting-surface"])
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, token := range []string{"awaiting-text"} {
			foreground, foregroundErr := parseColor(theme.Theme.Colors[token])
			if foregroundErr != nil {
				t.Fatal(foregroundErr)
			}
			if ratio := contrastRatio(background, foreground); ratio < 4.5 {
				t.Errorf("theme %q %s contrast = %.2f, want at least 4.5", theme.Metadata.Name, token, ratio)
			}
		}
	}
}

func contrastRatio(first, second color.NRGBA) float64 {
	firstLuminance := relativeLuminance(first)
	secondLuminance := relativeLuminance(second)
	return (max(firstLuminance, secondLuminance) + .05) / (min(firstLuminance, secondLuminance) + .05)
}

func relativeLuminance(value color.NRGBA) float64 {
	linear := func(channel uint8) float64 {
		normalized := float64(channel) / 255
		if normalized <= .04045 {
			return normalized / 12.92
		}
		return math.Pow((normalized+.055)/1.055, 2.4)
	}
	return .2126*linear(value.R) + .7152*linear(value.G) + .0722*linear(value.B)
}

func TestCSSGradientLineUsesWebAngleCoordinates(t *testing.T) {
	start, end := cssGradientLine(image.Rect(0, 0, 200, 100), 145)
	if end.X <= start.X || end.Y <= start.Y {
		t.Fatalf("145-degree CSS gradient line = %v -> %v, want down and right", start, end)
	}
	centerX, centerY := (start.X+end.X)/2, (start.Y+end.Y)/2
	if math.Abs(float64(centerX-100)) > .0001 || math.Abs(float64(centerY-50)) > .0001 {
		t.Fatalf("gradient center = (%g,%g), want (100,50)", centerX, centerY)
	}
}

func TestRenderedPageBackgroundIsContinuousAcrossFormerSplit(t *testing.T) {
	background := renderPageBackground(image.Pt(320, 180), palette{
		backgroundStart: color.NRGBA{R: 76, G: 42, B: 132, A: 255},
		background:      color.NRGBA{R: 15, G: 22, B: 52, A: 255},
		backgroundEnd:   color.NRGBA{R: 32, G: 84, B: 113, A: 255},
		backgroundGlowA: color.NRGBA{R: 126, G: 75, B: 207, A: 255},
		backgroundGlowB: color.NRGBA{R: 65, G: 195, B: 231, A: 255},
	})
	maximumDelta := 0
	for y := 1; y < background.Bounds().Dy()-1; y++ {
		for x := 1; x < background.Bounds().Dx()-1; x++ {
			current := background.NRGBAAt(x, y)
			for _, neighbor := range []color.NRGBA{background.NRGBAAt(x+1, y), background.NRGBAAt(x, y+1)} {
				for _, delta := range []int{absInt(int(current.R) - int(neighbor.R)), absInt(int(current.G) - int(neighbor.G)), absInt(int(current.B) - int(neighbor.B))} {
					maximumDelta = max(maximumDelta, delta)
				}
			}
		}
	}
	if maximumDelta > 3 {
		t.Fatalf("largest adjacent gradient-channel delta = %d, want a continuous raster without a diagonal seam", maximumDelta)
	}
}

func TestGradientTextureSizeCapsRasterWorkAndPreservesAspect(t *testing.T) {
	for _, test := range []struct {
		target image.Point
		want   image.Point
	}{
		{target: image.Pt(320, 180), want: image.Pt(320, 180)},
		{target: image.Pt(1920, 1080), want: image.Pt(384, 216)},
		{target: image.Pt(1080, 1920), want: image.Pt(216, 384)},
		{target: image.Pt(1900, 170), want: image.Pt(384, 34)},
	} {
		if got := gradientTextureSize(test.target); got != test.want {
			t.Errorf("gradientTextureSize(%v) = %v, want %v", test.target, got, test.want)
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func TestExecutionGridWeightsMatchSharedTableColumns(t *testing.T) {
	for _, test := range []struct {
		role     string
		children int
	}{
		{role: "queued-execution-header", children: 8},
		{role: "queued-execution-job-row", children: 8},
		{role: "history-execution-header", children: 7},
		{role: "history-execution-job-row", children: 7},
	} {
		if got := executionGridWeights(test.role, test.children); len(got) != test.children {
			t.Errorf("executionGridWeights(%q, %d) has %d columns", test.role, test.children, len(got))
		}
	}
	if got := executionGridWeights("queued-execution-header", 7); got != nil {
		t.Fatalf("mismatched table columns = %v, want nil", got)
	}
}

func TestVisibleOutputGroupStateTracksFirstVisibleExpandedStep(t *testing.T) {
	renderer := &Renderer{disclosures: map[string]bool{"step:2": true}}
	items := []any{
		map[string]any{"state_key": "step:1"},
		map[string]any{"state_key": "step:2"},
	}
	if key, expanded := renderer.visibleOutputGroupState(items, 1); key != "step:2" || !expanded {
		t.Fatalf("visible output group = %q, %v", key, expanded)
	}
	if key, expanded := renderer.visibleOutputGroupState(items, 0); key != "step:1" || expanded {
		t.Fatalf("collapsed output group = %q, %v", key, expanded)
	}
	if key, expanded := renderer.visibleOutputGroupState(items, 2); key != "" || expanded {
		t.Fatalf("out-of-range output group = %q, %v", key, expanded)
	}
}

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
	controls, err := sharedUI.LoadControls()
	if err != nil {
		t.Fatal(err)
	}
	if renderer.controls != controls.Controls {
		t.Fatalf("native controls = %#v, want shared contract %#v", renderer.controls, controls.Controls)
	}
	if logo, ok := renderer.images["ciwi-logo"]; !ok || logo.Filter != paint.FilterNearest {
		t.Fatal("ciwi logo must use nearest-neighbor filtering")
	}
	typography, err := sharedUI.LoadTypography()
	if err != nil {
		t.Fatal(err)
	}
	wantBodyTypeface := font.Typeface(typography.Typography.Families["body"])
	if renderer.theme.Face != wantBodyTypeface {
		t.Fatalf("native body typeface = %q, want shared typography family %q", renderer.theme.Face, wantBodyTypeface)
	}
	if got := renderer.nativeTextStyle("body", false).font.Weight; got != font.Weight(50) {
		t.Fatalf("native regular text weight = %v, want 450-equivalent", got)
	}
	if got := renderer.nativeTextStyle("body", true).font.Weight; got != font.Bold {
		t.Fatalf("native strong text weight = %v, want bold", got)
	}
	wantAccentStrong, err := parseColor(theme.Theme.Colors["accent-strong"])
	if err != nil {
		t.Fatal(err)
	}
	if renderer.palette.accentStrong != wantAccentStrong {
		t.Fatalf("native strong accent = %v, want shared theme token %v", renderer.palette.accentStrong, wantAccentStrong)
	}
	wantPill, err := parseColor(theme.Theme.Colors["pill-background"])
	if err != nil {
		t.Fatal(err)
	}
	if renderer.palette.pillBackground != wantPill {
		t.Fatalf("native pill background = %v, want shared theme token %v", renderer.palette.pillBackground, wantPill)
	}
	for token, got := range map[string]color.NRGBA{
		"notice-background": renderer.palette.noticeBackground,
		"notice-text":       renderer.palette.noticeText,
		"notice-border":     renderer.palette.noticeBorder,
		"awaiting-surface":  renderer.palette.awaitingSurface,
		"awaiting-border":   renderer.palette.awaitingBorder,
		"awaiting-text":     renderer.palette.awaitingText,
		"console-surface":   renderer.palette.consoleSurface,
		"console-text":      renderer.palette.consoleText,
		"console-muted":     renderer.palette.consoleMuted,
		"console-accent":    renderer.palette.consoleAccent,
		"console-success":   renderer.palette.consoleSuccess,
	} {
		want, parseErr := parseColor(theme.Theme.Colors[token])
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if got != want {
			t.Errorf("native %s = %v, want shared theme token %v", token, got, want)
		}
	}
	if renderer.metrics.textBody != 16 || renderer.metrics.textControl != 14 || renderer.metrics.textHeading != 18 || renderer.metrics.textJobTitle != 20 || renderer.metrics.textTitle != 28 {
		t.Fatalf("native type scale = body %v control %v heading %v job title %v title %v, want browser 16/14/18/20/28", renderer.metrics.textBody, renderer.metrics.textControl, renderer.metrics.textHeading, renderer.metrics.textJobTitle, renderer.metrics.textTitle)
	}
	if renderer.metrics.heroPadding != 16 || renderer.metrics.imageBrandWidth != 110 || renderer.metrics.imageBrandHeight != 91 {
		t.Fatalf("native masthead metrics = padding %v image %vx%v, want browser 16 and 110x91", renderer.metrics.heroPadding, renderer.metrics.imageBrandWidth, renderer.metrics.imageBrandHeight)
	}
	projectIcon, err := sharedUI.Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: "v0.2.0"},
		Projects: []*cnpv1.ProjectSummary{{
			Id: 1, Name: "ciwi", RepoUrl: "https://github.com/izzyreal/ciwi",
			PipelineCountLabel: "1 pipeline",
			ProjectIcon:        projectIcon, ProjectIconContentType: "image/png",
			Pipelines:      []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "build", SupportsDryRun: true}},
			PipelineChains: []*cnpv1.PipelineChainSummary{{Id: "build+release", Name: "Build and release", SequenceLabel: "build → release", SupportsDryRun: true}},
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
	if len(renderer.dynamicImages) != 1 {
		t.Fatalf("expanded front-page project images = %d, want 1", len(renderer.dynamicImages))
	}
	var foundTitle bool
	var foundChain bool
	var foundPipelineCount bool
	var foundQueuedEmpty bool
	var foundHistoryEmpty bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "ciwi" {
			foundTitle = true
		}
		if selectable.Text() == "Chain: Build and release" {
			foundChain = true
		}
		if selectable.Text() == "1 pipeline" {
			foundPipelineCount = true
		}
		if selectable.Text() == "No queued jobs." {
			foundQueuedEmpty = true
		}
		if selectable.Text() == "No execution history." {
			foundHistoryEmpty = true
		}
	}
	if !foundTitle {
		t.Fatal("front-page title is not rendered as selectable text")
	}
	var foundVersion bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "Server v0.2.0" {
			foundVersion = true
			break
		}
	}
	if !foundVersion {
		t.Fatal("front-page version is not rendered as a compact badge")
	}
	if !foundChain {
		t.Fatal("front-page pipeline chain is not rendered")
	}
	if !foundPipelineCount {
		t.Fatal("front-page project summary does not show its compact pipeline count")
	}
	if !foundQueuedEmpty || !foundHistoryEmpty {
		t.Fatalf("front-page empty states = queued %v history %v, want both visible", foundQueuedEmpty, foundHistoryEmpty)
	}
	buttonsWithDryRun := len(renderer.buttons)
	view := &cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: "v0.2.0"},
		Projects: []*cnpv1.ProjectSummary{{
			Id: 1, Name: "ciwi", RepoUrl: "https://github.com/izzyreal/ciwi",
			Pipelines:      []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "build"}},
			PipelineChains: []*cnpv1.PipelineChainSummary{{Id: "build+release", Name: "Build and release", SequenceLabel: "build → release"}},
		}},
	}
	withoutDryRun, err := frontPageBindingData(view)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	comparison.SetData(withoutDryRun)
	operations.Reset()
	comparison.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if got := buttonsWithDryRun - len(comparison.buttons); got != 2 {
		t.Fatalf("dry-run controls added %d buttons, want one pipeline and one chain button", got)
	}
	if _, ok := renderer.images["ciwi-logo"]; !ok {
		t.Fatal("embedded ciwi logo is unavailable to the native renderer")
	}
	if renderer.icons["settings"] == nil || renderer.icons["vault"] == nil || renderer.icons["player-play"] == nil || renderer.icons["loader-2"] == nil || renderer.icons["arrow-left"] == nil || renderer.icons["trash"] == nil || renderer.icons["zoom-in"] == nil || renderer.icons["zoom-out"] == nil {
		t.Fatal("declared screen icons are unavailable to the native renderer")
	}
}

func TestExecutionCardBindingsPreserveServerSummary(t *testing.T) {
	cards := []any{map[string]any{
		"summary_label": "12/17 successful, 3 in progress, 2 waiting",
		"status":        "running", "summary_tone": "warning",
	}}
	ensureExecutionCardBindings(cards)
	card := cards[0].(map[string]any)
	if got := card["summary_label"]; got != "12/17 successful, 3 in progress, 2 waiting" {
		t.Fatalf("summary label = %q", got)
	}
	if card["status"] != "running" || card["summary_tone"] != "warning" {
		t.Fatalf("status decoration = %#v", card)
	}
}

func TestSharedScreenIconsExistInNativeRenderer(t *testing.T) {
	routes, err := sharedUI.LoadRoutes()
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{}
	var visit func(uidsl.Node)
	visit = func(node uidsl.Node) {
		if node.Icon != "" {
			required[node.Icon] = true
		}
		if node.Disclosure != nil {
			for _, child := range node.Disclosure.Summary {
				visit(child)
			}
		}
		if node.GraphView != nil {
			for _, child := range node.GraphView.Details {
				visit(child)
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	loaded := map[string]bool{}
	for _, route := range routes.Routes {
		if loaded[route.Screen] {
			continue
		}
		loaded[route.Screen] = true
		screen, loadErr := sharedUI.LoadScreen(route.Screen)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		visit(screen.Screen.Root)
	}
	icons := tablerIcons()
	for icon := range required {
		if icons[icon] == nil {
			t.Errorf("native icon renderer is missing declarative icon %q", icon)
		}
	}
}

func TestRendererExpandsExecutionCardWithoutNavigating(t *testing.T) {
	screen, err := sharedUI.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	var navigated bool
	renderer, err := NewRenderer(screen, theme, func(action uidsl.Action, _ map[string]string) {
		navigated = action.Command == "navigate"
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: "v0.2.0"},
		HistoryExecutions: []*cnpv1.ExecutionCardSummary{{
			Key: "pipeline:build", Title: "ciwi build", Summary: &cnpv1.ExecutionSummary{TotalJobs: 1, Succeeded: 1},
			JobExecutionIds: []string{"job-1", "job-2"}, Status: "succeeded", SummaryTone: "success",
			SummaryLabel: "1/1 successful", JobExecutionIdsCsv: "job-1,job-2",
			Sections: []*cnpv1.ExecutionCardSection{{Key: "build", Label: "build", Jobs: []*cnpv1.ExecutionCardJob{{Id: "job-1", Label: "linux", Status: "succeeded"}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	status, err := uidsl.Resolve(data, "frontPage.history_executions.0.status")
	if err != nil || status != "succeeded" {
		t.Fatalf("history execution status = %v, err=%v", status, err)
	}
	ids, err := uidsl.Resolve(data, "frontPage.history_executions.0.job_execution_ids_csv")
	if err != nil || ids != "job-1,job-2" {
		t.Fatalf("history execution ids = %v, err=%v", ids, err)
	}
	if got := splitExecutionIDs(" job-1,job-2 ,, "); len(got) != 2 || got[0] != "job-1" || got[1] != "job-2" {
		t.Fatalf("split execution ids = %v", got)
	}
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if navigated {
		t.Fatal("collapsed execution card navigated while being laid out")
	}
	const historyStateKey = "front-history:pipeline:build"
	var historyDisclosure string
	for key := range renderer.buttons {
		if strings.Contains(key, "pipeline:build") && strings.HasSuffix(key, "/disclosure-toggle") {
			historyDisclosure = strings.TrimSuffix(key, "/disclosure-toggle")
			break
		}
	}
	if historyDisclosure == "" {
		t.Fatal("history execution is not rendered as a disclosure")
	}
	if summary := renderer.selectable(historyDisclosure + "/summary/1"); summary.Text() != "1/1 successful" {
		t.Fatalf("history summary = %q", summary.Text())
	}
	deleteButton := renderer.buttons[historyDisclosure+"/summary/2"]
	if deleteButton == nil {
		t.Fatal("collapsed history execution does not expose its delete action")
	}
	deleteButton.Click()
	// Gio reports the same pointer release to the row-sized disclosure target.
	// The nested destructive control must own it exclusively.
	renderer.button(historyDisclosure + "/disclosure-toggle").Click()
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if renderer.pending == nil || renderer.pending.action.Command != "delete-execution" {
		t.Fatal("history header delete action did not request confirmation")
	}
	if renderer.disclosures[historyStateKey] {
		t.Fatal("history delete action also activated its parent disclosure")
	}
	renderer.pending = nil
	label := renderer.selectable(historyDisclosure + "/label")
	label.SetCaret(0, 2)
	renderer.button(historyDisclosure + "/disclosure-label").Click()
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if renderer.disclosures[historyStateKey] {
		t.Fatal("selecting disclosure label text unexpectedly expanded the card")
	}
	label.ClearSelection()
	renderer.button(historyDisclosure + "/disclosure-label").Click()
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if !renderer.disclosures[historyStateKey] {
		t.Fatal("clicking disclosure label did not expand the card")
	}
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	var foundJob bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "linux" {
			foundJob = true
		}
	}
	if !foundJob {
		t.Fatal("expanded execution card does not show its job rows")
	}
}

func TestNestedButtonConsumesGenericParentActivation(t *testing.T) {
	screen := &uidsl.ScreenDocument{
		Metadata: uidsl.Metadata{Name: "nested-action"},
		Screen: uidsl.Screen{Root: uidsl.Node{Component: "page", Children: []uidsl.Node{{
			Component: "card",
			Actions:   []uidsl.Action{{Command: "navigate", Arguments: map[string]string{"route": "/jobs/job-1"}}},
			Children: []uidsl.Node{{
				Component: "button", Text: &uidsl.Text{Literal: "Delete"},
				Actions: []uidsl.Action{{Command: "delete-execution", Confirm: &uidsl.Confirmation{Title: "Delete?", Message: "Confirm deletion."}}},
			}},
		}}}},
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	var navigated bool
	renderer, err := NewRenderer(screen, theme, func(action uidsl.Action, _ map[string]string) {
		navigated = navigated || action.Command == "navigate"
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(map[string]any{"ready": true})
	const parentPath = "nested-action/root/0"
	renderer.button(parentPath).Click()
	renderer.button(parentPath + "/0").Click()
	renderer.Layout(layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(600, 400))})
	if navigated {
		t.Fatal("nested button activation propagated to its navigable parent")
	}
	if renderer.pending == nil || renderer.pending.action.Command != "delete-execution" {
		t.Fatalf("nested button confirmation = %#v", renderer.pending)
	}
}

func TestGraphRunButtonConsumesGraphNodeSelection(t *testing.T) {
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	var ran bool
	renderer, err := NewRenderer(&uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "graph-interaction"}}, theme, func(action uidsl.Action, _ map[string]string) {
		ran = ran || action.Command == "run-pipeline"
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := uidsl.Node{
		GraphView: &uidsl.GraphView{Details: []uidsl.Node{{Component: "text", Text: &uidsl.Text{Literal: "Details"}}}},
		Actions:   []uidsl.Action{{Command: "run-pipeline"}},
	}
	graphNode := &definitionGraphNode{id: "build", label: "build", data: map[string]any{"id": "build"}}
	const path = "graph/node/build"
	renderer.button(path + "/select").Click()
	renderer.button(path + "/run").Click()
	renderer.layoutDefinitionGraphNode(
		layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(240, 90))},
		owner, graphNode, map[string]any{}, path, "project-1", false,
	)
	if !ran {
		t.Fatal("graph run button did not dispatch its action")
	}
	if selected := renderer.graphSelections["project-1"]; selected != "" {
		t.Fatalf("graph run button also selected node %q", selected)
	}
}

func TestGraphRootRunActionIsIndependentFromSelection(t *testing.T) {
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(&uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "graph-root-interaction"}}, theme, func(uidsl.Action, map[string]string) {})
	if err != nil {
		t.Fatal(err)
	}
	owner := uidsl.Node{GraphView: &uidsl.GraphView{
		Details: []uidsl.Node{{Component: "text", Text: &uidsl.Text{Literal: "Details"}}},
		Root: &uidsl.GraphRoot{
			ActionVisible: &uidsl.Condition{Binding: "graphRoot.runnable"},
			Actions: []uidsl.Action{{On: "activate", Command: "run-chain", Arguments: map[string]string{
				"projectId": "{{graphRoot.project_id}}", "chainId": "{{graphRoot.chain_id}}",
			}, Confirm: &uidsl.Confirmation{Title: "Run chain", Message: "Queue the chain."}}},
		},
	}}
	graphNode := &definitionGraphNode{
		id: "__root__:chain:release", label: "Chain: Release", root: true,
		data: map[string]any{"graphRoot": map[string]any{"runnable": true, "project_id": "1", "chain_id": "release"}},
	}
	const path = "graph/node/__root__:chain:release"
	renderer.button(path + "/run").Click()
	renderer.layoutDefinitionGraphNode(
		layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(240, 90))},
		owner, graphNode, map[string]any{}, path, "project-1", false,
	)
	if renderer.pending == nil || renderer.pending.action.Command != "run-chain" || renderer.pending.arguments["projectId"] != "1" || renderer.pending.arguments["chainId"] != "release" {
		t.Fatalf("graph root confirmation = %#v", renderer.pending)
	}
	if selected := renderer.graphSelections["project-1"]; selected != "" {
		t.Fatalf("graph root action selected %q", selected)
	}

	renderer.pending = nil
	graphNode.data.(map[string]any)["graphRoot"].(map[string]any)["runnable"] = false
	renderer.button(path + "/run").Click()
	renderer.layoutDefinitionGraphNode(
		layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(240, 90))},
		owner, graphNode, map[string]any{}, path, "project-1", false,
	)
	if renderer.pending != nil {
		t.Fatal("informational graph root dispatched its hidden action")
	}
}

func TestResolveDefinitionGraphNodesPrependsRootToVisibleEntries(t *testing.T) {
	graph := uidsl.GraphView{
		Nodes: "view.nodes", As: "item", NodeKey: "item.id", NodeLabel: uidsl.Text{Binding: "item.id"}, Dependencies: "item.needs",
		Root: &uidsl.GraphRoot{Binding: "view.root", As: "graphRoot", Key: "graphRoot.id", Label: uidsl.Text{Binding: "graphRoot.label"}, Meta: uidsl.Text{Binding: "graphRoot.meta"}},
	}
	data := map[string]any{"view": map[string]any{
		"root": map[string]any{"id": "pipeline:7", "label": "Pipeline: build", "meta": "2 jobs"},
		"nodes": []any{
			map[string]any{"id": "build", "needs": []any{}},
			map[string]any{"id": "release", "needs": []any{"build"}},
			map[string]any{"id": "detached", "needs": []any{"outside-filter"}},
		},
	}}
	nodes, err := resolveDefinitionGraphNodes(graph, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 4 || !nodes[0].root || nodes[0].label != "Pipeline: build" {
		t.Fatalf("resolved graph root = %#v", nodes)
	}
	rootID := nodes[0].id
	if !slices.Contains(nodes[1].dependencies, rootID) || slices.Contains(nodes[2].dependencies, rootID) || !slices.Contains(nodes[3].dependencies, rootID) {
		t.Fatalf("root dependencies = build %v, release %v, detached %v", nodes[1].dependencies, nodes[2].dependencies, nodes[3].dependencies)
	}
	if selected := defaultDefinitionGraphNode(nodes); selected == nil || selected.id != "build" {
		t.Fatalf("default graph selection = %#v, want build", selected)
	}
	layoutDefinitionGraph(nodes, 210, 76, 58, 24, 16)
	if nodes[0].level != 0 || nodes[1].level != 1 || nodes[2].level != 2 || nodes[3].level != 1 {
		t.Fatalf("rooted graph levels = %d, %d, %d, %d", nodes[0].level, nodes[1].level, nodes[2].level, nodes[3].level)
	}
}

func TestDefinitionGraphNodeSurfaceUsesSharedSelectionSemantics(t *testing.T) {
	colors := palette{
		surface: color.NRGBA{R: 10, G: 20, B: 30, A: 255},
		border:  color.NRGBA{R: 40, G: 50, B: 60, A: 255},
		accent:  color.NRGBA{R: 70, G: 80, B: 90, A: 255},
		focus:   color.NRGBA{R: 100, G: 110, B: 120, A: 255},
	}

	border, background, width := definitionGraphNodeSurface(colors, true, false)
	if border != colors.accent || background == colors.surface || width != 1 {
		t.Fatalf("hover surface = (%v, %v, %v), want accent, tinted surface, 1dp", border, background, width)
	}

	border, background, width = definitionGraphNodeSurface(colors, true, true)
	if border != colors.focus || background == colors.surface || width != 2 {
		t.Fatalf("selected hover surface = (%v, %v, %v), want focus, tinted surface, 2dp", border, background, width)
	}
}

func TestHiddenNodesDoNotOccupyNativeFlexGaps(t *testing.T) {
	renderer := &Renderer{}
	visible := uidsl.Node{Visible: &uidsl.Condition{Binding: "item.visible", Equals: "true"}}
	data := map[string]any{"item": map[string]any{"visible": false}}
	if renderer.nodeOccupiesLayout(visible, data) {
		t.Fatal("conditionally hidden node still occupies native layout")
	}
	visible.Visible.Not = true
	if !renderer.nodeOccupiesLayout(visible, data) {
		t.Fatal("negated visible condition was removed from native layout")
	}
	visible.Visible = &uidsl.Condition{Binding: "item.missing", Equals: "true"}
	if !renderer.nodeOccupiesLayout(visible, data) {
		t.Fatal("unresolved visibility binding must remain in layout to render its error")
	}
}

func TestRendererPersistsProjectDisclosureAndBulkExecutionState(t *testing.T) {
	screen, err := sharedUI.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]bool
	renderer.SetDisclosureChange(func(states map[string]bool) { persisted = states })
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: "v0.2.0"},
		Projects: []*cnpv1.ProjectSummary{{
			Id: 1, Name: "ciwi", RepoUrl: "https://github.com/izzyreal/ciwi", RepoRef: "main", ConfigFile: "ciwi-project.yaml",
			PipelineCountLabel: "1 pipeline",
			Pipelines:          []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "build"}},
		}},
		HistoryExecutions: []*cnpv1.ExecutionCardSummary{{
			Key: "pipeline:build", Title: "ciwi build", Summary: &cnpv1.ExecutionSummary{TotalJobs: 1, Succeeded: 1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))}
	renderer.Layout(gtx)
	if !renderer.disclosures["front-project:1"] {
		t.Fatal("project disclosure did not default to expanded")
	}
	var projectPath string
	for path := range renderer.buttons {
		if strings.HasSuffix(path, "/disclosure-header") {
			projectPath = strings.TrimSuffix(path, "/disclosure-header")
			break
		}
	}
	if projectPath == "" {
		t.Fatal("project disclosure label was not rendered")
	}
	summaryLabels := map[string]bool{}
	for _, selectable := range renderer.selectables {
		summaryLabels[selectable.Text()] = true
	}
	for _, expected := range []string{"ciwi", "https://github.com/izzyreal/ciwi", "branch:main", "ciwi-project.yaml", "1 pipeline"} {
		if !summaryLabels[expected] {
			t.Errorf("collapsed project summary is missing %q", expected)
		}
	}
	renderer.button(projectPath + "/disclosure-header").Click()
	operations.Reset()
	renderer.Layout(gtx)
	if renderer.disclosures["front-project:1"] || persisted["front-project:1"] {
		t.Fatalf("collapsed project state was not persisted: state=%v persisted=%v", renderer.disclosures, persisted)
	}
	renderer.button(projectPath + "/disclosure-header").Click()
	operations.Reset()
	renderer.Layout(gtx)
	if !renderer.disclosures["front-project:1"] || !persisted["front-project:1"] {
		t.Fatalf("clicking the project row did not expand it: state=%v persisted=%v", renderer.disclosures, persisted)
	}
	projectName := renderer.selectable(projectPath + "/summary/0")
	projectName.SetCaret(0, 2)
	renderer.button(projectPath + "/disclosure-header").Click()
	operations.Reset()
	renderer.Layout(gtx)
	if !renderer.disclosures["front-project:1"] {
		t.Fatal("selecting project-row text unexpectedly collapsed the project")
	}
	projectName.ClearSelection()
	var navigatedRoute string
	renderer.onAction = func(action uidsl.Action, arguments map[string]string) {
		if action.Command == "navigate" {
			navigatedRoute = arguments["route"]
		}
	}
	renderer.button(projectPath + "/summary/0").Click()
	operations.Reset()
	renderer.Layout(gtx)
	if navigatedRoute != "/projects/1" {
		t.Fatalf("project-name link route = %q, want /projects/1", navigatedRoute)
	}
	if !renderer.disclosures["front-project:1"] {
		t.Fatal("clicking the project-name link unexpectedly toggled the project row")
	}
	renderer.dispatchFromLayout(gtx, uidsl.Action{
		Command: "set-disclosures", Arguments: map[string]string{"prefix": "front-history:", "expanded": "true"},
	}, data)
	if !renderer.disclosures["front-history:pipeline:build"] || !persisted["front-history:pipeline:build"] {
		t.Fatalf("bulk-expanded history state was not persisted: state=%v persisted=%v", renderer.disclosures, persisted)
	}
}

func TestRendererLaysOutSharedProjectDetails(t *testing.T) {
	screen, err := sharedUI.LoadScreen("project-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	projectIcon, err := sharedUI.Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := projectDetailsBindingData(&cnpv1.ProjectDetailsView{
		Project: &cnpv1.ProjectSummary{Id: 1, Name: "ciwi", ProjectIcon: projectIcon, ProjectIconContentType: "image/png", PipelineChains: []*cnpv1.PipelineChainSummary{{
			Id: "build+release", Name: "Build and release", SequenceLabel: "build → release", SupportsDryRun: true,
		}}},
		Pipelines: []*cnpv1.ProjectPipelineDetails{{
			Id: 7, PipelineId: "build", Dependencies: "none", JobsCount: 1, SupportsDryRun: true,
			SummaryLabel: "1 job · depends on: none", GraphSummaryLabel: "1 job · 0 dependencies",
			Jobs: []*cnpv1.ProjectJobDetails{{
				Id: "compile", StepsCount: 1, SupportsDryRun: true, RunsOnLabel: "darwin/arm64", ToolsLabel: "go=1.25", TimeoutSeconds: 600, MatrixCount: 1,
				SummaryLabel: "1 step · runs on: darwin/arm64", TimeoutLabel: "10m 0s", MatrixLabel: "1 combination",
				Steps: []*cnpv1.ProjectStepDetails{{Index: 0, Position: 1, Name: "Compile", Type: "run", Command: "go build ./...", Environment: []string{"CGO_ENABLED=1"}, EnvironmentLabel: "CGO_ENABLED=1", SkipDryRun: true}},
			}},
		}},
		StructureFilters: []*cnpv1.ProjectStructureFilter{
			{Value: "all-pipelines", Label: "All Pipelines", PipelineIds: []string{"build"}, ShowPipelineStructure: true, Root: &cnpv1.ProjectStructureRoot{Id: "project:1:all-pipelines", Label: "ciwi", Meta: "Project · 1 pipeline", ProjectId: 1}},
			{Value: "all-chains", Label: "All chains", ShowChainStructure: true, Root: &cnpv1.ProjectStructureRoot{Id: "project:1:all-chains", Label: "ciwi", Meta: "Project · 1 pipeline chain", ProjectId: 1}},
			{Value: "chain:build+release", Label: "Build and release (chain)", PipelineIds: []string{"build", "release"}, ShowPipelineStructure: true, Root: &cnpv1.ProjectStructureRoot{Id: "chain:build+release", Label: "Chain: Build and release", Meta: "build → release", Runnable: true, ProjectId: 1, ChainId: "build+release"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	renderer.SetViewStates(map[string]string{"project-structure:1": "list"})
	var operations op.Ops
	dimensions := renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if dimensions.Size != image.Pt(1100, 760) {
		t.Fatalf("dimensions = %v", dimensions.Size)
	}
	if len(renderer.dynamicImages) != 1 {
		t.Fatalf("bound project images = %d, want 1", len(renderer.dynamicImages))
	}
	if got := len(renderer.buttons); got < 5 {
		t.Fatalf("project details did not expose pipeline, dry-run, and job controls: %d widgets", got)
	}
	var foundJob, foundPipelineSummary, foundJobSummary bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "Job: compile" {
			foundJob = true
		}
		if selectable.Text() == "1 job · depends on: none" {
			foundPipelineSummary = true
		}
		if selectable.Text() == "1 step · runs on: darwin/arm64" {
			foundJobSummary = true
		}
	}
	if !foundJob {
		t.Fatal("pipeline did not default to expanded")
	}
	if !foundPipelineSummary || !foundJobSummary {
		t.Fatalf("project summaries missing: pipeline=%v job=%v", foundPipelineSummary, foundJobSummary)
	}
	if !renderer.disclosures["project-pipeline:1:7"] {
		t.Fatal("pipeline disclosure did not use its stable expanded state")
	}
	renderer.disclosures["project-job:1:7:compile"] = true
	renderer.disclosures["project-step:1:7:compile:0"] = true
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	var foundCommand, foundEnvironment, foundDryRunNotice bool
	for _, selectable := range renderer.selectables {
		switch selectable.Text() {
		case "go build ./...":
			foundCommand = true
		case "Environment: CGO_ENABLED=1":
			foundEnvironment = true
		case "Skipped during dry runs":
			foundDryRunNotice = true
		}
	}
	for _, editor := range renderer.textEditors {
		if editor.Text() == "go build ./..." {
			foundCommand = true
		}
	}
	if !foundCommand || !foundEnvironment || !foundDryRunNotice {
		t.Fatalf("configured step details missing: command=%v environment=%v dry-run=%v", foundCommand, foundEnvironment, foundDryRunNotice)
	}
	if !renderer.SetProjectStructureFilter("all-chains") {
		t.Fatal("all-chains filter was rejected")
	}
	renderer.SetViewStates(map[string]string{"project-chains:1": "list"})
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	root := renderer.data.(map[string]any)["projectDetails"].(map[string]any)
	if visible := root["visible_pipelines"].([]any); len(visible) != 0 {
		t.Fatalf("all-chains retained pipeline graph nodes: %#v", visible)
	}
	var foundChain, foundChainSequence bool
	for _, selectable := range renderer.selectables {
		foundChain = foundChain || selectable.Text() == "Chain: Build and release"
	}
	for _, editor := range renderer.textEditors {
		if editor.Text() == "build → release" {
			foundChainSequence = true
			if editor.SingleLine {
				t.Fatal("long monospace chain labels must remain wrap-capable")
			}
		}
	}
	if !foundChain || !foundChainSequence {
		t.Fatalf("chain list missing: chain=%v sequence=%v", foundChain, foundChainSequence)
	}
}

func TestRendererLaysOutPersistentProjectPipelineGraph(t *testing.T) {
	screen, err := sharedUI.LoadScreen("project-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := projectDetailsBindingData(&cnpv1.ProjectDetailsView{
		Project: &cnpv1.ProjectSummary{Id: 41, Name: "ciwi"},
		Pipelines: []*cnpv1.ProjectPipelineDetails{
			{Id: 7, PipelineId: "build", Dependencies: "none", JobsCount: 2, SummaryLabel: "2 jobs · depends on: none", GraphSummaryLabel: "2 jobs · 0 dependencies", Jobs: []*cnpv1.ProjectJobDetails{
				{Id: "unit-tests", StepsCount: 1, Steps: []*cnpv1.ProjectStepDetails{{Index: 0, Position: 1, Name: "Test", Type: "run", Command: "go test ./..."}}},
				{Id: "package", Needs: []string{"unit-tests"}, StepsCount: 1, Steps: []*cnpv1.ProjectStepDetails{{Index: 0, Position: 1, Name: "Package", Type: "run", Command: "go build ./..."}}},
			}},
			{Id: 8, PipelineId: "release", DependsOn: []string{"build"}, Dependencies: "build", JobsCount: 1, SummaryLabel: "1 job · depends on: build", GraphSummaryLabel: "1 job · 1 dependency", Jobs: []*cnpv1.ProjectJobDetails{{Id: "publish", StepsCount: 1}}},
		},
		StructureFilters: []*cnpv1.ProjectStructureFilter{{
			Value: "all-pipelines", Label: "All Pipelines", PipelineIds: []string{"build", "release"}, ShowPipelineStructure: true,
			Root: &cnpv1.ProjectStructureRoot{Id: "project:41:all-pipelines", Label: "ciwi", Meta: "Project · 2 pipelines", ProjectId: 41},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if mode := renderer.viewModes["project-structure:41"]; mode != "graph" {
		t.Fatalf("default graph mode = %q", mode)
	}
	var foundBuild, foundRelease, foundBuildMeta bool
	for _, selectable := range renderer.selectables {
		switch selectable.Text() {
		case "build":
			foundBuild = true
		case "release":
			foundRelease = true
		case "2 jobs · 0 dependencies":
			foundBuildMeta = true
		}
	}
	for _, editor := range renderer.textEditors {
		switch editor.Text() {
		case "build":
			foundBuild = true
		case "release":
			foundRelease = true
		}
	}
	if !foundBuild || !foundRelease || !foundBuildMeta {
		t.Fatalf("graph copy missing: build=%v release=%v meta=%v", foundBuild, foundRelease, foundBuildMeta)
	}
	var buildRun, releaseRun bool
	for path := range renderer.buttons {
		buildRun = buildRun || strings.HasSuffix(path, "/graph/node/build/run")
		releaseRun = releaseRun || strings.HasSuffix(path, "/graph/node/release/run")
	}
	if !buildRun || !releaseRun {
		t.Fatal("pipeline graph did not expose per-node run controls")
	}
	if selected := renderer.graphSelections["project-structure:41"]; selected != "build" {
		t.Fatalf("default selected pipeline = %q, want build", selected)
	}
	var buildSelect, releaseSelect, unitSelect bool
	for path := range renderer.buttons {
		buildSelect = buildSelect || strings.HasSuffix(path, "/graph/node/build/select")
		releaseSelect = releaseSelect || strings.HasSuffix(path, "/graph/node/release/select")
		unitSelect = unitSelect || strings.HasSuffix(path, "/details/build/1/graph/node/unit-tests/select")
	}
	if !buildSelect || !releaseSelect || !unitSelect {
		t.Fatalf("selectable graph nodes missing: build=%v release=%v unit=%v", buildSelect, releaseSelect, unitSelect)
	}
	for path := range renderer.buttons {
		if strings.HasSuffix(path, "/zoom-in") || strings.HasSuffix(path, "/zoom-out") {
			t.Fatalf("native graph retained a discrete zoom button at %q", path)
		}
	}
	if selected := renderer.graphSelections["project-jobs:41:7"]; selected != "unit-tests" {
		t.Fatalf("default selected job = %q, want unit-tests", selected)
	}

	nodes := []*definitionGraphNode{
		{id: "build"},
		{id: "sign", dependencies: []string{"build"}},
		{id: "release", dependencies: []string{"sign"}},
	}
	width, height := layoutDefinitionGraph(nodes, 210, 76, 58, 24, 16)
	if nodes[0].level != 0 || nodes[1].level != 1 || nodes[2].level != 2 {
		t.Fatalf("dependency levels = %d, %d, %d", nodes[0].level, nodes[1].level, nodes[2].level)
	}
	if width <= 3*210 || height < 76 {
		t.Fatalf("graph dimensions = %dx%d", width, height)
	}
}

func TestDefinitionGraphFitCanScaleBelowManualZoomFloor(t *testing.T) {
	scale := definitionGraphFitScale(320, 420, 1200, 700, 16)
	if scale >= definitionGraphMinScale {
		t.Fatalf("portrait fit scale = %.3f, want below the %.2f manual floor", scale, definitionGraphMinScale)
	}
	if gotWidth := float32(1200) * scale; gotWidth > 320-32+0.01 {
		t.Fatalf("fitted graph width = %.2f, viewport content width = %d", gotWidth, 320-32)
	}
}

func TestGraphViewportCentersAndClampsBothAxes(t *testing.T) {
	state := &graphViewportState{}
	state.center(.5, 400, 200, 320, 240)
	if state.offset.X != -60 || state.offset.Y != -70 {
		t.Fatalf("centered offset = %+v, want (-60,-70)", state.offset)
	}
	state.offset = f32.Pt(1000, -1000)
	state.clamp(1, 500, 600, 320, 240)
	if state.offset != f32.Pt(180, 0) {
		t.Fatalf("clamped offset = %+v, want (180,0)", state.offset)
	}
}

func TestGraphViewportUsesTwoTouchesForPanAndZoom(t *testing.T) {
	state := &graphViewportState{lastCentroid: f32.Pt(150, 100), lastDistance: 100}
	scale := float32(1)
	if !state.transformTouch(&scale, f32.Pt(160, 110), 140, .45, 1.5) {
		t.Fatal("two-touch graph gesture did not report a viewport change")
	}
	if scale <= 1 || state.offset == (f32.Point{}) {
		t.Fatalf("two-touch transform = scale %.2f offset %+v", scale, state.offset)
	}
}

func TestGraphViewportReceivesTwoTouchesOutsideNodeSurfaces(t *testing.T) {
	screen := &uidsl.ScreenDocument{
		Metadata: uidsl.Metadata{Name: "graph-viewport-hit-area"},
		Screen: uidsl.Screen{Root: uidsl.Node{Component: "page", Children: []uidsl.Node{{
			Component: "graph-view",
			GraphView: &uidsl.GraphView{
				StateKey: "gesture", DefaultMode: "graph", Nodes: "view.nodes", As: "graphNode", NodeKey: "graphNode.id",
				NodeLabel: uidsl.Text{Binding: "graphNode.id"}, Details: []uidsl.Node{{Component: "text", Text: &uidsl.Text{Literal: "Details"}}},
			},
		}}}},
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(map[string]any{"view": map[string]any{"nodes": []any{
		map[string]any{"id": "one"}, map[string]any{"id": "two"}, map[string]any{"id": "three"},
		map[string]any{"id": "four"}, map[string]any{"id": "five"},
	}}})
	var router input.Router
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(390, 600))}
	renderer.Layout(gtx)
	router.Frame(gtx.Ops)
	var nodeBounds image.Rectangle
	var findNode func(input.SemanticNode)
	findNode = func(node input.SemanticNode) {
		if node.Desc.Description == "Select one" {
			nodeBounds = node.Desc.Bounds
		}
		for _, child := range node.Children {
			findNode(child)
		}
	}
	for _, node := range router.AppendSemantics(nil) {
		findNode(node)
	}
	if nodeBounds.Empty() {
		t.Fatal("graph node semantics missing")
	}
	y := float32(nodeBounds.Min.Y + nodeBounds.Size().Y/2)
	router.Queue(
		pointer.Event{Source: pointer.Touch, Kind: pointer.Press, PointerID: 1, Position: f32.Pt(30, y)},
		pointer.Event{Source: pointer.Touch, Kind: pointer.Press, PointerID: 2, Position: f32.Pt(70, y)},
	)
	gtx.Reset()
	renderer.Layout(gtx)
	router.Frame(gtx.Ops)
	viewport := renderer.graphViewports["gesture"]
	if viewport == nil || !viewport.gestureActive || len(viewport.touches) != 2 {
		t.Fatalf("blank viewport touches did not start a graph gesture: %#v", viewport)
	}
	oldScale := renderer.graphScales["gesture"]
	router.Queue(
		pointer.Event{Source: pointer.Touch, Kind: pointer.Move, PointerID: 1, Position: f32.Pt(25, y+20)},
		pointer.Event{Source: pointer.Touch, Kind: pointer.Move, PointerID: 2, Position: f32.Pt(95, y+20)},
	)
	gtx.Reset()
	renderer.Layout(gtx)
	router.Frame(gtx.Ops)
	if scale := renderer.graphScales["gesture"]; scale == oldScale {
		t.Fatalf("blank viewport pinch left graph scale unchanged at %.3f", scale)
	}
}

func TestGraphViewportEndsWholeGestureWhenEitherTouchEnds(t *testing.T) {
	state := &graphViewportState{}
	if state.pressTouch(1, f32.Pt(100, 100)) {
		t.Fatal("one touch started a graph gesture")
	}
	if !state.pressTouch(2, f32.Pt(200, 100)) {
		t.Fatal("two touches did not start a graph gesture")
	}
	scale := float32(1)
	if !state.dragTouch(2, f32.Pt(240, 100), &scale, definitionGraphMinScale, definitionGraphMaxScale) {
		t.Fatal("active two-touch gesture did not transform the graph")
	}
	state.endTouch(1)
	if state.gestureActive || len(state.touches) != 0 || state.lastDistance != 0 {
		t.Fatalf("ending one participant retained gesture state: %+v", state)
	}
	oldScale, oldOffset := scale, state.offset
	if state.dragTouch(2, f32.Pt(280, 100), &scale, definitionGraphMinScale, definitionGraphMaxScale) {
		t.Fatal("remaining finger continued the ended gesture")
	}
	if scale != oldScale || state.offset != oldOffset {
		t.Fatalf("one-finger movement changed ended gesture: scale=%v offset=%+v", scale, state.offset)
	}
	if state.pressTouch(1, f32.Pt(100, 100)) || !state.pressTouch(2, f32.Pt(200, 100)) {
		t.Fatal("fresh two-finger gesture did not start after cleanup")
	}
}

func TestGraphViewportResetsReusedTouchID(t *testing.T) {
	state := &graphViewportState{}
	state.pressTouch(1, f32.Pt(100, 100))
	state.pressTouch(2, f32.Pt(200, 100))
	if !state.gestureActive {
		t.Fatal("test setup did not start a gesture")
	}
	if state.pressTouch(1, f32.Pt(120, 100)) {
		t.Fatal("reused touch ID retained a stale second participant")
	}
	if state.gestureActive || len(state.touches) != 1 {
		t.Fatalf("reused touch ID state = active %v, touches %v", state.gestureActive, state.touches)
	}
	scale := float32(1)
	if state.dragTouch(1, f32.Pt(160, 100), &scale, definitionGraphMinScale, definitionGraphMaxScale) || scale != 1 {
		t.Fatal("reused single touch transformed the graph")
	}
}

func TestScrollGestureGuardOnlyArmsAfterTouchDrag(t *testing.T) {
	guard := &scrollGestureGuard{}
	list := &layout.List{}
	guard.observe(list)
	if guard.inertial {
		t.Fatal("stationary list armed the momentum tap guard")
	}
	list.Position.Offset = 12
	guard.observe(list)
	if guard.inertial {
		t.Fatal("programmatic list movement armed the momentum tap guard")
	}
	guard.observeState(layout.Position{Offset: 24}, true)
	guard.observeState(layout.Position{Offset: 24}, false)
	if guard.inertial || !guard.inertiaCandidate {
		t.Fatal("released moving drag did not wait for possible fling movement")
	}
	guard.observeState(layout.Position{Offset: 36}, false)
	if !guard.inertial {
		t.Fatal("fling movement after a touch drag did not arm the momentum tap guard")
	}
}

func TestScrollGestureGuardSuppressesMomentumTapThroughRelease(t *testing.T) {
	guard := &scrollGestureGuard{}
	var router input.Router
	var operations op.Ops
	frame := func(events ...pointer.Event) bool {
		if len(events) > 0 {
			queued := make([]event.Event, len(events))
			for index := range events {
				queued[index] = events[index]
			}
			router.Queue(queued...)
		}
		operations.Reset()
		gtx := layout.Context{Ops: &operations, Source: router.Source(), Constraints: layout.Exact(image.Pt(200, 200))}
		suppressed := guard.suppressActivations(gtx)
		area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		pass := pointer.PassOp{}.Push(gtx.Ops)
		event.Op(gtx.Ops, guard)
		pass.Pop()
		area.Pop()
		router.Frame(gtx.Ops)
		return suppressed
	}

	frame()
	guard.inertial = true
	if !frame(pointer.Event{Source: pointer.Touch, Kind: pointer.Press, PointerID: 7, Position: f32.Pt(50, 50)}) {
		t.Fatal("touch beginning during inertia was not suppressed")
	}
	guard.inertial = false
	if !frame(pointer.Event{Source: pointer.Touch, Kind: pointer.Release, PointerID: 7, Position: f32.Pt(50, 50)}) {
		t.Fatal("momentum-stopping touch release was not suppressed")
	}
	if frame() || len(guard.guardedTouches) != 0 {
		t.Fatal("completed momentum-stopping touch retained suppression")
	}
}

func TestGuardedListTouchDuringInertiaCanStartFreshFling(t *testing.T) {
	renderer := &Renderer{}
	list := &layout.List{Axis: layout.Vertical}
	var router input.Router
	var operations op.Ops
	base := time.Now()
	frame := func(elapsed time.Duration, events ...pointer.Event) layout.Position {
		if len(events) > 0 {
			queued := make([]event.Event, len(events))
			for index := range events {
				queued[index] = events[index]
			}
			router.Queue(queued...)
		}
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: base.Add(elapsed),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(200, 240)),
		}
		renderer.suppressTouchActivation = false
		renderer.layoutGuardedList(gtx, "test", list, 20, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(200, 60)}
		})
		router.Frame(gtx.Ops)
		return list.Position
	}

	frame(0)
	renderer.scrollGuards["test"].inertial = true
	frame(time.Millisecond, pointer.Event{
		Source: pointer.Touch, Kind: pointer.Press, PointerID: 3,
		Position: f32.Pt(100, 180), Time: time.Millisecond,
	})
	if !list.Dragging() || !renderer.suppressTouchActivation {
		t.Fatal("touch during inertia neither took over the list nor suppressed activation")
	}
	frame(17*time.Millisecond, pointer.Event{
		Source: pointer.Touch, Kind: pointer.Move, PointerID: 3,
		Position: f32.Pt(100, 120), Time: 17 * time.Millisecond,
	})
	beforeDrag := list.Position
	frame(33*time.Millisecond, pointer.Event{
		Source: pointer.Touch, Kind: pointer.Move, PointerID: 3,
		Position: f32.Pt(100, 60), Time: 33 * time.Millisecond,
	})
	if !list.Dragging() || list.Position == beforeDrag {
		t.Fatal("drag beginning during inertia did not move the list")
	}
	frame(49*time.Millisecond, pointer.Event{
		Source: pointer.Touch, Kind: pointer.Release, PointerID: 3,
		Position: f32.Pt(100, 60), Time: 49 * time.Millisecond,
	})
	if list.Dragging() {
		t.Fatal("released takeover drag remained active")
	}
	beforeFling := list.Position
	frame(99 * time.Millisecond)
	if list.Position == beforeFling {
		t.Fatal("released takeover drag did not start a fresh fling")
	}
}

func TestNativeProjectSummaryWrapsAtItsActualWidth(t *testing.T) {
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(&uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "project-summary-wrap"}}, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	summary := []uidsl.Node{{Component: "text", Text: &uidsl.Text{Literal: "A project with a name"}, Style: uidsl.Style{Role: "link"}}}
	for _, label := range []string{"Managed YAML", "repository", "branch:main", "ciwi-project.yaml"} {
		summary = append(summary, uidsl.Node{Component: "badge", Text: &uidsl.Text{Literal: label}, Style: uidsl.Style{Tone: "muted"}})
	}
	summary = append(summary,
		uidsl.Node{Component: "spacer", Layout: uidsl.Layout{Grow: true}},
		uidsl.Node{Component: "badge", Text: &uidsl.Text{Literal: "4 pipelines"}, Style: uidsl.Style{Tone: "muted"}},
	)
	node := uidsl.Node{Component: "disclosure", Disclosure: &uidsl.Disclosure{Summary: summary}, Style: uidsl.Style{Role: "project-row"}}
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(220, 500)}}
	dimensions := renderer.layoutWrappedProjectSummary(gtx, node, map[string]any{}, "project", func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(14, 14)}
	})
	if dimensions.Size.Y < 60 {
		t.Fatalf("narrow project summary height = %d, want multiple packed rows", dimensions.Size.Y)
	}
}

func TestNativeBadgeFlowsWrapAtTheirActualWidth(t *testing.T) {
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(&uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "project-header-wrap"}}, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"project-header-metadata", "settings-project-summary"} {
		t.Run(role, func(t *testing.T) {
			node := uidsl.Node{
				Component: "row",
				Layout:    uidsl.Layout{Direction: "horizontal", Gap: "small", Wrap: true},
				Style:     uidsl.Style{Role: role},
				Children: []uidsl.Node{
					{Component: "badge", Text: &uidsl.Text{Literal: "https://github.com/izzyreal/vmpc-juce"}, Style: uidsl.Style{Tone: "muted"}},
					{Component: "badge", Text: &uidsl.Text{Literal: "branch:master"}, Style: uidsl.Style{Tone: "muted"}},
					{Component: "badge", Text: &uidsl.Text{Literal: "ciwi-project.yaml"}, Style: uidsl.Style{Tone: "muted"}},
				},
			}
			wideContext := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1000, 500)}}
			wide := renderer.layoutChildren(wideContext, node, map[string]any{}, role)
			narrowContext := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(360, 500)}}
			narrow := renderer.layoutChildren(narrowContext, node, map[string]any{}, role)
			if narrow.Size.X > 360 {
				t.Fatalf("wrapped metadata width = %d, want <= 360", narrow.Size.X)
			}
			if narrow.Size.Y <= wide.Size.Y {
				t.Fatalf("narrow metadata height = %d, wide height = %d; want multiple rows", narrow.Size.Y, wide.Size.Y)
			}
		})
	}
}

func TestCompactProjectHeaderRowCentersNaturalHeightChildren(t *testing.T) {
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(390, 200))}
	var backMinY, titleMinY, logoMinY int
	dimensions := layoutCompactProjectHeaderRow(gtx, 12,
		func(gtx layout.Context) layout.Dimensions {
			backMinY = gtx.Constraints.Min.Y
			return layout.Dimensions{Size: image.Pt(40, 40)}
		},
		func(gtx layout.Context) layout.Dimensions {
			titleMinY = gtx.Constraints.Min.Y
			return layout.Dimensions{Size: image.Pt(180, 42)}
		},
		func(gtx layout.Context) layout.Dimensions {
			logoMinY = gtx.Constraints.Min.Y
			return layout.Dimensions{Size: image.Pt(72, 72)}
		},
	)
	if dimensions.Size.Y != 72 {
		t.Fatalf("compact header height = %d, want logo height 72", dimensions.Size.Y)
	}
	if backMinY != 0 || titleMinY != 0 || logoMinY != 0 {
		t.Fatalf("compact header forced child heights: back=%d title=%d logo=%d", backMinY, titleMinY, logoMinY)
	}
}

func TestConsumedMomentumTouchSuppressesClickableActivation(t *testing.T) {
	renderer := &Renderer{suppressTouchActivation: true}
	button := new(widget.Clickable)
	button.Click()
	if renderer.clicked(layout.Context{Ops: new(op.Ops)}, button) {
		t.Fatal("consumed momentum-stopping touch activated its underlying control")
	}
}

func TestGraphNodeHoverFillIsVisiblyAccentTinted(t *testing.T) {
	surface := color.NRGBA{R: 18, G: 56, B: 36, A: 255}
	accent := color.NRGBA{R: 174, G: 235, B: 66, A: 255}
	hover := graphNodeHoverFill(surface, accent)
	if hover == surface || hover.A != surface.A || hover.G <= surface.G {
		t.Fatalf("hover fill = %#v, surface = %#v", hover, surface)
	}
}

func TestRendererChangesThemeFromSharedSettingsSelect(t *testing.T) {
	screen, err := sharedUI.LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	defaultTheme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	var selected string
	renderer, err := NewRenderer(screen, defaultTheme, func(action uidsl.Action, arguments map[string]string) {
		if action.Command == "change-theme" {
			selected = arguments["theme"]
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := settingsBindingData(&cnpv1.ServerInfo{Name: "ciwi", Version: "v0.2.0", Hostname: "buildbox", ApiVersion: 1}, themes, "default")
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	var spaceOption *widget.Clickable
	for path := range renderer.buttons {
		if !strings.HasSuffix(path, "/select-toggle") {
			continue
		}
		candidate := strings.TrimSuffix(path, "/select-toggle")
		renderer.button(candidate + "/select-toggle").Click()
		operations.Reset()
		renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
		if option := renderer.buttons[candidate+"/option/space"]; option != nil {
			spaceOption = option
			break
		}
	}
	if spaceOption == nil {
		t.Fatal("settings theme options were not expanded")
	}
	spaceOption.Click()
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if selected != "space" {
		t.Fatalf("selected theme = %q", selected)
	}
	spaceTheme, err := findTheme("space")
	if err != nil {
		t.Fatal(err)
	}
	before := renderer.palette.background
	renderer.loaderTexture(image.Pt(18, 18), renderer.palette.accent)
	if len(renderer.loaderTextures) == 0 {
		t.Fatal("loader texture cache was not populated")
	}
	if err := renderer.SetTheme(spaceTheme); err != nil {
		t.Fatal(err)
	}
	if renderer.ThemeName() != "space" {
		t.Fatalf("pending theme name = %q", renderer.ThemeName())
	}
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if renderer.palette.background == before {
		t.Fatal("theme palette did not change")
	}
	if len(renderer.loaderTextures) != 0 {
		t.Fatal("theme change did not clear loader textures")
	}
}

func TestRendererLaysOutNativeConnectionScreen(t *testing.T) {
	screen, err := sharedUI.LoadScreen("connection")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(connectionBindingData(connectionModeExplicit, "tcp://127.0.0.1:8113", "Not connected", false))
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(900, 700))})
	if got := bindingString(renderer.data, "connection.endpoint"); got != "tcp://127.0.0.1:8113" {
		t.Fatalf("connection endpoint = %q", got)
	}
}

func TestSettingsRendersSSHHostFingerprintAsSelectableMonospaceText(t *testing.T) {
	screen, err := sharedUI.LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	data, err := settingsBindingData(&cnpv1.ServerInfo{Name: "ciwi", Version: "v0.2.8"}, themes, "default")
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	const fingerprint = "SHA256:rglajyExampleFingerprint"
	applyConnectionBindings(renderer, "settings", connectionModeSSH, "", sshConnectionSettings{PendingFingerprint: fingerprint})
	renderer.Layout(layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1100, 1400))})
	for _, editor := range renderer.textEditors {
		if editor.Text() == fingerprint {
			if !editor.ReadOnly {
				t.Fatal("SSH host fingerprint editor must be read-only")
			}
			return
		}
	}
	t.Fatal("SSH host fingerprint was not rendered through the monospace text path")
}

func TestSettingsBindingDataUsesSelectedThemeDescription(t *testing.T) {
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	data, err := settingsBindingData(&cnpv1.ServerInfo{Name: "ciwi", Version: "v0.2.0"}, themes, "jungle")
	if err != nil {
		t.Fatal(err)
	}
	root := data["settings"].(map[string]any)
	if root["selected_theme"] != "jungle" || root["selected_theme_description"] == "" {
		t.Fatalf("settings binding = %+v", root)
	}
}

func TestDecorateSettingsUpdateBuildsNativeStatus(t *testing.T) {
	settings := map[string]any{}
	decorateSettingsUpdate(settings, &cnpv1.ServerUpdateStatus{
		CurrentVersion: "v0.2.0", LatestVersion: "v0.2.1", UpdateAvailable: true,
		SelfUpdateSupported: true, LastApplyStatus: "success", Message: "ready",
		BlockedAgentIds: []string{"agent-manual"},
	}, nil)
	if settings["update_supported"] != true {
		t.Fatalf("update_supported = %#v", settings["update_supported"])
	}
	status := fmt.Sprint(settings["update_status_label"])
	if !strings.Contains(status, "Current: v0.2.0") || !strings.Contains(status, "Update available") {
		t.Fatalf("update_status_label = %q", status)
	}
	if !strings.Contains(fmt.Sprint(settings["blocked_agent_notice"]), "agent-manual") {
		t.Fatalf("blocked_agent_notice = %q", settings["blocked_agent_notice"])
	}
}

func TestConditionEnabledUsesDeclarativeBinding(t *testing.T) {
	data := map[string]any{"settings": map[string]any{"supported": true, "selection": ""}}
	if !conditionEnabled(&uidsl.Condition{Binding: "settings.supported"}, data) {
		t.Fatal("true binding was disabled")
	}
	if conditionEnabled(&uidsl.Condition{Binding: "settings.selection", Empty: true, Not: true}, data) {
		t.Fatal("empty selection was enabled")
	}
}

func TestChangeThemeCommandPersistsNativePreference(t *testing.T) {
	screen, err := sharedUI.LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	defaultTheme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, defaultTheme, nil)
	if err != nil {
		t.Fatal(err)
	}
	themes, err := sharedUI.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	data, err := settingsBindingData(&cnpv1.ServerInfo{Name: "ciwi"}, themes, "default")
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	preferencePath := filepath.Join(t.TempDir(), "native-ui.json")
	handleCommand(renderer, &navigationState{screen: "settings"}, commandRequest{
		action: uidsl.Action{Command: "change-theme"}, arguments: map[string]string{"theme": "space"},
	}, preferencePath)
	preferences, err := loadNativePreferences(preferencePath)
	if err != nil {
		t.Fatal(err)
	}
	if preferences.Theme != "space" || renderer.ThemeName() != "space" {
		t.Fatalf("preferences=%+v renderer theme=%q", preferences, renderer.ThemeName())
	}
}

func TestRendererLaysOutSharedJobDetails(t *testing.T) {
	screen, err := sharedUI.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", Title: "Job: compile", Context: "ciwi · pipeline build · execution job-1",
		Status: "running", StatusLabel: "Running", Mode: "Run", Created: "2026-08-02T10:00:00Z",
		CanCancel: true, CanRerun: true,
		Timeline: []*cnpv1.JobTimelineItem{{
			Id: "step:1", Kind: "step", Title: "Job step 1/1: Compile", Status: "in progress", StatusLabel: "In progress",
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
	if got := len(renderer.buttons); got != 10 {
		t.Fatalf("job view created %d clickable widgets, want execution controls, Back, timeline selection, log downloads, and output buttons", got)
	}
	if len(renderer.scrollers) != 1 {
		t.Fatalf("collapsed job view scrollers = %d, want only execution path", len(renderer.scrollers))
	}
	if renderer.outputScroller != nil {
		t.Fatal("collapsed desktop output groups retained a height-filling scroller")
	}
}

func TestRendererLaysOutAuthoritativeJobHeaderAndDetailCards(t *testing.T) {
	screen, err := sharedUI.LoadScreen("job-details")
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
	projectIcon, err := sharedUI.Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", ProjectId: 7, ProjectIcon: projectIcon, Title: "ciwi / build / compile",
		Status: "succeeded", StatusLabel: "Succeeded", Mode: "Ordinary run",
		JobProperties:             []*cnpv1.JobDetailRow{{Label: "Job Execution ID", Value: "job-1"}},
		CacheStatisticsEmpty:      "No cache statistics reported for this job.",
		HostToolRequirements:      &cnpv1.ToolRequirements{Summary: "Requirements matched", Tone: "success"},
		ContainerToolRequirements: &cnpv1.ToolRequirements{EmptyLabel: "No container tool requirements declared for this job."},
		ReleaseSummary:            []*cnpv1.JobDetailRow{{Label: "Tag", Value: "v0.3.0"}}, HasReleaseSummary: true,
		Artifacts:      &cnpv1.ReportDetails{Summary: "1 artifact", Nodes: []*cnpv1.TreeNode{{Label: "dist/app.zip", Detail: "2 KB"}}},
		TestReport:     &cnpv1.ReportDetails{Summary: "1 total · 1 passed", Tone: "success"},
		CoverageReport: &cnpv1.ReportDetails{Summary: "80.00% overall", Nodes: []*cnpv1.TreeNode{{Label: "main.go", Detail: "80.00% · 8/10"}}},
		RunContext: &cnpv1.JobRunContext{Available: true, ScopeLabel: "Pipeline run", Pipelines: []*cnpv1.JobRunContextPipeline{{
			PipelineId: "build", Status: "succeeded", SummaryLabel: "succeeded · 1 job(s)",
			Jobs: []*cnpv1.JobRunContextJob{{Id: "compile", Status: "succeeded", SummaryLabel: "succeeded · 1 execution(s)"}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 2200))})
	renderer.list.ScrollTo(3)
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 2200))})
	if len(renderer.dynamicImages) != 1 {
		t.Fatalf("job header dynamic images = %d, want project icon", len(renderer.dynamicImages))
	}
	wanted := map[string]bool{
		"ciwi / build / compile": false, "Job Properties": false, "Cache Statistics": false,
		"Host Tool Requirements": false, "Container Tool Requirements": false,
		"Release Summary": false, "Run Context": false, "Artifacts": false, "Test Report": false, "Coverage Report": false,
		"dist/app.zip": false, "main.go": false,
	}
	for _, selectable := range renderer.selectables {
		if _, ok := wanted[selectable.Text()]; ok {
			wanted[selectable.Text()] = true
		}
	}
	for _, editor := range renderer.textEditors {
		if _, ok := wanted[editor.Text()]; ok {
			wanted[editor.Text()] = true
		}
	}
	for label, found := range wanted {
		if !found {
			t.Errorf("job details did not render %q", label)
		}
	}
}

func TestJobOutputBindingUsesSelectableReadOnlyEditor(t *testing.T) {
	screen, err := sharedUI.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", Title: "Job: compile", StatusLabel: "Running", Mode: "Run",
		OutputGroups: []*cnpv1.JobOutputGroup{{Id: "step:1", StateKey: "job-output:job-1:step:1", Kind: "step", Title: "Job step 1/1: Compile"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	renderer.SetDisclosureStates(map[string]bool{"job-output:job-1:step:1": true})
	renderer.ApplyJobOutput(jobOutputSnapshot{Outputs: map[string]string{"step:1": "hello\n"}})
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	editor := renderer.outputEditors["step:1"]
	if editor == nil {
		t.Fatalf("output editors = %+v", renderer.outputEditors)
	}
	if !editor.ReadOnly || editor.Text() != "hello\n" {
		t.Fatalf("editor readOnly=%v text=%q", editor.ReadOnly, editor.Text())
	}
}

func TestRendererSelectsTimelineItemAndFindsOutput(t *testing.T) {
	screen, err := sharedUI.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", Title: "Job: compile", StatusLabel: "Succeeded", Mode: "Run",
		Timeline: []*cnpv1.JobTimelineItem{
			{Id: "phase:1", Title: "Prepare workspace", Status: "succeeded", StatusLabel: "Succeeded"},
			{Id: "step:1", Title: "Compile", Status: "succeeded", StatusLabel: "Succeeded"},
		},
		OutputGroups: []*cnpv1.JobOutputGroup{{Id: "step:1", StateKey: "job-output:job-1:step:1", Kind: "step", Title: "Job step 1/1: Compile", Reached: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	var operations op.Ops
	gtx := layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))}
	renderer.Layout(gtx)
	renderer.dispatchFromLayout(gtx, uidsl.Action{
		Command: "select-timeline-item", Arguments: map[string]string{"id": "{{item.id}}"},
	}, mergeData(data, "item", map[string]any{"id": "step:1"}))
	selected, err := uidsl.Resolve(renderer.data, "jobDetails.selected_timeline_item.id")
	if err != nil || selected != "step:1" {
		t.Fatalf("selected timeline item = %v, err=%v", selected, err)
	}
	outputEditor := new(widget.Editor)
	outputEditor.ReadOnly = true
	outputEditor.SetText("compile one\ncompile two\n")
	renderer.outputEditors["step:1"] = outputEditor
	renderer.ApplyJobOutput(jobOutputSnapshot{Outputs: map[string]string{"step:1": "compile one\ncompile two\n"}, Errors: map[string]string{}, ExitCodes: map[string]string{}})
	renderer.dispatchFromLayout(gtx, uidsl.Action{
		Command: "change-output-search", Arguments: map[string]string{"query": "{{input.value}}"},
	}, mergeData(renderer.data, "input", map[string]any{"value": "compile"}))
	if start, end := outputEditor.Selection(); start != 0 || end != len("compile") {
		t.Fatalf("first output selection = %d:%d", start, end)
	}
	count, err := uidsl.Resolve(renderer.data, "jobDetails.output_search_count")
	if err != nil || count != "1/2" {
		t.Fatalf("output search count = %v, err=%v", count, err)
	}
}

func TestOutputMatchesUsesRuneOffsets(t *testing.T) {
	matches := outputMatches("héllo hello", "hello")
	if len(matches) != 1 || matches[0] != [2]int{6, 11} {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestNativeJobOutputBufferKeepsBoundedTail(t *testing.T) {
	buffer := &jobOutputBuffer{}
	buffer.reset("job-1")
	buffer.append(&cnpv1.JobOutputBatch{
		JobExecutionId: "job-1",
		Events:         []*cnpv1.JobOutputEvent{{Type: "output", ItemId: "step:1", Text: strings.Repeat("x", maxNativeOutputBytes+100)}},
	})
	if !buffer.omitted["step:1"] || len(buffer.events) != 1 || len(buffer.events[0].Text) > maxNativeOutputBytes {
		t.Fatalf("buffer events=%d omitted=%v text length=%d", len(buffer.events), buffer.omitted, len(buffer.events[0].Text))
	}
}

func TestNativeJobOutputBufferMaintainsIncrementalSnapshot(t *testing.T) {
	buffer := &jobOutputBuffer{}
	buffer.reset("job-1")
	first := strings.Repeat("a", 600*1024)
	second := strings.Repeat("b", 600*1024)
	buffer.append(&cnpv1.JobOutputBatch{JobExecutionId: "job-1", Events: []*cnpv1.JobOutputEvent{
		{Type: "output", ItemId: "step:1", Text: first},
		{Type: "output", ItemId: "step:1", Text: second},
		{Type: "finished", ItemId: "step:1", Error: "failed", ExitCode: "1"},
	}})
	if buffer.bytes != len(second) || buffer.snapshot.Outputs["step:1"] != second || !buffer.omitted["step:1"] {
		t.Fatalf("incremental output snapshot bytes=%d output=%d omitted=%v", buffer.bytes, len(buffer.snapshot.Outputs["step:1"]), buffer.omitted)
	}
	if buffer.snapshot.Errors["step:1"] != "failed" || buffer.snapshot.ExitCodes["step:1"] != "1" || !buffer.dirty {
		t.Fatalf("incremental finish snapshot = %#v", buffer.snapshot)
	}
}

func TestSemanticToneUsesSharedStatusCategories(t *testing.T) {
	tests := map[string]string{
		"succeeded": "success", "failed": "danger", "queued": "warning",
		"in progress": "accent", "warning": "warning", "accent": "accent",
		"muted": "muted", "unknown": "muted",
	}
	for status, want := range tests {
		if got := semanticTone(status); got != want {
			t.Errorf("semanticTone(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestRendererHonorsActionConfirmation(t *testing.T) {
	var dispatched bool
	renderer := &Renderer{onAction: func(_ uidsl.Action, arguments map[string]string) {
		dispatched = arguments["pipelineDbId"] == "7" && arguments["pipelineJobId"] == "compile" && arguments["dryRun"] == "true"
	}}
	renderer.dispatch(uidsl.Action{
		Command:   "run-pipeline",
		Arguments: map[string]string{"pipelineDbId": "{{pipeline.id}}", "pipelineJobId": "{{job.id}}", "dryRun": "true"},
		Confirm:   &uidsl.Confirmation{Title: "Run pipeline", Message: "Queue another execution."},
	}, map[string]any{"pipeline": map[string]any{"id": float64(7)}, "job": map[string]any{"id": "compile"}})
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

func TestModalOverlayPreservesBodyViewportConstraints(t *testing.T) {
	var operations op.Ops
	constraints := layout.Exact(image.Pt(1100, 760))
	gtx := layout.Context{Ops: &operations, Constraints: constraints}
	var bodyConstraints layout.Constraints

	dimensions := layoutModalOverlay(gtx, func(gtx layout.Context) layout.Dimensions {
		bodyConstraints = gtx.Constraints
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(400, 240)}
	})

	if bodyConstraints != constraints {
		t.Fatalf("body constraints = %#v, want %#v", bodyConstraints, constraints)
	}
	if dimensions.Size != constraints.Max {
		t.Fatalf("overlay dimensions = %v, want %v", dimensions.Size, constraints.Max)
	}
}

func TestRendererUpdatesRepeatedItemBindingWithoutMutatingInput(t *testing.T) {
	project := map[string]any{"id": float64(7), "action_status": ""}
	data := map[string]any{"settings": map[string]any{"projects": []any{project}}}
	renderer := &Renderer{data: data}

	if !renderer.SetRepeatedItemBinding("settings", "projects", "id", "7", "action_status", "Reloading…") {
		t.Fatal("project binding was not found")
	}
	if project["action_status"] != "" {
		t.Fatal("input data was mutated")
	}
	root := renderer.data.(map[string]any)["settings"].(map[string]any)
	updated := root["projects"].([]any)[0].(map[string]any)
	if updated["action_status"] != "Reloading…" {
		t.Fatalf("action_status = %q", updated["action_status"])
	}
	if renderer.SetRepeatedItemBinding("settings", "projects", "id", "8", "action_status", "unexpected") {
		t.Fatal("missing project was reported as updated")
	}
}

func TestPreserveSettingsUIStateAcrossRefresh(t *testing.T) {
	previous := map[string]any{"settings": map[string]any{
		"import_repo_url": "https://example.test/repo",
		"projects": []any{
			map[string]any{"id": float64(7), "action_status": "Reloaded successfully", "action_tone": "success"},
			map[string]any{"id": float64(8), "action_status": "", "action_tone": "muted"},
		},
	}}
	next := map[string]any{"settings": map[string]any{
		"import_repo_url": "",
		"projects": []any{
			map[string]any{"id": float64(7), "action_status": "", "action_tone": "muted"},
			map[string]any{"id": float64(8), "action_status": "", "action_tone": "muted"},
		},
	}}

	preserveSettingsUIState(previous, next)
	root := next["settings"].(map[string]any)
	if root["import_repo_url"] != "https://example.test/repo" {
		t.Fatalf("import_repo_url = %q", root["import_repo_url"])
	}
	projects := root["projects"].([]any)
	first := projects[0].(map[string]any)
	if first["action_status"] != "Reloaded successfully" || first["action_tone"] != "success" {
		t.Fatalf("preserved project state = %#v", first)
	}
	second := projects[1].(map[string]any)
	if second["action_status"] != "" {
		t.Fatalf("empty project state should not be overlaid: %#v", second)
	}
}

func TestPreserveSettingsUIStateRecognizesCompletedUpdate(t *testing.T) {
	previous := map[string]any{"settings": map[string]any{
		"selected_update_version": "v0.2.2", "selected_rollback_version": "",
		"update_result": "Waiting for restart…", "update_result_tone": "muted",
		"update_versions":   []any{map[string]any{"value": "v0.2.2", "label": "v0.2.2"}},
		"rollback_versions": []any{map[string]any{"value": "", "label": "Refresh versions"}},
	}}
	next := map[string]any{"settings": map[string]any{
		"update_current_version": "0.2.2", "selected_update_version": "", "selected_rollback_version": "",
		"update_result": "", "update_result_tone": "muted", "rollback_result": "", "rollback_result_tone": "muted",
		"update_versions": []any{}, "rollback_versions": []any{},
	}}

	preserveSettingsUIState(previous, next)
	root := next["settings"].(map[string]any)
	if root["update_result"] != "Update successful." || root["selected_update_version"] != "" {
		t.Fatalf("completed update state = %#v", root)
	}
}

func TestPipelineRunSelectionUsesSharedPipelineAndChainArguments(t *testing.T) {
	selection := pipelineRunSelection(map[string]string{
		"pipelineJobId": " compile ", "dryRun": "true", "sourceRef": " feature/native ",
		"agentId": " agent-1 ", "executionMode": " offline_cached ",
	})
	if selection.PipelineJobId != "compile" || !selection.DryRun || selection.SourceRef != "feature/native" || selection.AgentId != "agent-1" || selection.ExecutionMode != "offline_cached" {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestRendererLaysOutSharedRunOptions(t *testing.T) {
	screen, err := sharedUI.LoadScreen("run-options")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := protobufBindingData("runOptions", "run-options", &cnpv1.RunOptionsView{
		TargetKind: "pipeline", TargetLabel: "build", PipelineDbId: 42, ProjectId: 7,
		SupportsDryRun: true, SourceRepo: "https://github.com/izzyreal/ciwi", PendingJobs: 6,
		DefaultSourceRef: "refs/heads/main", SelectedSourceRef: "refs/heads/main",
		SourceRefs:     []*cnpv1.RunOption{{Value: "refs/heads/main", Label: "main"}},
		EligibleAgents: []*cnpv1.RunOption{{Value: "", Label: "Any eligible agent"}, {Value: "agent-1", Label: "agent-1"}},
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
	selectToggles := 0
	for path := range renderer.buttons {
		if strings.HasSuffix(path, "/select-toggle") {
			selectToggles++
		}
	}
	if selectToggles != 2 {
		t.Fatalf("select controls = %d, want source-ref and agent selectors", selectToggles)
	}
}

func TestSetAgentRunOptionUpdatesNavigationAndBinding(t *testing.T) {
	renderer := &Renderer{data: map[string]any{"runOptions": map[string]any{"selected_agent_id": ""}}}
	navigation := &navigationState{screen: "run-options", pipelineDBID: 42}
	refreshEligibility, err := applyRunOptionSelection(renderer, navigation, "agentId", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if refreshEligibility {
		t.Fatal("agent selection should not recompute eligibility")
	}
	if navigation.agentID != "agent-1" {
		t.Fatalf("selected agent = %q", navigation.agentID)
	}
	root := renderer.data.(map[string]any)["runOptions"].(map[string]any)
	if root["selected_agent_id"] != "agent-1" {
		t.Fatalf("binding selected_agent_id = %#v", root["selected_agent_id"])
	}
	refreshEligibility, err = applyRunOptionSelection(renderer, navigation, "sourceRef", "refs/heads/main")
	if err != nil || !refreshEligibility {
		t.Fatalf("source selection refresh=%v error=%v", refreshEligibility, err)
	}
}

func TestPipelineRunOptionsRoutePreservesProjectForImmediateBack(t *testing.T) {
	navigation, err := navigationForRoute("/run-options/projects/7/pipelines/42")
	if err != nil {
		t.Fatal(err)
	}
	if navigation.screen != "run-options" || navigation.projectID != 7 || navigation.pipelineDBID != 42 {
		t.Fatalf("navigation = %+v", navigation)
	}
	root := runOptionsLoadingData(navigation)["runOptions"].(map[string]any)
	if root["project_id"] != int64(7) || root["target_kind"] != "loading" {
		t.Fatalf("loading data = %+v", root)
	}
}

func TestAgentScriptRouteAndLoadingDataAreImmediate(t *testing.T) {
	navigation, err := navigationForRoute("/agents/agent-1/script")
	if err != nil {
		t.Fatal(err)
	}
	if navigation.screen != "agent-script" || navigation.agentScriptID != "agent-1" {
		t.Fatalf("navigation = %+v", navigation)
	}
	data, err := screenLoadingData(navigation, "v0.2.9", "default", connectionModeDiscover, "", sshConnectionSettings{})
	if err != nil {
		t.Fatal(err)
	}
	root := data["agentScript"].(map[string]any)
	if root["agent_id"] != "agent-1" || root["can_run"] != false {
		t.Fatalf("loading data = %+v", root)
	}
}

func TestProjectDetailsLoadingDataSeedsAnImmediateLocalShell(t *testing.T) {
	navigation := navigationState{screen: "project-details", projectID: 41}
	data, err := screenLoadingData(navigation, "v0.2.9", "default", connectionModeDiscover, "", sshConnectionSettings{})
	if err != nil {
		t.Fatal(err)
	}
	root := data["projectDetails"].(map[string]any)
	project := root["project"].(map[string]any)
	if root["loading"] != true || root["ready"] != false || root["load_error"] != "" {
		t.Fatalf("project loading state = %#v", root)
	}
	if root["show_chain_structure"] != false || root["show_pipeline_structure"] != false || project["name"] != "Project" {
		t.Fatalf("project loading shell = root %#v project %#v", root, project)
	}
	frontPage := map[string]any{"frontPage": map[string]any{"projects": []any{map[string]any{
		"id": float64(41), "name": "ciwi", "project_icon": []byte("icon"), "project_icon_content_type": "image/png",
		"repo_url": "https://example.invalid/ciwi", "repo_ref": "main", "config_file": "ciwi-project.yaml",
	}}}}
	seedProjectDetailsLoadingData(data, frontPage, 41)
	if project["name"] != "ciwi" || string(project["project_icon"].([]byte)) != "icon" || project["repo_ref"] != "main" {
		t.Fatalf("seeded project shell = %#v", project)
	}
	screen, err := sharedUI.LoadScreen("project-details")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNativeBindings(screen, data); err != nil {
		t.Fatalf("project loading shell bindings: %v", err)
	}
}

func TestJobDetailsLoadingDataProvidesNestedRequirementSchemas(t *testing.T) {
	navigation := navigationState{screen: "job-details", jobID: "job-1"}
	data, err := screenLoadingData(navigation, "v0.2.9", "default", connectionModeDiscover, "", sshConnectionSettings{})
	if err != nil {
		t.Fatal(err)
	}
	root := data["jobDetails"].(map[string]any)
	for _, key := range []string{"host_tool_requirements", "container_tool_requirements"} {
		requirements, ok := root[key].(map[string]any)
		if !ok {
			t.Fatalf("%s loading schema = %#v", key, root[key])
		}
		for _, field := range []string{"empty_label", "summary", "tone", "issues"} {
			if _, exists := requirements[field]; !exists {
				t.Errorf("%s loading schema is missing %s: %#v", key, field, requirements)
			}
		}
	}
	runContext, ok := root["run_context"].(map[string]any)
	if !ok || runContext["available"] != false {
		t.Fatalf("run_context loading schema = %#v", root["run_context"])
	}

	screen, err := sharedUI.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	renderer.Layout(layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(390, 844))})
	for path := range renderer.selectables {
		if strings.HasPrefix(path, "error/") {
			t.Fatalf("loading job details rendered binding error %q", path)
		}
	}
}

func TestReadScreensRenderStructuredLoadingStateWithoutBindingErrors(t *testing.T) {
	tests := []struct {
		screen     string
		navigation navigationState
	}{
		{screen: "front-page", navigation: navigationState{screen: "front-page"}},
		{screen: "project-details", navigation: navigationState{screen: "project-details", projectID: 1}},
		{screen: "job-details", navigation: navigationState{screen: "job-details", jobID: "job-1"}},
		{screen: "settings", navigation: navigationState{screen: "settings"}},
		{screen: "agents", navigation: navigationState{screen: "agents"}},
		{screen: "agent-details", navigation: navigationState{screen: "agent-details", agentDetailsID: "agent-1"}},
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.screen, func(t *testing.T) {
			if _, readScreen := nativeScreenCacheKeyFor(test.navigation); !readScreen {
				t.Fatalf("%+v is not classified as a read screen", test.navigation)
			}
			screen, loadErr := sharedUI.LoadScreen(test.screen)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			renderer, rendererErr := NewRenderer(screen, theme, nil)
			if rendererErr != nil {
				t.Fatal(rendererErr)
			}
			data, dataErr := screenLoadingData(test.navigation, "v0.2.9", "default", connectionModeDiscover, "", sshConnectionSettings{})
			if dataErr != nil {
				t.Fatal(dataErr)
			}
			data["client"] = nativeConnectionState{connecting: true, status: "Trying to connect…"}.binding()
			renderer.SetScreenAndData(screen, data)
			renderer.Layout(layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(390, 844))})
			if renderer.data == nil {
				t.Fatal("structured loading data was discarded")
			}
			for path := range renderer.selectables {
				if strings.HasPrefix(path, "error/") {
					t.Fatalf("structured loading state rendered binding error %q", path)
				}
			}
		})
	}
}

func TestSetAgentScriptFieldUpdatesNavigationAndBindings(t *testing.T) {
	renderer := &Renderer{data: map[string]any{"agentScript": map[string]any{
		"selected_shell": "", "script": "",
	}}}
	navigation := &navigationState{screen: "agent-script", agentScriptID: "agent-1"}
	handleCommand(renderer, navigation, commandRequest{
		action:    uidsl.Action{Command: "set-agent-script-field"},
		arguments: map[string]string{"field": "shell", "value": "powershell"},
	}, "")
	if navigation.scriptShell != "powershell" || !strings.Contains(navigation.script, "$ErrorActionPreference") {
		t.Fatalf("navigation = %+v", navigation)
	}
	root := renderer.data.(map[string]any)["agentScript"].(map[string]any)
	if root["selected_shell"] != "powershell" || root["script"] != navigation.script {
		t.Fatalf("bindings = %+v", root)
	}
}

func TestEveryConnectedScreenHasImmediateLoadingData(t *testing.T) {
	tests := []navigationState{
		{screen: "front-page"}, {screen: "project-details", projectID: 1},
		{screen: "job-details", jobID: "job-1"}, {screen: "settings"},
		{screen: "run-options", pipelineDBID: 1}, {screen: "agents"},
		{screen: "agent-details", agentDetailsID: "agent-1"},
		{screen: "agent-script", agentScriptID: "agent-1"},
	}
	for _, navigation := range tests {
		t.Run(navigation.screen, func(t *testing.T) {
			if _, err := screenLoadingData(navigation, "v0.2.9", "default", connectionModeDiscover, "", sshConnectionSettings{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInteractiveControlsDoNotReceiveGenericActionWrapper(t *testing.T) {
	for _, component := range []string{"button", "select", "input"} {
		if !componentHandlesOwnActions(component) {
			t.Errorf("%s should dispatch its own actions", component)
		}
	}
	for _, component := range []string{"card", "row", "text"} {
		if componentHandlesOwnActions(component) {
			t.Errorf("%s should retain generic action wrapping", component)
		}
	}
}

func TestActionableNoticeExpires(t *testing.T) {
	renderer := &Renderer{}
	renderer.ShowNotice("Script queued", "Show job execution", uidsl.Action{Command: "navigate"}, map[string]string{"route": "/jobs/job-1"}, time.Hour)
	noticeExpiry := renderer.NoticeExpiry()
	if noticeExpiry.IsZero() || renderer.notice == nil || renderer.notice.arguments["route"] != "/jobs/job-1" {
		t.Fatalf("notice state = %#v, expiry=%v", renderer.notice, noticeExpiry)
	}
	if !renderer.ClearExpiredNotice(noticeExpiry) || renderer.notice != nil {
		t.Fatalf("notice did not expire: %#v", renderer.notice)
	}
	if !renderer.NoticeExpiry().IsZero() {
		t.Fatalf("expired notice retained expiry %v", renderer.NoticeExpiry())
	}
}

func TestNativeQueuedJobsNoticeNavigatesAndScrollsAfterTouch(t *testing.T) {
	screen, err := sharedUI.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("space")
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan commandRequest, 1)
	renderer, err := NewRenderer(screen, theme, func(action uidsl.Action, arguments map[string]string) {
		commands <- commandRequest{action: action, arguments: arguments}
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := offlineFrontPageBindingData()
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	renderer.ShowNotice("Pipeline queued", "Show queued jobs", uidsl.Action{Command: "navigate"}, map[string]string{"route": "/", "section": "queued-executions"}, time.Hour)
	var router input.Router
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(390, 600))}
	renderer.Layout(gtx)
	router.Frame(gtx.Ops)
	actionBounds, ok := semanticClickBoundsForLabel(router.AppendSemantics(nil), "Show queued jobs")
	if !ok {
		t.Fatalf("notice action is missing from semantics: %#v", router.AppendSemantics(nil))
	}
	center := actionBounds.Min.Add(actionBounds.Size().Div(2))
	router.Queue(
		pointer.Event{Source: pointer.Touch, Kind: pointer.Press, Position: f32.Pt(float32(center.X), float32(center.Y))},
		pointer.Event{Source: pointer.Touch, Kind: pointer.Release, Position: f32.Pt(float32(center.X), float32(center.Y))},
	)
	gtx.Reset()
	renderer.Layout(gtx)
	router.Frame(gtx.Ops)
	var command commandRequest
	select {
	case command = <-commands:
	default:
		t.Fatal("notice action was not dispatched")
	}
	if command.action.Command != "navigate" || command.arguments["route"] != "/" || command.arguments["section"] != "queued-executions" {
		t.Fatalf("notice action = %#v", command)
	}
	if renderer.notice != nil {
		t.Fatalf("notice was not dismissed: %#v", renderer.notice)
	}
	navigation, err := navigationForRoute(command.arguments["route"])
	if err != nil {
		t.Fatal(err)
	}
	if navigation.screen != "front-page" {
		t.Fatalf("navigation = %+v", navigation)
	}
	renderer.ScrollToSection(command.arguments["section"])
	renderer.SetScreenAndData(screen, data)
	gtx.Reset()
	renderer.Layout(gtx)
	router.Frame(gtx.Ops)
	if _, redraw := router.WakeupTime(); !redraw {
		t.Fatal("queued section scroll did not request its settling frame")
	}
	gtx.Reset()
	renderer.Layout(gtx)
	router.Frame(gtx.Ops)
	if renderer.pendingScrollSection != "" || renderer.list.Position.Offset <= 0 {
		t.Fatalf("queued section target pending=%q position=%+v", renderer.pendingScrollSection, renderer.list.Position)
	}
}

func semanticClickBoundsForLabel(nodes []input.SemanticNode, label string) (image.Rectangle, bool) {
	for _, node := range nodes {
		if node.Desc.Gestures&input.ClickGesture != 0 && semanticTreeHasLabel(node.Children, label) {
			return node.Desc.Bounds, true
		}
		if bounds, ok := semanticClickBoundsForLabel(node.Children, label); ok {
			return bounds, true
		}
	}
	return image.Rectangle{}, false
}

func semanticTreeHasLabel(nodes []input.SemanticNode, label string) bool {
	for _, node := range nodes {
		if node.Desc.Label == label || semanticTreeHasLabel(node.Children, label) {
			return true
		}
	}
	return false
}

func TestNativeNoticesQueueDeduplicateAndAdvance(t *testing.T) {
	renderer := &Renderer{}
	renderer.ShowNotice("First", "", uidsl.Action{}, nil, time.Hour)
	renderer.ShowNotice("Second", "Show", uidsl.Action{Command: "navigate"}, map[string]string{"route": "/jobs/2"}, 2*time.Hour)
	renderer.ShowNotice("Second", "Show", uidsl.Action{Command: "navigate"}, map[string]string{"route": "/jobs/2"}, 2*time.Hour)
	if renderer.notice == nil || renderer.notice.message != "First" || len(renderer.noticeQueue) != 1 {
		t.Fatalf("notice state = %#v, queue=%#v", renderer.notice, renderer.noticeQueue)
	}
	firstExpiry := renderer.notice.expires
	if !renderer.ClearExpiredNotice(firstExpiry) {
		t.Fatal("first notice did not expire")
	}
	if renderer.notice == nil || renderer.notice.message != "Second" || len(renderer.noticeQueue) != 0 {
		t.Fatalf("advanced notice state = %#v, queue=%#v", renderer.notice, renderer.noticeQueue)
	}
	if want := firstExpiry.Add(2 * time.Hour); !renderer.notice.expires.Equal(want) {
		t.Fatalf("second expiry = %v, want %v", renderer.notice.expires, want)
	}
}

func TestNativeNoticeQueueKeepsLatestFour(t *testing.T) {
	renderer := &Renderer{}
	for _, message := range []string{"One", "Two", "Three", "Four", "Five", "Six"} {
		renderer.ShowNotice(message, "", uidsl.Action{}, nil, time.Hour)
	}
	if renderer.notice == nil || renderer.notice.message != "One" {
		t.Fatalf("active notice = %#v, want One", renderer.notice)
	}
	if len(renderer.noticeQueue) != 3 {
		t.Fatalf("queued notice count = %d, want 3", len(renderer.noticeQueue))
	}
	got := []string{renderer.noticeQueue[0].message, renderer.noticeQueue[1].message, renderer.noticeQueue[2].message}
	if strings.Join(got, ",") != "Four,Five,Six" {
		t.Fatalf("queued notices = %q, want latest waiting notices", got)
	}
	renderer.dismissNotice()
	if renderer.notice == nil || renderer.notice.message != "Four" {
		t.Fatalf("notice after dismiss = %#v, want Four", renderer.notice)
	}
}

func TestNativeNoticePausesAndResumesExpiry(t *testing.T) {
	renderer := &Renderer{}
	renderer.ShowNotice("Queued", "Show", uidsl.Action{Command: "navigate"}, map[string]string{"route": "/"}, time.Hour)
	started := renderer.notice.expires.Add(-time.Hour)
	renderer.setNoticePaused(true, started.Add(10*time.Minute))
	if !renderer.notice.paused || !renderer.notice.expires.IsZero() || renderer.notice.remaining != 50*time.Minute {
		t.Fatalf("paused notice = %#v", renderer.notice)
	}
	resumedAt := started.Add(20 * time.Minute)
	renderer.setNoticePaused(false, resumedAt)
	if renderer.notice.paused || !renderer.notice.expires.Equal(resumedAt.Add(50*time.Minute)) {
		t.Fatalf("resumed notice = %#v", renderer.notice)
	}
}

func TestNativeAlertUsesDedicatedModalState(t *testing.T) {
	renderer := &Renderer{}
	renderer.ShowNotice("Background notice", "", uidsl.Action{}, nil, time.Hour)
	renderer.ShowAlert("Action failed", "server rejected request")
	if renderer.alert == nil || renderer.alert.title != "Action failed" || renderer.alert.message != "server rejected request" {
		t.Fatalf("alert state = %#v", renderer.alert)
	}
	if renderer.notice == nil || renderer.notice.message != "Background notice" {
		t.Fatalf("showing alert changed notice state to %#v", renderer.notice)
	}
}

func TestOfflineFrontPageDoesNotInventServerVersion(t *testing.T) {
	data, err := offlineFrontPageBindingData()
	if err != nil {
		t.Fatal(err)
	}
	frontPage := data["frontPage"].(map[string]any)
	server := frontPage["server"].(map[string]any)
	if got := fmt.Sprint(server["version"]); got != "Unavailable" {
		t.Fatalf("offline server version = %q, want Unavailable", got)
	}
}

func TestNativeNoticeCanTargetFrontPageSection(t *testing.T) {
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
	data, err := offlineFrontPageBindingData()
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	renderer.ScrollToSection("queued-executions")
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(390, 300))}
	renderer.Layout(gtx)
	renderer.Layout(layout.Context{Ops: new(op.Ops), Constraints: gtx.Constraints})
	if renderer.pendingScrollSection != "" {
		t.Fatalf("pending section target was not consumed: %q", renderer.pendingScrollSection)
	}
	visible := renderer.visibleRootChildIndices(screen.Screen.Root.Children, renderer.data)
	if renderer.list.Position.First >= len(visible) || screen.Screen.Root.Children[visible[renderer.list.Position.First]].ID != "queued-executions" {
		t.Fatalf("front-page list first item = %d (%v), want queued section", renderer.list.Position.First, visible)
	}
}

func TestEvaluateSemanticProgressAdvancesFromServerSnapshot(t *testing.T) {
	snapshot := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	state, fraction := evaluateSemanticProgress(semanticProgress{
		state: "determinate", fraction: .2, snapshotUnixMS: snapshot.UnixMilli(), ratePerMS: .0001,
	}, snapshot.Add(3*time.Second))
	if state != "determinate" || fraction < .4999 || fraction > .5001 {
		t.Fatalf("state=%q fraction=%g", state, fraction)
	}
	state, fraction = evaluateSemanticProgress(semanticProgress{
		state: "determinate", fraction: .9, snapshotUnixMS: snapshot.UnixMilli(), ratePerMS: .0001,
	}, snapshot.Add(3*time.Second))
	if state != "determinate" || fraction != determinateProgressLimit {
		t.Fatalf("state=%q fraction=%g", state, fraction)
	}
	state, fraction = evaluateSemanticProgress(semanticProgress{state: "overrun", fraction: 1}, snapshot)
	if state != "overrun" || fraction != 1 {
		t.Fatalf("explicit server overrun state=%q fraction=%g", state, fraction)
	}
}

func TestIndeterminateProgressPositionEasesAcrossRoundTrip(t *testing.T) {
	start := time.Unix(0, 0)
	tests := []struct {
		offset time.Duration
		want   float64
	}{
		{offset: 0, want: 0},
		{offset: indeterminateProgressDuration / 4, want: .5},
		{offset: indeterminateProgressDuration / 2, want: 1},
		{offset: 3 * indeterminateProgressDuration / 4, want: .5},
		{offset: indeterminateProgressDuration, want: 0},
	}
	for _, test := range tests {
		got := indeterminateProgressPosition(start.Add(test.offset))
		if got < test.want-.000001 || got > test.want+.000001 {
			t.Errorf("position at %v = %g, want %g", test.offset, got, test.want)
		}
	}
}

func TestConnectionPulseOpacityEasesSlowlyWithoutDisappearing(t *testing.T) {
	start := time.Unix(0, 0)
	minimum := connectionPulseOpacity(start)
	middle := connectionPulseOpacity(start.Add(connectionPulseDuration / 2))
	end := connectionPulseOpacity(start.Add(connectionPulseDuration))
	if minimum < connectionPulseMinimum-.000001 || minimum > connectionPulseMinimum+.000001 {
		t.Fatalf("pulse minimum = %g, want %g", minimum, connectionPulseMinimum)
	}
	if middle < .999999 || middle > 1.000001 {
		t.Fatalf("pulse maximum = %g, want 1", middle)
	}
	if end < minimum-.000001 || end > minimum+.000001 {
		t.Fatalf("pulse end = %g, want cycle minimum %g", end, minimum)
	}
}

func TestProgressTrackUsesAvailableWidth(t *testing.T) {
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Constraints{Min: image.Pt(120, 24), Max: image.Pt(640, 100)}}
	renderer := &Renderer{palette: palette{success: color.NRGBA{G: 255, A: 255}}}
	dimensions := renderer.progressWidget(
		uidsl.Node{Progress: &uidsl.Progress{Binding: "item.progress"}},
		map[string]any{"item": map[string]any{"progress": map[string]any{"state": "complete"}}},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(120, 24))}
		},
	)(gtx)
	if dimensions.Size.X != 640 {
		t.Fatalf("progress width = %d, want available width 640", dimensions.Size.X)
	}
	if dimensions.Size.Y != 24 {
		t.Fatalf("progress height = %d, want content height 24", dimensions.Size.Y)
	}
}

func TestExecutionSectionHeaderProgressUsesFullPaddedHeader(t *testing.T) {
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
	var header *uidsl.Node
	var findHeader func(uidsl.Node)
	findHeader = func(node uidsl.Node) {
		if node.Style.Role == "execution-section-header" {
			copy := node
			header = &copy
			return
		}
		for _, child := range node.Children {
			findHeader(child)
		}
		if node.Disclosure != nil {
			for _, child := range node.Disclosure.Summary {
				findHeader(child)
			}
		}
	}
	findHeader(screen.Screen.Root)
	if header == nil {
		t.Fatal("execution section header is missing")
	}
	data := map[string]any{"section": map[string]any{
		"label":    "build",
		"progress": map[string]any{"state": "determinate", "fraction": .25},
	}}
	for _, width := range []int{390, 844} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Constraints{Max: image.Pt(width, 100)}}
			dimensions := renderer.layoutNode(gtx, *header, data, "section-header")
			if dimensions.Size.X != width {
				t.Fatalf("section header width = %d, want %d", dimensions.Size.X, width)
			}
			minimumHeight := 2*gtx.Dp(renderer.metrics.spaceSmall) + gtx.Sp(12)
			if dimensions.Size.Y < minimumHeight {
				t.Fatalf("section header height = %d, want at least padded label height %d", dimensions.Size.Y, minimumHeight)
			}
		})
	}
}

func TestExecutionSectionHeaderProgressRetainsSemanticFill(t *testing.T) {
	size := image.Pt(400, 32)
	now := time.Unix(100, 0)
	tests := []struct {
		name      string
		progress  semanticProgress
		wantWidth int
	}{
		{name: "determinate", progress: semanticProgress{state: "determinate", fraction: .25}, wantWidth: 100},
		{name: "indeterminate", progress: semanticProgress{state: "indeterminate"}, wantWidth: 88},
		{name: "overrun", progress: semanticProgress{state: "overrun", fraction: 1}, wantWidth: 400},
		{name: "complete", progress: semanticProgress{state: "complete", fraction: 1}, wantWidth: 400},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rect, _, _, ok := semanticProgressPaint(test.progress, size, now)
			if !ok {
				t.Fatal("semantic progress did not produce a fill")
			}
			if rect.Dx() != test.wantWidth || rect.Dy() != size.Y {
				t.Fatalf("progress fill = %v, want width %d and full height %d", rect, test.wantWidth, size.Y)
			}
		})
	}
}

func TestDisclosureProgressAlwaysUsesStableHeaderLayer(t *testing.T) {
	node := uidsl.Node{Component: "disclosure", Style: uidsl.Style{Role: "output-group"}}
	if usesSurfaceProgress(node, false) {
		t.Fatal("collapsed output-group progress must stay on the stable header layer")
	}
	if usesSurfaceProgress(node, true) {
		t.Fatal("expanded output-group progress must stay on its header")
	}
}

func TestCollapsingOutputGroupClearsStaleListPosition(t *testing.T) {
	list := &layout.List{Axis: layout.Vertical, ScrollToEnd: true}
	list.Position = layout.Position{BeforeEnd: true, First: 2, Offset: -480, OffsetLast: 120, Count: 2, Length: 1400}
	renderer := &Renderer{
		disclosures:    map[string]bool{"job-output:job-1:step-2": true},
		outputScroller: list,
	}

	renderer.setDisclosureState("job-output:job-1:step-2", false, false)

	if list.ScrollToEnd || list.Position != (layout.Position{}) {
		t.Fatalf("collapsed output list retained ScrollToEnd=%v position=%+v", list.ScrollToEnd, list.Position)
	}
}

func TestHeartbeatPulseMatchesBrowserFade(t *testing.T) {
	now := time.Unix(100, 0)
	if got := heartbeatPulseOpacity(now.UnixMilli(), now); got != 1 {
		t.Fatalf("heartbeat start opacity = %g, want 1", got)
	}
	middle := heartbeatPulseOpacity(now.UnixMilli(), now.Add(heartbeatPulseDuration/2))
	if middle < .589 || middle > .591 {
		t.Fatalf("heartbeat midpoint opacity = %g, want .59", middle)
	}
	if got := heartbeatPulseOpacity(now.UnixMilli(), now.Add(heartbeatPulseDuration)); got != heartbeatPulseMinimum {
		t.Fatalf("heartbeat end opacity = %g, want %g", got, heartbeatPulseMinimum)
	}
	if got := heartbeatPulseOpacity(0, now); got != heartbeatPulseMinimum {
		t.Fatalf("missing heartbeat opacity = %g, want %g", got, heartbeatPulseMinimum)
	}
}

func TestHeartbeatUnixMillisAcceptsProtobufJSONIntegers(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
	}{
		{name: "protobuf string", value: "1786000000123", want: 1786000000123},
		{name: "floating point JSON", value: float64(1786000000123), want: 1786000000123},
		{name: "native integer", value: int64(1786000000123), want: 1786000000123},
		{name: "zero", value: "0", want: 0},
		{name: "malformed", value: "not-a-timestamp", want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := heartbeatUnixMillis(test.value); got != test.want {
				t.Fatalf("heartbeat timestamp = %d, want %d", got, test.want)
			}
		})
	}
}

func TestOutputGroupBodyDefaultsToSharedConsoleText(t *testing.T) {
	children := []uidsl.Node{
		{Component: "text"},
		{Component: "text", Style: uidsl.Style{Tone: "danger"}},
		{Component: "row", Children: []uidsl.Node{{Component: "text"}}},
	}
	got := withDefaultConsoleText(children)
	if got[0].Style.Tone != "console-text" || got[1].Style.Tone != "danger" || got[2].Children[0].Style.Tone != "console-text" {
		t.Fatalf("console defaults = %#v", got)
	}
	if children[0].Style.Tone != "" || children[2].Children[0].Style.Tone != "" {
		t.Fatal("console defaults mutated the shared screen document")
	}
}

func TestProgressColorMatchesBrowserSRGBComposition(t *testing.T) {
	background := color.NRGBA{R: 0x11, G: 0x19, B: 0x36, A: 0xff}
	foreground := color.NRGBA{R: 0x72, G: 0xe6, B: 0xbc, A: 0xff}
	want := color.NRGBA{R: 0x22, G: 0x3e, B: 0x4e, A: 0xff}
	if got := mixColorSRGB(background, foreground, .18); got != want {
		t.Fatalf("native progress composition = %v, want browser sRGB result %v", got, want)
	}
}

func TestSurfaceProgressTextureMatchesBrowserSRGBComposition(t *testing.T) {
	colors := palette{
		surface:     color.NRGBA{R: 0x11, G: 0x19, B: 0x36, A: 0xff},
		subtle:      color.NRGBA{R: 0x14, G: 0x1d, B: 0x39, A: 0xff},
		surfaceGlow: color.NRGBA{R: 0x24, G: 0x20, B: 0x52, A: 0xff},
		success:     color.NRGBA{R: 0x52, G: 0xe2, B: 0xa2, A: 0xff},
	}
	size := image.Pt(24, 12)
	base := renderSurfaceBackground(size, colors)
	progress := renderSurfaceProgressBackground(size, colors, .18)
	point := image.Pt(9, 5)
	want := mixColorSRGB(base.NRGBAAt(point.X, point.Y), colors.success, .18)
	if got := progress.NRGBAAt(point.X, point.Y); got != want {
		t.Fatalf("surface progress pixel = %v, want browser sRGB composition %v", got, want)
	}
}

func TestOutputTailingUsesCompactStatefulIconToggle(t *testing.T) {
	if tablerIcons()["arrow-bar-to-down"] == nil {
		t.Fatal("arrow-bar-to-down icon is unavailable")
	}
	renderer := &Renderer{data: map[string]any{"jobDetails": map[string]any{
		"tailing_label": "Tailing: Off", "tailing_tone": "warning",
	}}}
	renderer.dispatchFromLayout(layout.Context{Ops: new(op.Ops)}, uidsl.Action{Command: "toggle-output-tailing"}, nil)
	label, labelErr := uidsl.Resolve(renderer.data, "jobDetails.tailing_label")
	tone, toneErr := uidsl.Resolve(renderer.data, "jobDetails.tailing_tone")
	if labelErr != nil || toneErr != nil || label != "Tailing: On" || tone != "success" {
		t.Fatalf("tailing state = label %v (%v), tone %v (%v)", label, labelErr, tone, toneErr)
	}
}

func TestCompactJobDetailsLetsPageScrollThroughExpandedOutput(t *testing.T) {
	screen, err := sharedUI.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", Title: "Job: compile", StatusLabel: "Running", Mode: "Run",
		OutputGroups: []*cnpv1.JobOutputGroup{
			{Id: "step:1", StateKey: "job-output:job-1:step:1", Kind: "step", Title: "Job step 1/2: Compile", Reached: true},
			{Id: "step:2", StateKey: "job-output:job-1:step:2", Kind: "step", Title: "Job step 2/2: Package", Reached: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	renderer.SetDisclosureStates(map[string]bool{"job-output:job-1:step:1": true})
	renderer.ApplyJobOutput(jobOutputSnapshot{Outputs: map[string]string{
		"step:1": strings.Repeat("compiler output line\n", 120),
		"step:2": "package complete\n",
	}})
	// Start at the Output / Error section so this test exercises the compact
	// rendering even when the preceding execution summary exceeds a phone.
	renderer.list.ScrollTo(2)
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(390, 844))})

	if renderer.outputScroller != nil {
		t.Fatal("compact output retained a nested vertical scroller")
	}
	if renderer.outputEditors["step:1"] == nil {
		t.Fatal("expanded step output was not laid out")
	}
	if renderer.list.Position.Length <= 844 {
		t.Fatalf("page scroll length = %d, want expanded output in the phone page", renderer.list.Position.Length)
	}
	outputGroups, found := screenNodeByID(screen.Screen.Root, "job-output-groups")
	if !found {
		t.Fatal("job output groups node is missing")
	}
	dimensions := renderer.layoutNode(layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(390, 10_000)},
	}, outputGroups, renderer.data, "test/compact-job-output-groups")
	if dimensions.Size.Y <= 660 {
		t.Fatalf("expanded compact output height = %d, want content taller than the desktop viewport bound", dimensions.Size.Y)
	}
}

func TestCollapsedDesktopOutputGroupsUseBoundedScroller(t *testing.T) {
	screen, err := sharedUI.LoadScreen("job-details")
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
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", Title: "Job: compile", StatusLabel: "Failed", Mode: "Run",
		OutputGroups: []*cnpv1.JobOutputGroup{
			{Id: "phase:1", StateKey: "job-output:job-1:phase:1", Kind: "phase", Title: "Ciwi phase 1/2: Prepare workspace", Reached: true},
			{Id: "phase:2", StateKey: "job-output:job-1:phase:2", Kind: "phase", Title: "Ciwi phase 2/2: Check out source", Reached: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	renderer.list.ScrollTo(2)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1400, 900))}
	renderer.Layout(gtx)
	if renderer.outputScroller == nil {
		t.Fatal("collapsed desktop output groups did not install their bounded scroller")
	}
	outputGroups, found := screenNodeByID(screen.Screen.Root, "job-output-groups")
	if !found {
		t.Fatal("job output groups node is missing")
	}
	dimensions := renderer.layoutNode(layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1000, 10_000)},
	}, outputGroups, data, "test/job-output-groups")
	if dimensions.Size.Y <= 0 || dimensions.Size.Y > 660 {
		t.Fatalf("collapsed output viewport height = %d, want content-sized up to 660dp", dimensions.Size.Y)
	}
	renderer.SetDisclosureStates(map[string]bool{"job-output:job-1:phase:1": true})
	renderer.Layout(gtx)
	if renderer.outputScroller == nil {
		t.Fatal("expanded desktop output group lost its bounded scroller")
	}
}

func screenNodeByID(node uidsl.Node, id string) (uidsl.Node, bool) {
	if node.ID == id {
		return node, true
	}
	for _, child := range node.Children {
		if found, ok := screenNodeByID(child, id); ok {
			return found, true
		}
	}
	return uidsl.Node{}, false
}

func TestRootPageInsetsScrollWithFirstAndLastContent(t *testing.T) {
	renderer := &Renderer{
		list:    layout.List{Axis: layout.Vertical},
		metrics: visualMetrics{pageInset: 16, spaceMedium: 12},
	}
	screen := &uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "inset-test"}}
	root := uidsl.Node{Layout: uidsl.Layout{Gap: "medium"}}
	children := []uidsl.Node{
		{Component: "spacer", Layout: uidsl.Layout{MinHeight: "100"}},
		{Component: "spacer", Layout: uidsl.Layout{MinHeight: "100"}},
	}
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(800, 120))}
	dimensions := renderer.layoutRootChildren(children, root, screen, map[string]any{})(gtx)
	if dimensions.Size != image.Pt(800, 120) {
		t.Fatalf("viewport dimensions = %v", dimensions.Size)
	}
	if renderer.list.Position.Length != 244 {
		t.Fatalf("scroll content length = %d, want top 16 + children 200 + gap 12 + bottom 16 = 244", renderer.list.Position.Length)
	}
}

func TestInvisibleRootChildrenDoNotAddPageGaps(t *testing.T) {
	renderer := &Renderer{
		list:    layout.List{Axis: layout.Vertical},
		metrics: visualMetrics{pageInset: 16, spaceMedium: 12},
	}
	screen := &uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "root-gap-test"}}
	root := uidsl.Node{Layout: uidsl.Layout{Gap: "medium"}}
	children := []uidsl.Node{
		{Component: "spacer", Layout: uidsl.Layout{MinHeight: "100"}},
		{Component: "spacer", Visible: &uidsl.Condition{Binding: "state.loading"}, Layout: uidsl.Layout{MinHeight: "100"}},
		{Component: "spacer", Visible: &uidsl.Condition{Binding: "state.error"}, Layout: uidsl.Layout{MinHeight: "100"}},
		{Component: "spacer", Layout: uidsl.Layout{MinHeight: "100"}},
	}
	data := map[string]any{"state": map[string]any{"loading": false, "error": false}}
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(800, 120))}
	renderer.layoutRootChildren(children, root, screen, data)(gtx)
	if renderer.list.Position.Length != 244 {
		t.Fatalf("root length with hidden lifecycle nodes = %d, want two visible children, one gap, and page insets", renderer.list.Position.Length)
	}
}

func TestCompactPageInsetUsesPhoneSizedGutter(t *testing.T) {
	renderer := &Renderer{metrics: visualMetrics{pageInset: 16}, compact: true}
	if got := renderer.pageInset(); got != 3.2 {
		t.Fatalf("phone page inset = %v, want 3.2", got)
	}
	renderer.compact = false
	if got := renderer.pageInset(); got != 16 {
		t.Fatalf("tablet page inset = %v, want 16", got)
	}
}

func TestCompactViewportKeepsIPadDesktopLayoutAndCatchesIPhoneLandscape(t *testing.T) {
	for _, test := range []struct {
		platform      string
		width, height float32
		want          bool
	}{
		{platform: "ios", width: 390, height: 844, want: true},
		{platform: "ios", width: 844, height: 390, want: true},
		{platform: "ios", width: 834, height: 1112, want: false},
		{platform: "darwin", width: 500, height: 900, want: true},
		{platform: "darwin", width: 900, height: 500, want: false},
	} {
		if got := compactViewport(test.platform, test.width, test.height); got != test.want {
			t.Errorf("compactViewport(%q, %v, %v) = %v, want %v", test.platform, test.width, test.height, got, test.want)
		}
	}
}

func TestIPhoneLandscapeCompactModeSurvivesPageListConstraints(t *testing.T) {
	screen, err := sharedUI.LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{
		Id: "job-1", Title: "VMPC2000XL / build / linux-offline-build", StatusLabel: "Running", Mode: "Run",
		CanCancel: true, CanRerun: true,
		OutputGroups: []*cnpv1.JobOutputGroup{{
			Id: "step:1", StateKey: "job-output:job-1:step:1", Kind: "step", Title: "Job step 1/1: Build", Reached: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	// Jump to the output section so a nested compact decision is exercised.
	// Gio measures vertical list items with an effectively unbounded height.
	renderer.list.ScrollTo(2)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
		Constraints: layout.Exact(image.Pt(1334, 750)),
	}
	renderer.layoutForPlatform(gtx, "ios")

	if !renderer.compact {
		t.Fatal("iPhone landscape viewport was not classified as compact")
	}
	if renderer.outputScroller != nil {
		t.Fatal("iPhone landscape output retained a nested vertical scroller")
	}
}

func TestCompactHeroUsesFrameModeWithUnboundedListHeight(t *testing.T) {
	theme, err := findTheme("default")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(&uidsl.ScreenDocument{}, theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	node := uidsl.Node{
		Component: "row",
		Layout:    uidsl.Layout{Direction: "horizontal", Gap: "small"},
		Style:     uidsl.Style{Role: "hero"},
		Children: []uidsl.Node{
			{Component: "text", Text: &uidsl.Text{Literal: "First"}},
			{Component: "text", Text: &uidsl.Text{Literal: "Second"}},
			{Component: "text", Text: &uidsl.Text{Literal: "Third"}},
		},
	}
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
		Constraints: layout.Constraints{Max: image.Pt(1334, 1_000_000)},
	}
	renderer.compact = false
	desktop := renderer.layoutChildren(gtx, node, map[string]any{}, "desktop-hero")
	gtx.Ops.Reset()
	renderer.compact = true
	compact := renderer.layoutChildren(gtx, node, map[string]any{}, "compact-hero")
	if compact.Size.Y <= desktop.Size.Y {
		t.Fatalf("compact hero height = %d, want greater than desktop row height %d", compact.Size.Y, desktop.Size.Y)
	}
}

func TestCompactControlRowDoesNotExplodeVertically(t *testing.T) {
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
	renderer.compact = true
	node := uidsl.Node{Component: "row", Layout: uidsl.Layout{Direction: "horizontal", Gap: "small"}, Children: []uidsl.Node{
		{Component: "text", Text: &uidsl.Text{Literal: "release-ios-gated"}, Layout: uidsl.Layout{Grow: true}},
		{Component: "button", Text: &uidsl.Text{Literal: "Options"}, Icon: "adjustments"},
		{Component: "button", Text: &uidsl.Text{Literal: "Run Chain"}, Icon: "player-play"},
		{Component: "button", Text: &uidsl.Text{Literal: "Dry Run"}, Icon: "player-play"},
	}}
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Constraints{Max: image.Pt(350, 2000)}}
	dimensions := renderer.layoutChildren(gtx, node, map[string]any{}, "compact-controls")
	if dimensions.Size.Y > 240 {
		t.Fatalf("compact control row height = %d, want <= 240", dimensions.Size.Y)
	}
}

func TestCompactExecutionDisclosureKeepsTitleReadable(t *testing.T) {
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
	renderer.compact = true
	node := uidsl.Node{
		Component: "disclosure", Text: &uidsl.Text{Literal: "ciwi Build and release Tue 04 Aug, 23:25:26 v0.2.7"},
		Disclosure: &uidsl.Disclosure{Summary: []uidsl.Node{
			{Component: "spacer", Layout: uidsl.Layout{Grow: true}},
			{Component: "text", Text: &uidsl.Text{Literal: "17/17 successful"}, Style: uidsl.Style{Emphasis: "strong"}},
			{Component: "button", Text: &uidsl.Text{Literal: "Delete execution"}, Icon: "trash", Style: uidsl.Style{Role: "icon-button"}},
		}},
		Style:  uidsl.Style{Role: "execution-row", Tone: "success", Emphasis: "strong"},
		Layout: uidsl.Layout{Padding: "small"},
	}
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Constraints{Max: image.Pt(350, 2000)}}
	dimensions := renderer.layoutNode(gtx, node, map[string]any{}, "compact-execution")
	if dimensions.Size.Y > 180 {
		t.Fatalf("compact execution header height = %d, want <= 180", dimensions.Size.Y)
	}
}

func TestCompactFrontPageContentRemainsBounded(t *testing.T) {
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
		Server: &cnpv1.ServerInfo{Version: "v0.2.7"},
		Projects: []*cnpv1.ProjectSummary{{
			Id: 1, Name: "ciwi", RepoUrl: "https://github.com/izzyreal/ciwi", RepoRef: "main", ConfigFile: "ciwi-project.yaml",
			Pipelines: []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "release-ios-gated", SupportsDryRun: true}},
		}},
		HistoryExecutions: []*cnpv1.ExecutionCardSummary{{
			Key: "chain:release", Title: "ciwi Build and release Tue 04 Aug, 23:25:26 v0.2.7",
			Summary: &cnpv1.ExecutionSummary{TotalJobs: 17, Succeeded: 17},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(375, 667))}
	renderer.Layout(gtx)
	// Text shaping varies slightly between the system font backends used by
	// macOS development and Linux CI. Keep enough tolerance for those metrics
	// while retaining a low ceiling that catches the multi-thousand-pixel row
	// expansion this regression test was introduced for.
	const maxCompactContentHeight = 2400
	if renderer.list.Position.Length > maxCompactContentHeight {
		t.Fatalf("compact front page content height = %d, want <= %d", renderer.list.Position.Length, maxCompactContentHeight)
	}
}

func TestCompactProjectDisclosureNavigatesWithoutOpeningSheet(t *testing.T) {
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
		Server: &cnpv1.ServerInfo{Version: "v0.2.8"},
		Projects: []*cnpv1.ProjectSummary{{
			Id: 1, Name: "ciwi", RepoUrl: "https://github.com/izzyreal/ciwi", RepoRef: "main", ConfigFile: "ciwi-project.yaml",
			Pipelines: []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "build"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	navigationCount := 0
	navigatedRoute := ""
	renderer.onAction = func(action uidsl.Action, arguments map[string]string) {
		if action.Command == "navigate" {
			navigationCount++
			navigatedRoute = arguments["route"]
		}
	}
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(390, 844))}
	renderer.Layout(gtx)
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "build" {
			t.Fatal("compact project details were rendered inline before opening the sheet")
		}
	}
	var projectHeader *widget.Clickable
	for path, button := range renderer.buttons {
		if strings.HasSuffix(path, "/disclosure-header") {
			projectHeader = button
			break
		}
	}
	if projectHeader == nil {
		t.Fatal("compact project disclosure header was not created")
	}
	projectHeader.Click()
	gtx.Ops.Reset()
	renderer.Layout(gtx)
	if navigationCount != 1 || navigatedRoute != "/projects/1" {
		t.Fatalf("project header navigation = (%d, %q), want (1, /projects/1)", navigationCount, navigatedRoute)
	}
	if renderer.activeSheet != nil {
		t.Fatalf("compact project header opened a sheet: %#v", renderer.activeSheet)
	}
	if _, exists := renderer.disclosures["front-project:1"]; exists {
		t.Fatalf("compact project navigation mutated disclosure state: %v", renderer.disclosures)
	}

	var projectName *widget.Clickable
	for path, button := range renderer.buttons {
		if strings.HasSuffix(path, "/summary/0") {
			projectName = button
			break
		}
	}
	if projectName == nil {
		t.Fatal("compact project-name navigation action was not created")
	}
	projectName.Click()
	gtx.Ops.Reset()
	renderer.Layout(gtx)
	if navigationCount != 2 || navigatedRoute != "/projects/1" {
		t.Fatalf("project-name navigation = (%d, %q), want one additional navigation", navigationCount, navigatedRoute)
	}
}

func TestFlexAlignmentDefaultsColumnsToStartAndRowsToMiddle(t *testing.T) {
	if got := flexAlignment(layout.Vertical, "", false); got != layout.Start {
		t.Fatalf("vertical alignment = %v, want start", got)
	}
	if got := flexAlignment(layout.Horizontal, "", false); got != layout.Middle {
		t.Fatalf("horizontal alignment = %v, want middle", got)
	}
	if got := flexAlignment(layout.Vertical, "center", false); got != layout.Middle {
		t.Fatalf("explicit center alignment = %v, want middle", got)
	}
	if got := flexAlignment(layout.Horizontal, "center", true); got != layout.Middle {
		t.Fatalf("explicit execution-grid alignment = %v, want middle", got)
	}
}

func TestTreeEntryNodeKeepsLeafActionsInCompactRows(t *testing.T) {
	tree := &uidsl.TreeView{
		StateKey: "artifacts", As: "treeNode", NodeKey: "treeNode.key",
		NodeLabel: uidsl.Text{Binding: "treeNode.label"}, NodeDetail: uidsl.Text{Binding: "treeNode.detail"},
		Children: "treeNode.children", ActionLabel: uidsl.Text{Binding: "treeNode.action_label"},
	}
	node := uidsl.Node{Component: "tree-view", TreeView: tree, Actions: []uidsl.Action{{On: "activate", Command: "download-artifact"}}}
	leaf, err := treeEntryNode(node, map[string]any{"treeNode": map[string]any{
		"key": "file", "label": "Ciwi.exe", "detail": "17.7 MB", "action_label": "Download", "children": []any{},
	}}, "file")
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Component != "row" || leaf.Style.Role != "tree-row" || len(leaf.Children) != 3 {
		t.Fatalf("leaf node = %+v", leaf)
	}
	if !leaf.Children[0].Layout.Grow || leaf.Children[2].Style.Role != "tree-action" {
		t.Fatalf("leaf layout = %+v", leaf.Children)
	}

	branch, err := treeEntryNode(node, map[string]any{"treeNode": map[string]any{
		"key": "dist", "label": "dist", "detail": "", "action_label": "Download .zip",
		"children": []any{map[string]any{"key": "file"}},
	}}, "dist")
	if err != nil {
		t.Fatal(err)
	}
	if branch.Component != "disclosure" || branch.Style.Role != "tree-branch" || len(branch.Disclosure.Summary) != 1 || branch.Disclosure.Summary[0].Style.Role != "tree-action" {
		t.Fatalf("branch node = %+v", branch)
	}
}

func TestProjectStructureFilterIncludesChainsAndSurvivesRefresh(t *testing.T) {
	renderer := &Renderer{}
	data := map[string]any{"projectDetails": map[string]any{
		"project": map[string]any{"id": 1, "name": "ciwi", "pipeline_chains": []any{map[string]any{
			"id": "release", "name": "Release", "sequence_label": "build → release", "pipelines": []any{"build", "release"},
		}}},
		"pipelines": []any{
			map[string]any{"pipeline_id": "build"}, map[string]any{"pipeline_id": "lint"}, map[string]any{"pipeline_id": "release"},
		},
		"structure_filters": []any{
			map[string]any{"value": "all-pipelines", "label": "All Pipelines", "pipeline_ids": []any{"build", "lint", "release"}, "show_pipeline_structure": true, "root": map[string]any{"id": "project:1:all-pipelines", "label": "ciwi", "meta": "Project · 3 pipelines", "runnable": false}},
			map[string]any{"value": "all-chains", "label": "All chains", "pipeline_ids": []any{}, "show_chain_structure": true, "root": map[string]any{"id": "project:1:all-chains", "label": "ciwi", "meta": "Project · 1 pipeline chain", "runnable": false}},
			map[string]any{"value": "chain:release", "label": "Release (chain)", "pipeline_ids": []any{"build", "release"}, "show_pipeline_structure": true, "root": map[string]any{"id": "chain:release", "label": "Chain: Release", "meta": "build → release", "runnable": true, "project_id": float64(1), "chain_id": "release"}},
		},
	}}
	renderer.SetData(data)
	if !renderer.SetProjectStructureFilter("all-chains") {
		t.Fatal("all-chains filter was rejected")
	}
	root := renderer.data.(map[string]any)["projectDetails"].(map[string]any)
	if visible := root["visible_pipelines"].([]any); len(visible) != 0 {
		t.Fatalf("all-chains visible pipelines = %#v, want none", visible)
	}
	if structureRoot := root["structure_root"].(map[string]any); structureRoot["meta"] != "Project · 1 pipeline chain" || structureRoot["runnable"] != false {
		t.Fatalf("all-chains structure root = %#v", structureRoot)
	}
	if !renderer.SetProjectStructureFilter("chain:release") {
		t.Fatal("chain filter was rejected")
	}
	root = renderer.data.(map[string]any)["projectDetails"].(map[string]any)
	if visible := root["visible_pipelines"].([]any); len(visible) != 2 {
		t.Fatalf("visible pipelines = %#v", visible)
	}
	structureRoot := root["structure_root"].(map[string]any)
	if structureRoot["id"] != "chain:release" || structureRoot["runnable"] != true || structureRoot["chain_id"] != "release" {
		t.Fatalf("chain structure root = %#v", structureRoot)
	}
	renderer.SetScreenAndData(&uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "project-details"}}, data)
	root = renderer.data.(map[string]any)["projectDetails"].(map[string]any)
	if root["structure_filter"] != "chain:release" || len(root["visible_pipelines"].([]any)) != 2 {
		t.Fatalf("refreshed filter state = %#v", root)
	}
}

func TestManagedYAMLActionArgumentsPreserveCITemplateBindings(t *testing.T) {
	raw := "steps:\n  - run: GOOS={{goos}} GOARCH={{goarch}} go build"
	arguments, err := actionArguments(uidsl.Action{Arguments: map[string]string{
		"projectId": "{{managedYAML.project_id}}",
		"revision":  "{{managedYAML.revision}}",
		"yaml":      "{{managedYAML.yaml}}",
	}}, map[string]any{"managedYAML": map[string]any{
		"project_id": 0, "revision": "", "yaml": raw,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if arguments["yaml"] != raw {
		t.Fatalf("managed YAML argument = %q, want %q", arguments["yaml"], raw)
	}
}

func BenchmarkRendererProjectDetailsCollapsed(b *testing.B) {
	screen, err := sharedUI.LoadScreen("project-details")
	if err != nil {
		b.Fatal(err)
	}
	theme, err := findTheme("default")
	if err != nil {
		b.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		b.Fatal(err)
	}
	view := &cnpv1.ProjectDetailsView{Project: &cnpv1.ProjectSummary{Id: 1, Name: "VMPC2000XL"}}
	for pipelineIndex := 0; pipelineIndex < 10; pipelineIndex++ {
		pipeline := &cnpv1.ProjectPipelineDetails{
			Id: int64(pipelineIndex + 1), PipelineId: "pipeline", Dependencies: "none", JobsCount: 2,
		}
		for jobIndex := 0; jobIndex < 2; jobIndex++ {
			job := &cnpv1.ProjectJobDetails{Id: "job", StepsCount: 12}
			for stepIndex := 0; stepIndex < 12; stepIndex++ {
				job.Steps = append(job.Steps, &cnpv1.ProjectStepDetails{
					Index: uint32(stepIndex), Position: uint32(stepIndex + 1), Name: "Configured build step", Type: "run",
				})
			}
			pipeline.Jobs = append(pipeline.Jobs, job)
		}
		view.Pipelines = append(view.Pipelines, pipeline)
	}
	data, err := projectDetailsBindingData(view)
	if err != nil {
		b.Fatal(err)
	}
	renderer.SetData(data)
	var operations op.Ops
	constraints := layout.Exact(image.Pt(1900, 1200))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		operations.Reset()
		renderer.Layout(layout.Context{Ops: &operations, Constraints: constraints})
	}
}
