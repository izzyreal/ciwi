//go:build darwin || ios || linux || windows

package gio

import (
	"sync"
	"testing"
	"time"

	"github.com/izzyreal/ciwi/internal/giodom"
	"github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedUI "github.com/izzyreal/ciwi/ui"
)

func TestNativeUIDefersScreenAndDOMMutationUntilDrain(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	previousScreen := renderer.screen
	previousDOM := &screenDOMRenderer{runtime: giodom.NewRuntime(renderer.theme, giodom.Options{})}
	renderer.dom = previousDOM
	invalidations := 0
	ui := newNativeUI(func() { invalidations++ })
	target := &uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "settings"}}
	data := map[string]any{"settings": map[string]any{"value": "queued"}}

	ui.SetScreenAndData(target, data)
	data["settings"].(map[string]any)["value"] = "mutated-after-post"
	if renderer.screen != previousScreen || renderer.dom != previousDOM {
		t.Fatal("posting a screen update mutated the active frame")
	}
	if invalidations != 1 {
		t.Fatalf("invalidations = %d, want 1", invalidations)
	}

	ui.drain(renderer)
	if renderer.screen != target || renderer.dom != nil {
		t.Fatalf("drained screen = %p dom = %p, want target and reset DOM", renderer.screen, renderer.dom)
	}
	if got := bindingString(renderer.data, "settings.value"); got != "queued" {
		t.Fatalf("drained data value = %q, want immutable queued snapshot", got)
	}
}

func TestNativeUIQueuesEveryControllerMutation(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	invalidations := 0
	ui := newNativeUI(func() { invalidations++ })
	themes, err := sharedUI.LoadThemes()
	if err != nil || len(themes) == 0 {
		t.Fatalf("themes = %d, %v", len(themes), err)
	}
	screen := &uidsl.ScreenDocument{Metadata: uidsl.Metadata{Name: "mailbox"}}
	ui.SetScreenAndData(screen, map[string]any{"root": map[string]any{"nested": map[string]any{"before": true}}})
	ui.SetDataBinding("client", map[string]any{"connected": true})
	ui.SetRootBinding("root", "value", "queued")
	ui.SetNestedBinding("root", "nested", "value", "queued")
	ui.SetOperations([]operations.Operation{{Fingerprint: "active", State: operations.StateRunning}})
	ui.ApplyJobOutput(jobOutputSnapshot{})
	ui.ScrollToSection("target")
	ui.SetProjectStructureFilter("jobs")
	ui.SetTheme(themes[0])
	ui.ShowAlert("queued alert", "message")
	ui.ShowNotice("queued notice", "", uidsl.Action{}, nil, time.Second)
	if renderer.screen == screen || renderer.pendingScrollSection != "" || renderer.alert != nil || renderer.notice != nil {
		t.Fatal("controller mutation escaped the UI mailbox before drain")
	}
	if invalidations != 11 {
		t.Fatalf("invalidations = %d, want 11", invalidations)
	}

	ui.drain(renderer)
	if renderer.screen != screen || renderer.pendingScrollSection != "target" || renderer.alert == nil || renderer.notice == nil {
		t.Fatalf("drained renderer state: screen=%p scroll=%q alert=%#v notice=%#v", renderer.screen, renderer.pendingScrollSection, renderer.alert, renderer.notice)
	}
	if renderer.activeOperations["active"].State != operations.StateRunning {
		t.Fatalf("active operations = %#v", renderer.activeOperations)
	}
	if renderer.pendingTheme == nil {
		t.Fatal("theme mutation was not applied during drain")
	}
	if got := bindingString(renderer.data, "root.nested.value"); got != "queued" {
		t.Fatalf("nested binding = %q", got)
	}
}

func TestNativeUIConcurrentPostsAreNotDropped(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	ui := newNativeUI(func() {})
	const updates = 200
	applied := make([]int, 0, updates)
	var wait sync.WaitGroup
	wait.Add(updates)
	for index := range updates {
		go func() {
			defer wait.Done()
			ui.post(func(*Renderer) { applied = append(applied, index) })
		}()
	}
	wait.Wait()
	ui.drain(renderer)
	if len(applied) != updates {
		t.Fatalf("applied updates = %d, want %d", len(applied), updates)
	}
}

func TestNativeLifecycleTeardownIsIOSOnly(t *testing.T) {
	if !nativeLifecycleEnabled("ios") {
		t.Fatal("iOS lifecycle events must control transport suspension")
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		if nativeLifecycleEnabled(goos) {
			t.Errorf("desktop platform %s unexpectedly tears down on focus loss", goos)
		}
	}
}

func TestNativeLifecyclePreservesInactiveEdgeAcrossQuickResume(t *testing.T) {
	lifecycle := newNativeLifecycleMailbox()
	lifecycle.Publish(true)
	lifecycle.Publish(false)
	lifecycle.Publish(true)
	<-lifecycle.Wake()
	snapshot := lifecycle.Snapshot()
	if !snapshot.Focused || snapshot.InactiveEpoch != 1 {
		t.Fatalf("lifecycle snapshot = %+v, want focused with one inactive edge", snapshot)
	}
	lifecycle.Publish(false)
	lifecycle.Publish(false)
	snapshot = lifecycle.Snapshot()
	if snapshot.Focused || snapshot.InactiveEpoch != 2 {
		t.Fatalf("duplicate inactive snapshot = %+v, want one additional edge", snapshot)
	}
}
