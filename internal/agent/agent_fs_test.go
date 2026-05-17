package agent

import (
	"errors"
	"testing"
	"time"
)

func TestRemoveAllWithRetryNonWindowsDelegatesDirectly(t *testing.T) {
	origGOOS := removeAllWithRetryGOOSFn
	origRemove := removeAllWithRetryFn
	origSleep := removeAllWithRetrySleepFn
	t.Cleanup(func() {
		removeAllWithRetryGOOSFn = origGOOS
		removeAllWithRetryFn = origRemove
		removeAllWithRetrySleepFn = origSleep
	})

	removeAllWithRetryGOOSFn = func() string { return "linux" }
	called := 0
	removeAllWithRetryFn = func(path string) error {
		called++
		if path != "/tmp/ciwi-remove-me" {
			t.Fatalf("unexpected remove path: %q", path)
		}
		return nil
	}
	removeAllWithRetrySleepFn = func(time.Duration) {
		t.Fatalf("sleep should not be used outside windows retry path")
	}

	if err := removeAllWithRetry("/tmp/ciwi-remove-me"); err != nil {
		t.Fatalf("removeAllWithRetry: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected one direct remove call, got %d", called)
	}
}

func TestRemoveAllWithRetryWindowsRetriesAndSucceeds(t *testing.T) {
	origGOOS := removeAllWithRetryGOOSFn
	origRemove := removeAllWithRetryFn
	origSleep := removeAllWithRetrySleepFn
	t.Cleanup(func() {
		removeAllWithRetryGOOSFn = origGOOS
		removeAllWithRetryFn = origRemove
		removeAllWithRetrySleepFn = origSleep
	})

	removeAllWithRetryGOOSFn = func() string { return "windows" }
	attempts := 0
	removeAllWithRetryFn = func(string) error {
		attempts++
		if attempts < 3 {
			return errors.New("sharing violation")
		}
		return nil
	}
	sleeps := 0
	removeAllWithRetrySleepFn = func(d time.Duration) {
		sleeps++
		if d != 500*time.Millisecond {
			t.Fatalf("unexpected sleep duration: %s", d)
		}
	}

	if err := removeAllWithRetry("C:/ciwi/cache"); err != nil {
		t.Fatalf("removeAllWithRetry: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 remove attempts, got %d", attempts)
	}
	if sleeps != 2 {
		t.Fatalf("expected 2 sleeps between failed attempts, got %d", sleeps)
	}
}

func TestRemoveAllWithRetryWindowsReturnsLastErrorAfterRetries(t *testing.T) {
	origGOOS := removeAllWithRetryGOOSFn
	origRemove := removeAllWithRetryFn
	origSleep := removeAllWithRetrySleepFn
	t.Cleanup(func() {
		removeAllWithRetryGOOSFn = origGOOS
		removeAllWithRetryFn = origRemove
		removeAllWithRetrySleepFn = origSleep
	})

	removeAllWithRetryGOOSFn = func() string { return "windows" }
	lastErr := errors.New("access denied")
	attempts := 0
	removeAllWithRetryFn = func(string) error {
		attempts++
		return lastErr
	}
	sleeps := 0
	removeAllWithRetrySleepFn = func(time.Duration) { sleeps++ }

	err := removeAllWithRetry("C:/ciwi/cache")
	if err == nil || !errors.Is(err, lastErr) {
		t.Fatalf("expected wrapped last error, got %v", err)
	}
	if attempts != 8 {
		t.Fatalf("expected 8 remove attempts, got %d", attempts)
	}
	if sleeps != 7 {
		t.Fatalf("expected 7 sleeps before final failure, got %d", sleeps)
	}
}
