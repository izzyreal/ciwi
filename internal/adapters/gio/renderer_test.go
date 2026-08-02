//go:build darwin

package gio

import (
	"image"
	"strings"
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
			Sections: []*cnpv1.ExecutionCardSection{{Key: "build", Label: "build", Jobs: []*cnpv1.ExecutionCardJob{{Id: "job-1", Label: "linux", Status: "succeeded"}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetData(data)
	var operations op.Ops
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if navigated {
		t.Fatal("collapsed execution card navigated while being laid out")
	}
	var historyDisclosure string
	for key := range renderer.disclosures {
		if strings.Contains(key, "pipeline:build") {
			historyDisclosure = key
			break
		}
	}
	if historyDisclosure == "" {
		// Disclosure state is lazy; identify its stable toggle button path.
		for key := range renderer.buttons {
			if strings.Contains(key, "pipeline:build") && strings.HasSuffix(key, "/disclosure-toggle") {
				historyDisclosure = strings.TrimSuffix(key, "/disclosure-toggle")
				break
			}
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
	if renderer.disclosures[historyDisclosure] {
		t.Fatal("selecting disclosure label text unexpectedly expanded the card")
	}
	label.ClearSelection()
	renderer.button(historyDisclosure + "/disclosure-label").Click()
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if !renderer.disclosures[historyDisclosure] {
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
	if got := len(renderer.buttons); got != 3 {
		t.Fatalf("collapsed project view created %d interactive widgets, want Back plus pipeline disclosure controls", got)
	}
	renderer.disclosures["project-details/root/1/1/7/0"] = true
	operations.Reset()
	renderer.Layout(layout.Context{Ops: &operations, Constraints: layout.Exact(image.Pt(1100, 760))})
	if got := len(renderer.buttons); got < 4 {
		t.Fatalf("expanded pipeline did not expose its Run action and job disclosure: %d widgets", got)
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
	if got := len(renderer.buttons); got != 5 {
		t.Fatalf("collapsed job view created %d interactive widgets, want Back plus output and timeline disclosure controls", got)
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
	renderer.disclosures["job-details/root/2/1"] = true
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
