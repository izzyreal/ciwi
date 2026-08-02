//go:build darwin

package gio

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	var foundTitle bool
	var foundChain bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "ciwi v0.2.0" {
			foundTitle = true
		}
		if selectable.Text() == "Chain: Build and release" {
			foundChain = true
		}
	}
	if !foundTitle {
		t.Fatal("front-page title is not rendered as selectable text")
	}
	if !foundChain {
		t.Fatal("front-page pipeline chain is not rendered")
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
		Project: &cnpv1.ProjectSummary{Id: 1, Name: "ciwi", PipelineChains: []*cnpv1.PipelineChainSummary{{
			Id: "build+release", Name: "Build and release", SequenceLabel: "build → release", SupportsDryRun: true,
		}}},
		Pipelines: []*cnpv1.ProjectPipelineDetails{{
			Id: 7, PipelineId: "build", Dependencies: "none", JobsCount: 1, SupportsDryRun: true,
			Jobs: []*cnpv1.ProjectJobDetails{{
				Id: "compile", StepsCount: 1, SupportsDryRun: true,
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
	if got := len(renderer.buttons); got < 8 {
		t.Fatalf("project details did not expose chain, pipeline, dry-run, and job controls: %d widgets", got)
	}
	var foundJob, foundChain bool
	for _, selectable := range renderer.selectables {
		if selectable.Text() == "Job: compile" {
			foundJob = true
		}
		if selectable.Text() == "Chain: Build and release" {
			foundChain = true
		}
	}
	if !foundJob {
		t.Fatal("pipeline did not default to expanded")
	}
	if !foundChain {
		t.Fatal("project pipeline chain was not rendered")
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
