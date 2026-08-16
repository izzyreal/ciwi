//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedui "github.com/izzyreal/ciwi/ui"
)

func TestCompiledScreenDOMIsReusedForAnimationOnlyFrames(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	screen := &uidsl.ScreenDocument{
		APIVersion: uidsl.APIVersion, Kind: "Screen", Metadata: uidsl.Metadata{Name: "cached-animation"},
		Screen: uidsl.Screen{Root: uidsl.Node{
			Component: "page", ID: "cached-animation",
			Children: []uidsl.Node{{
				Component: "button", ID: "update-check", Text: &uidsl.Text{Binding: "form.label"},
				Actions: []uidsl.Action{{On: "activate", Command: "check-server-updates"}},
			}},
		}},
	}
	renderer.SetScreenAndData(screen, map[string]any{"form": map[string]any{"label": "Check for updates"}})
	fingerprint, err := operations.Fingerprint("check-server-updates", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	renderer.SetOperations([]operations.Operation{{
		ID: "update-check", Fingerprint: fingerprint, State: operations.StateRunning, PendingLabel: "Checking for updates…",
	}})
	button := screen.Screen.Root.Children[0]
	if label, enabled := renderer.buttonNodeState(&button, renderer.data); label != "Checking for updates…" || enabled || button.Icon != "loader-2" {
		t.Fatalf("pending button state = (%q, %v, %q)", label, enabled, button.Icon)
	}

	if at, wake := layoutCachedScreenFrame(renderer, time.Unix(1_800_000_000, 0), image.Pt(800, 600)); !wake || !at.IsZero() {
		t.Fatalf("first loader wakeup = (%v, %v), want immediate", at, wake)
	}
	if got := renderer.dom.compiled.builds; got != 1 {
		t.Fatalf("initial compiled DOM builds = %d, want 1", got)
	}
	if at, wake := layoutCachedScreenFrame(renderer, time.Unix(1_800_000_000, int64(time.Second/60)), image.Pt(800, 600)); !wake || !at.IsZero() {
		t.Fatalf("reused loader wakeup = (%v, %v), want immediate", at, wake)
	}
	if got := renderer.dom.compiled.builds; got != 1 {
		t.Fatalf("animation-only compiled DOM builds = %d, want 1", got)
	}

	if !renderer.SetRootBinding("form", "label", "Checking") {
		t.Fatal("could not update cached screen binding")
	}
	layoutCachedScreenFrame(renderer, time.Unix(1_800_000_001, 0), image.Pt(800, 600))
	if got := renderer.dom.compiled.builds; got != 2 {
		t.Fatalf("data-change compiled DOM builds = %d, want 2", got)
	}

	renderer.ShowAlert("Cached overlay", "Overlay changes must not rebuild the base screen.")
	layoutCachedScreenFrame(renderer, time.Unix(1_800_000_002, 0), image.Pt(800, 600))
	if got := renderer.dom.compiled.builds; got != 2 {
		t.Fatalf("overlay compiled DOM builds = %d, want 2", got)
	}

	renderer.SetOperations(nil)
	if _, wake := layoutCachedScreenFrame(renderer, time.Unix(1_800_000_003, 0), image.Pt(800, 600)); wake {
		t.Fatal("completed operation retained loader animation")
	}
	if got := renderer.dom.compiled.builds; got != 3 {
		t.Fatalf("operation-change compiled DOM builds = %d, want 3", got)
	}

	layoutCachedScreenFrame(renderer, time.Unix(1_800_000_004, 0), image.Pt(801, 600))
	if got := renderer.dom.compiled.builds; got != 4 {
		t.Fatalf("viewport-change compiled DOM builds = %d, want 4", got)
	}
}

func TestAnimatedLoaderRequestsImmediateFrame(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	router := new(input.Router)
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now: time.Unix(1_800_000_000, 0), Constraints: layout.Exact(image.Pt(21, 21)),
	}
	renderer.layoutAnimatedLoader(gtx, renderer.palette.accent)
	router.Frame(operations)
	at, wake := router.WakeupTime()
	if !wake || !at.IsZero() {
		t.Fatalf("loader wakeup = (%v, %v), want immediate", at, wake)
	}
}

func BenchmarkCachedSettingsAnimationFrame(b *testing.B) {
	screen, err := sharedui.LoadScreen("settings")
	if err != nil {
		b.Fatal(err)
	}
	themes, err := sharedui.LoadThemes()
	if err != nil || len(themes) == 0 {
		b.Fatalf("themes = %d, %v", len(themes), err)
	}
	renderer, err := NewRenderer(screen, themes[0], nil)
	if err != nil {
		b.Fatal(err)
	}
	data, err := offlineSettingsBindingData("benchmark", themes[0].Metadata.Name, connectionModeDiscover, "", sshConnectionSettings{})
	if err != nil {
		b.Fatal(err)
	}
	renderer.SetScreenAndData(screen, data)
	fingerprint, err := operations.Fingerprint("check-server-updates", map[string]string{})
	if err != nil {
		b.Fatal(err)
	}
	renderer.SetOperations([]operations.Operation{{
		ID: "update-check", Fingerprint: fingerprint, State: operations.StateRunning, PendingLabel: "Checking for updates…",
	}})
	renderer.ScrollToSection("server-version-management")

	operations := new(op.Ops)
	frame := func(now time.Time) {
		gtx := layout.Context{
			Ops: operations, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: now,
			Constraints: layout.Exact(image.Pt(1180, 780)),
		}
		renderer.Layout(gtx)
		operations.Reset()
	}
	frame(time.Unix(1_800_000_000, 0))
	if renderer.dom == nil || renderer.dom.compiled.builds != 1 {
		b.Fatalf("initial compiled DOM cache = %#v", renderer.dom)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		frame(time.Unix(1_800_000_000, int64(iteration+1)*int64(time.Second/60)))
	}
	b.StopTimer()
	if got := renderer.dom.compiled.builds; got != 1 {
		b.Fatalf("steady-frame compiled DOM builds = %d, want 1", got)
	}
}

func layoutCachedScreenFrame(renderer *Renderer, now time.Time, size image.Point) (time.Time, bool) {
	router := new(input.Router)
	operations := new(op.Ops)
	gtx := layout.Context{
		Ops: operations, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: now,
		Constraints: layout.Exact(size),
	}
	renderer.Layout(gtx)
	router.Frame(operations)
	return router.WakeupTime()
}
