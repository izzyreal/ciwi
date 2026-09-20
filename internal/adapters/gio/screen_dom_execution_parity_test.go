//go:build darwin || ios || linux || windows

package gio

import (
	"testing"

	"gioui.org/font"
	"github.com/izzyreal/ciwi/internal/giodom"
	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestExecutionRowsUseWebSummaryTypographyAndProgressBackground(t *testing.T) {
	screen, err := sharedui.LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"frontPage.queued_executions", "frontPage.history_executions"} {
		t.Run(source, func(t *testing.T) {
			renderer := responsiveTestRenderer(t)
			list, ok := findResponsiveTestNodeByRepeatSource(screen.Screen.Root, source)
			if !ok || len(list.Children) == 0 {
				t.Fatal("execution declaration missing")
			}
			for _, mode := range []string{"determinate", "complete", "indeterminate", "overrun"} {
				data := map[string]any{"execution": map[string]any{
					"kind": "pipeline", "job_execution_ids_csv": "job-1", "key": "parity", "title": "cupuacu Build and release", "status": "danger", "summary_tone": "danger",
					"summary_label": "2/11 successful, 1 failed, 5 in progress, 3 waiting",
					"summary":       map[string]any{"succeeded": 2, "total_jobs": 11},
					"progress":      map[string]any{"state": mode, "fraction": .35},
				}}
				compiled := renderer.compileDOMNode(list.Children[0], data, "execution")
				summary := findResponsiveTestElementByKey(compiled, "execution/summary/1")
				if summary == nil || summary.Kind != giodom.KindText {
					t.Fatal("summary text missing")
				}
				if summary.Text.Size != 14 || summary.Text.LineHeightScale != 1.2 || summary.Text.Font.Weight != font.Bold {
					t.Fatalf("summary typography = %+v", summary.Text)
				}
				progress := findResponsiveTestElementByKey(compiled, "execution/progress-header")
				if progress == nil || progress.Progress.CompositeBackground == nil || *progress.Progress.CompositeBackground != renderer.palette.surfaceRaised {
					t.Fatalf("%s: missing execution surface for sRGB compositing", mode)
				}
				dims := layoutResponsiveElement(renderer, *compiled, 1180, 780)
				// Two 16.8-unit lines plus 8-unit padding and a 1-unit border
				// on each side. History has a single line and a delete button.
				if dims.Size.Y > 54 {
					t.Fatalf("%s: collapsed row too tall: %v", mode, dims.Size)
				}
			}
		})
	}
}
