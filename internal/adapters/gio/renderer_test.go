//go:build darwin || linux || windows

package gio

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
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
	want := map[font.Weight]bool{font.Normal: false, font.Bold: false}
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
	if logo, ok := renderer.images["ciwi-logo"]; !ok || logo.Filter != paint.FilterNearest {
		t.Fatal("ciwi logo must use nearest-neighbor filtering")
	}
	if renderer.theme.Face != ciwiBodyTypeface {
		t.Fatalf("native body typeface = %q, want browser stack %q", renderer.theme.Face, ciwiBodyTypeface)
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
	if renderer.metrics.textBody != 16 || renderer.metrics.textControl != 14 || renderer.metrics.textHeading != 18 || renderer.metrics.textTitle != 28 {
		t.Fatalf("native type scale = body %v control %v heading %v title %v, want browser 16/14/18/28", renderer.metrics.textBody, renderer.metrics.textControl, renderer.metrics.textHeading, renderer.metrics.textTitle)
	}
	if renderer.metrics.heroPadding != 16 || renderer.metrics.imageBrandWidth != 110 || renderer.metrics.imageBrandHeight != 91 {
		t.Fatalf("native masthead metrics = padding %v image %vx%v, want browser 16 and 110x91", renderer.metrics.heroPadding, renderer.metrics.imageBrandWidth, renderer.metrics.imageBrandHeight)
	}
	if !renderer.statusText.ReadOnly {
		t.Fatal("status text must remain selectable but read-only")
	}
	projectIcon, err := sharedUI.Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server: &cnpv1.ServerInfo{Version: "v0.2.0"},
		Projects: []*cnpv1.ProjectSummary{{
			Id: 1, Name: "ciwi", RepoUrl: "https://github.com/izzyreal/ciwi",
			ProjectIcon: projectIcon, ProjectIconContentType: "image/png",
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
		if selectable.Text() == "v0.2.0" {
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
	if renderer.icons["settings"] == nil || renderer.icons["player-play"] == nil || renderer.icons["loader-2"] == nil || renderer.icons["arrow-left"] == nil || renderer.icons["trash"] == nil || renderer.icons["zoom-in"] == nil || renderer.icons["zoom-out"] == nil {
		t.Fatal("declared screen icons are unavailable to the native renderer")
	}
}

func TestExecutionCardDecorationMatchesWebSummary(t *testing.T) {
	cards := []any{map[string]any{
		"summary": map[string]any{
			"total_jobs": float64(17), "succeeded": float64(12),
			"failed": float64(0), "in_progress": float64(3), "waiting": float64(2),
		},
	}}
	decorateExecutionCards(cards, true)
	card := cards[0].(map[string]any)
	if got := card["summary_label"]; got != "12/17 successful, 3 in progress, 2 waiting" {
		t.Fatalf("summary label = %q", got)
	}
	if card["status"] != "running" || card["summary_tone"] != "warning" {
		t.Fatalf("status decoration = %#v", card)
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
			JobExecutionIds: []string{"job-1", "job-2"},
			Sections:        []*cnpv1.ExecutionCardSection{{Key: "build", Label: "build", Jobs: []*cnpv1.ExecutionCardJob{{Id: "job-1", Label: "linux", Status: "succeeded"}}}},
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
	if summary := renderer.selectable(historyDisclosure + "/summary/0"); summary.Text() != "1/1 successful" {
		t.Fatalf("history summary = %q", summary.Text())
	}
	deleteButton := renderer.buttons[historyDisclosure+"/summary/1"]
	if deleteButton == nil {
		t.Fatal("collapsed history execution does not expose its delete action")
	}
	deleteButton.Click()
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if renderer.pending == nil || renderer.pending.action.Command != "delete-execution" {
		t.Fatal("history header delete action did not request confirmation")
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
			Pipelines: []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "build"}},
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
	for path, selectable := range renderer.selectables {
		if selectable.Text() == "Project:" && strings.HasSuffix(path, "/label") {
			projectPath = strings.TrimSuffix(path, "/label")
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
			Jobs: []*cnpv1.ProjectJobDetails{{
				Id: "compile", StepsCount: 1, SupportsDryRun: true, RunsOnLabel: "darwin/arm64", ToolsLabel: "go=1.25", TimeoutSeconds: 600, MatrixCount: 1,
				Steps: []*cnpv1.ProjectStepDetails{{Index: 0, Position: 1, Name: "Compile", Type: "run", Command: "go build ./...", Environment: []string{"CGO_ENABLED=1"}, SkipDryRun: true}},
			}},
		}},
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
	if got := len(renderer.buttons); got < 8 {
		t.Fatalf("project details did not expose chain, pipeline, dry-run, and job controls: %d widgets", got)
	}
	var foundJob, foundChain, foundChainSequence, foundPipelineSummary, foundJobSummary bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "Job: compile" {
			foundJob = true
		}
		if selectable.Text() == "Chain: Build and release" {
			foundChain = true
		}
		if selectable.Text() == "1 job · depends on: none" {
			foundPipelineSummary = true
		}
		if selectable.Text() == "1 step · runs on: darwin/arm64" {
			foundJobSummary = true
		}
	}
	for _, editor := range renderer.textEditors {
		if editor.Text() == "build → release" {
			foundChainSequence = true
			if editor.SingleLine {
				t.Fatal("long monospace chain labels must remain wrap-capable")
			}
		}
	}
	if !foundJob {
		t.Fatal("pipeline did not default to expanded")
	}
	if !foundChain {
		t.Fatal("project pipeline chain was not rendered")
	}
	if !foundChainSequence {
		t.Fatal("project pipeline sequence was not rendered through the monospace text path")
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
			{Id: 7, PipelineId: "build", Dependencies: "none", JobsCount: 2, Jobs: []*cnpv1.ProjectJobDetails{
				{Id: "unit-tests", StepsCount: 1, Steps: []*cnpv1.ProjectStepDetails{{Index: 0, Position: 1, Name: "Test", Type: "run", Command: "go test ./..."}}},
				{Id: "package", Needs: []string{"unit-tests"}, StepsCount: 1, Steps: []*cnpv1.ProjectStepDetails{{Index: 0, Position: 1, Name: "Package", Type: "run", Command: "go build ./..."}}},
			}},
			{Id: 8, PipelineId: "release", DependsOn: []string{"build"}, Dependencies: "build", JobsCount: 1, Jobs: []*cnpv1.ProjectJobDetails{{Id: "publish", StepsCount: 1}}},
		},
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
	if got := len(renderer.buttons); got != 8 {
		t.Fatalf("job view created %d clickable widgets, want execution controls, Back, timeline selection, and output buttons", got)
	}
	if len(renderer.scrollers) != 2 {
		t.Fatalf("execution-path and grouped-output scrollers = %d", len(renderer.scrollers))
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

func TestTransientStatusExpiresWithoutClearingPersistentErrors(t *testing.T) {
	renderer := &Renderer{}
	renderer.SetTransientStatus("Queued", time.Hour)
	expires := renderer.StatusExpiry()
	if expires.IsZero() || renderer.ClearExpiredStatus(expires.Add(-time.Nanosecond)) {
		t.Fatal("transient status expired too early")
	}
	if !renderer.ClearExpiredStatus(expires) || renderer.status != "" || !renderer.StatusExpiry().IsZero() {
		t.Fatalf("expired status was not cleared: status=%q expiry=%v", renderer.status, renderer.StatusExpiry())
	}
	renderer.SetTransientStatus("Queued", time.Hour)
	renderer.SetStatus("Run failed")
	if !renderer.StatusExpiry().IsZero() || renderer.ClearExpiredStatus(time.Now().Add(2*time.Hour)) || renderer.status != "Run failed" {
		t.Fatalf("persistent status unexpectedly expired: status=%q expiry=%v", renderer.status, renderer.StatusExpiry())
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
	if state != "overrun" || fraction != 1 {
		t.Fatalf("state=%q fraction=%g", state, fraction)
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
	dimensions := renderer.layoutRootChildren(children, root, screen, map[string]any{}, "")(gtx)
	if dimensions.Size != image.Pt(800, 120) {
		t.Fatalf("viewport dimensions = %v", dimensions.Size)
	}
	if renderer.list.Position.Length != 244 {
		t.Fatalf("scroll content length = %d, want top 16 + children 200 + gap 12 + bottom 16 = 244", renderer.list.Position.Length)
	}
}

func TestCompactPageInsetUsesPhoneSizedGutter(t *testing.T) {
	renderer := &Renderer{metrics: visualMetrics{pageInset: 16}}
	phone := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(375, 667))}
	if got := renderer.pageInset(phone); got != 3.2 {
		t.Fatalf("phone page inset = %v, want 3.2", got)
	}
	tablet := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(834, 1112))}
	if got := renderer.pageInset(tablet); got != 16 {
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
	node := uidsl.Node{
		Component: "disclosure", Text: &uidsl.Text{Literal: "ciwi Build and release Tue 04 Aug, 23:25:26 v0.2.7"},
		Disclosure: &uidsl.Disclosure{Summary: []uidsl.Node{
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

func TestCompactProjectDisclosureOpensAndClosesSheet(t *testing.T) {
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
	if renderer.activeSheet == nil || renderer.activeSheet.title != "Project: ciwi" {
		t.Fatalf("active compact sheet = %#v", renderer.activeSheet)
	}
	gtx.Ops.Reset()
	renderer.Layout(gtx)
	if title := renderer.selectable("compact-sheet/title").Text(); title != "Project: ciwi" {
		t.Fatalf("compact sheet title = %q", title)
	}
	foundPipeline := false
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "build" {
			foundPipeline = true
			break
		}
	}
	if !foundPipeline {
		t.Fatal("compact project sheet did not render pipeline details")
	}
	renderer.button("compact-sheet/close").Click()
	gtx.Ops.Reset()
	renderer.Layout(gtx)
	if renderer.activeSheet != nil {
		t.Fatal("compact sheet did not close")
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
	if got := flexAlignment(layout.Horizontal, "center", true); got != layout.Start {
		t.Fatalf("execution-grid alignment = %v, want start", got)
	}
}

func TestProjectStructureFilterIncludesChainsAndSurvivesRefresh(t *testing.T) {
	renderer := &Renderer{}
	data := map[string]any{"projectDetails": map[string]any{
		"project": map[string]any{"pipeline_chains": []any{map[string]any{"id": "release", "pipelines": []any{"build", "release"}}}},
		"pipelines": []any{
			map[string]any{"pipeline_id": "build"}, map[string]any{"pipeline_id": "lint"}, map[string]any{"pipeline_id": "release"},
		},
	}}
	renderer.SetData(data)
	if !renderer.SetProjectStructureFilter("chain:release") {
		t.Fatal("chain filter was rejected")
	}
	root := renderer.data.(map[string]any)["projectDetails"].(map[string]any)
	if visible := root["visible_pipelines"].([]any); len(visible) != 2 {
		t.Fatalf("visible pipelines = %#v", visible)
	}
	renderer.SetScreenAndData(&uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "project-details"}}, data)
	root = renderer.data.(map[string]any)["projectDetails"].(map[string]any)
	if root["structure_filter"] != "chain:release" || len(root["visible_pipelines"].([]any)) != 2 {
		t.Fatalf("refreshed filter state = %#v", root)
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
