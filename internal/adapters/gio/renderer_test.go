//go:build darwin

package gio

import (
	"image"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
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
	if !renderer.statusText.ReadOnly {
		t.Fatal("status text must remain selectable but read-only")
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
	var foundTitle bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "ciwi v0.2.0" {
			foundTitle = true
			break
		}
	}
	if !foundTitle {
		t.Fatal("front-page title is not rendered as selectable text")
	}
	if _, ok := renderer.images["ciwi-logo"]; !ok {
		t.Fatal("embedded ciwi logo is unavailable to the native renderer")
	}
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "ciwi" {
			t.Fatal("image was rendered as placeholder text")
		}
	}
	if renderer.icons["settings"] == nil || renderer.icons["player-play"] == nil || renderer.icons["arrow-left"] == nil || renderer.icons["trash"] == nil {
		t.Fatal("declared screen icons are unavailable to the native renderer")
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
			Id: 1, Name: "ciwi", Pipelines: []*cnpv1.PipelineSummary{{Id: 7, PipelineId: "build"}},
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
		if selectable.Text() == "Project: ciwi" && strings.HasSuffix(path, "/label") {
			projectPath = strings.TrimSuffix(path, "/label")
			break
		}
	}
	if projectPath == "" {
		t.Fatal("project disclosure label was not rendered")
	}
	renderer.button(projectPath + "/disclosure-toggle").Click()
	operations.Reset()
	renderer.Layout(gtx)
	if renderer.disclosures["front-project:1"] || persisted["front-project:1"] {
		t.Fatalf("collapsed project state was not persisted: state=%v persisted=%v", renderer.disclosures, persisted)
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
	data, err := projectDetailsBindingData(&cnpv1.ProjectDetailsView{
		Project: &cnpv1.ProjectSummary{Id: 1, Name: "ciwi"},
		Pipelines: []*cnpv1.ProjectPipelineDetails{{
			Id: 7, PipelineId: "build", Dependencies: "none", JobsCount: 1,
			Jobs: []*cnpv1.ProjectJobDetails{{
				Id: "compile", StepsCount: 1,
				Steps: []*cnpv1.ProjectStepDetails{{Index: 0, Position: 1, Name: "Compile", Type: "run"}},
			}},
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
	if got := len(renderer.buttons); got < 4 {
		t.Fatalf("default-expanded pipeline did not expose its Run action and job disclosure: %d widgets", got)
	}
	var foundJob bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "Job: compile" {
			foundJob = true
			break
		}
	}
	if !foundJob {
		t.Fatal("pipeline did not default to expanded")
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
	selectPath := ""
	for path := range renderer.buttons {
		if strings.HasSuffix(path, "/select-toggle") {
			selectPath = strings.TrimSuffix(path, "/select-toggle")
			break
		}
	}
	if selectPath == "" {
		t.Fatal("settings theme select is unavailable")
	}
	renderer.button(selectPath + "/select-toggle").Click()
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	spaceOption := renderer.buttons[selectPath+"/option/space"]
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
	handleCommand(t.Context(), nil, renderer, nil, &navigationState{screen: "settings"}, commandRequest{
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
	if got := len(renderer.buttons); got != 9 {
		t.Fatalf("job view created %d interactive widgets, want execution controls, Back, timeline selection, and output controls", got)
	}
	if len(renderer.scrollers) != 1 {
		t.Fatalf("horizontal execution-path scrollers = %d", len(renderer.scrollers))
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
	data, err := jobDetailsBindingData(&cnpv1.JobDetailsView{Id: "job-1", Title: "Job: compile", StatusLabel: "Running", Mode: "Run"})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	renderer.SetRootBinding("jobDetails", "output", "hello\n")
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if len(renderer.textEditors) != 1 {
		t.Fatalf("text editors = %d", len(renderer.textEditors))
	}
	for _, editor := range renderer.textEditors {
		if !editor.ReadOnly || editor.Text() != "hello\n" {
			t.Fatalf("editor readOnly=%v text=%q", editor.ReadOnly, editor.Text())
		}
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
	renderer.outputEditor = new(widget.Editor)
	renderer.outputEditor.ReadOnly = true
	renderer.outputEditor.SetText("compile one\ncompile two\n")
	renderer.SetRootBinding("jobDetails", "output", "compile one\ncompile two\n")
	renderer.dispatchFromLayout(gtx, uidsl.Action{
		Command: "change-output-search", Arguments: map[string]string{"query": "{{input.value}}"},
	}, mergeData(renderer.data, "input", map[string]any{"value": "compile"}))
	if start, end := renderer.outputEditor.Selection(); start != 0 || end != len("compile") {
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
		Lines:          []*cnpv1.JobOutputLine{{Text: strings.Repeat("x", maxNativeOutputBytes+100)}},
	})
	if !strings.HasPrefix(buffer.text, "[ciwi native: earlier output omitted]\n") || len(buffer.text) > maxNativeOutputBytes+100 {
		t.Fatalf("buffer length=%d prefix=%q", len(buffer.text), buffer.text[:min(len(buffer.text), 50)])
	}
}

func TestSemanticToneUsesSharedStatusCategories(t *testing.T) {
	tests := map[string]string{
		"succeeded": "success", "failed": "danger", "queued": "warning",
		"in progress": "accent", "unknown": "muted",
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
