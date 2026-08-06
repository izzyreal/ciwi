//go:build darwin || linux || windows

package gio

import (
	"fmt"
	"image"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

func BenchmarkFrontPageCollapsedExecutionHistoryGPU(b *testing.B) {
	const historyCount = 30
	base := time.Unix(1_800_000_000, 0)
	history := make([]*cnpv1.ExecutionCardSummary, 0, historyCount)
	for index := range historyCount {
		id := fmt.Sprintf("history-%02d", index)
		history = append(history, &cnpv1.ExecutionCardSummary{
			Key: id, Kind: "pipeline", Title: "ciwi build and test",
			JobExecutionIds: []string{id + "-job"},
			Summary:         &cnpv1.ExecutionSummary{TotalJobs: 1, Succeeded: 1},
			Sections: []*cnpv1.ExecutionCardSection{{
				Key: "build", Label: "build",
				Jobs: []*cnpv1.ExecutionCardJob{{Id: id + "-job", Label: "macOS", Status: "succeeded"}},
			}},
		})
	}
	running := &cnpv1.ExecutionCardSummary{
		Key: "running", Kind: "pipeline", Title: "ciwi native client",
		JobExecutionIds: []string{"running-job"},
		Summary:         &cnpv1.ExecutionSummary{TotalJobs: 1, InProgress: 1},
		Progress:        &cnpv1.Progress{State: "indeterminate", SnapshotUnixMs: base.UnixMilli()},
		Sections: []*cnpv1.ExecutionCardSection{{
			Key: "build", Label: "build",
			Progress: &cnpv1.Progress{State: "indeterminate", SnapshotUnixMs: base.UnixMilli()},
			Jobs: []*cnpv1.ExecutionCardJob{{
				Id: "running-job", Label: "macOS", Status: "in-progress",
				Progress: &cnpv1.Progress{State: "indeterminate", SnapshotUnixMs: base.UnixMilli()},
			}},
		}},
	}

	screen, err := sharedUI.LoadScreen("front-page")
	if err != nil {
		b.Fatal(err)
	}
	theme, err := findTheme("space")
	if err != nil {
		b.Fatal(err)
	}
	renderer, err := NewRenderer(screen, theme, nil)
	if err != nil {
		b.Fatal(err)
	}
	data, err := frontPageBindingData(&cnpv1.FrontPageView{
		Server:            &cnpv1.ServerInfo{Version: "benchmark"},
		QueuedExecutions:  []*cnpv1.ExecutionCardSummary{running},
		HistoryExecutions: history,
	})
	if err != nil {
		b.Fatal(err)
	}
	renderer.SetData(data)
	// Position the page at the execution sections, matching the mostly-static
	// workload this benchmark protects. All disclosures remain collapsed.
	renderer.list.Position.First = 2

	size := image.Pt(1440, 900)
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		b.Skipf("headless GPU unavailable: %v", err)
	}
	defer window.Release()
	var operations op.Ops
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(size),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	frame := 0
	drawFrame := func() {
		operations.Reset()
		gtx.Now = base.Add(time.Duration(frame) * progressFrameInterval)
		renderer.Layout(gtx)
		if err := window.Frame(&operations); err != nil {
			b.Fatal(err)
		}
		frame++
	}
	for range 3 {
		drawFrame()
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		drawFrame()
	}
}
