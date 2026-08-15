//go:build darwin || ios || linux || windows

package gio

import (
	"sync"
	"time"

	"github.com/izzyreal/ciwi/internal/presentation/operations"
	"github.com/izzyreal/ciwi/pkg/uidsl"
)

// nativeRenderer is the controller-facing one-way rendering sink. Calls report
// no synchronous renderer result; nativeUI queues them for the Gio event owner.
type nativeRenderer interface {
	ApplyJobOutput(jobOutputSnapshot)
	ScrollToSection(string)
	SetDataBinding(string, any)
	SetNestedBinding(string, string, string, any)
	SetOperations([]operations.Operation)
	SetProjectStructureFilter(string)
	SetRootBinding(string, string, any)
	SetScreenAndData(*uidsl.ScreenDocument, any)
	SetTheme(*uidsl.ThemeDocument)
	ShowAlert(string, string)
	ShowNotice(string, string, uidsl.Action, map[string]string, time.Duration)
}

// nativeUI is a mailbox for renderer mutations. The controller owns network
// and navigation state; only the Gio event goroutine drains this mailbox and
// touches Renderer or its DOM runtime.
type nativeUI struct {
	invalidate func()

	mu      sync.Mutex
	pending []func(*Renderer)
}

func newNativeUI(invalidate func()) *nativeUI {
	return &nativeUI{invalidate: invalidate}
}

func (u *nativeUI) post(update func(*Renderer)) {
	if update == nil {
		return
	}
	u.mu.Lock()
	u.pending = append(u.pending, update)
	u.mu.Unlock()
	if u.invalidate != nil {
		u.invalidate()
	}
}

// drain is called only by the Gio event goroutine, immediately before layout.
func (u *nativeUI) drain(renderer *Renderer) {
	u.mu.Lock()
	pending := u.pending
	u.pending = nil
	u.mu.Unlock()
	for _, update := range pending {
		update(renderer)
	}
}

func (u *nativeUI) ApplyJobOutput(snapshot jobOutputSnapshot) {
	snapshot.Outputs = cloneStringMap(snapshot.Outputs)
	snapshot.Errors = cloneStringMap(snapshot.Errors)
	snapshot.ExitCodes = cloneStringMap(snapshot.ExitCodes)
	u.post(func(renderer *Renderer) { renderer.ApplyJobOutput(snapshot) })
}

func (u *nativeUI) ScrollToSection(section string) {
	u.post(func(renderer *Renderer) { renderer.ScrollToSection(section) })
}

func (u *nativeUI) SetDataBinding(key string, value any) {
	value = cloneBindingValue(value)
	u.post(func(renderer *Renderer) { renderer.SetDataBinding(key, value) })
}

func (u *nativeUI) SetNestedBinding(root, objectKey, key string, value any) {
	value = cloneBindingValue(value)
	u.post(func(renderer *Renderer) { renderer.SetNestedBinding(root, objectKey, key, value) })
}

func (u *nativeUI) SetOperations(snapshot []operations.Operation) {
	snapshot = append([]operations.Operation(nil), snapshot...)
	u.post(func(renderer *Renderer) { renderer.SetOperations(snapshot) })
}

func (u *nativeUI) SetProjectStructureFilter(filter string) {
	u.post(func(renderer *Renderer) {
		if !renderer.SetProjectStructureFilter(filter) {
			renderer.ShowAlert("Project structure unavailable", "The selected project structure filter is unavailable.")
		}
	})
}

func (u *nativeUI) SetRootBinding(root, key string, value any) {
	value = cloneBindingValue(value)
	u.post(func(renderer *Renderer) { renderer.SetRootBinding(root, key, value) })
}

func (u *nativeUI) SetScreenAndData(screen *uidsl.ScreenDocument, data any) {
	data = cloneBindingValue(data)
	u.post(func(renderer *Renderer) { renderer.SetScreenAndData(screen, data) })
}

func (u *nativeUI) SetTheme(theme *uidsl.ThemeDocument) {
	// Embedded theme documents have already passed parsing and validation.
	u.post(func(renderer *Renderer) {
		if err := renderer.SetTheme(theme); err != nil {
			renderer.ShowAlert("Theme change failed", err.Error())
		}
	})
}

func (u *nativeUI) ShowAlert(title, message string) {
	u.post(func(renderer *Renderer) { renderer.ShowAlert(title, message) })
}

func (u *nativeUI) ShowNotice(message, actionLabel string, action uidsl.Action, arguments map[string]string, duration time.Duration) {
	arguments = cloneStringMap(arguments)
	u.post(func(renderer *Renderer) { renderer.ShowNotice(message, actionLabel, action, arguments, duration) })
}
