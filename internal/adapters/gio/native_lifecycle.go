//go:build darwin || ios || linux || windows

package gio

import (
	"runtime"
	"sync"
)

type nativeLifecycleSnapshot struct {
	Focused       bool
	InactiveEpoch uint64
}

// nativeLifecycleMailbox combines a level (the latest focus state) with an
// edge counter. The counter preserves a brief inactive transition even when
// iOS becomes active again before the controller consumes the notification.
type nativeLifecycleMailbox struct {
	mu sync.Mutex

	initialized   bool
	focused       bool
	inactiveEpoch uint64
	wake          chan struct{}
}

func newNativeLifecycleMailbox() *nativeLifecycleMailbox {
	return &nativeLifecycleMailbox{wake: make(chan struct{}, 1)}
}

func publishNativeLifecycle(lifecycle *nativeLifecycleMailbox, focused bool) {
	if lifecycle == nil || !nativeLifecycleEnabled(runtime.GOOS) {
		return
	}
	lifecycle.Publish(focused)
}

func nativeLifecycleEnabled(goos string) bool { return goos == "ios" }

func (m *nativeLifecycleMailbox) Publish(focused bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.initialized {
		m.initialized = true
		m.focused = focused
		if !focused {
			m.inactiveEpoch++
		}
	} else if m.focused != focused {
		m.focused = focused
		if !focused {
			m.inactiveEpoch++
		}
	}
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *nativeLifecycleMailbox) Wake() <-chan struct{} {
	if m == nil {
		return nil
	}
	return m.wake
}

func (m *nativeLifecycleMailbox) Snapshot() nativeLifecycleSnapshot {
	if m == nil {
		return nativeLifecycleSnapshot{Focused: true}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return nativeLifecycleSnapshot{Focused: m.focused, InactiveEpoch: m.inactiveEpoch}
}
