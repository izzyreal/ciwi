//go:build darwin || linux || windows

package gio

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestScreenRefreshCoordinatorCoalescesAndPrioritizesRequests(t *testing.T) {
	var coordinator screenRefreshCoordinator
	passive := screenRefreshRequest{origin: screenRefreshPassive}
	if !coordinator.request(passive) {
		t.Fatal("first refresh request did not start")
	}
	if coordinator.request(screenRefreshRequest{origin: screenRefreshRetry}) || coordinator.request(screenRefreshRequest{origin: screenRefreshOperation, recoverMissingRoute: true}) {
		t.Fatal("in-flight refresh was superseded instead of coalesced")
	}
	pending, ok := coordinator.complete()
	if !ok || pending.origin != screenRefreshOperation || !pending.recoverMissingRoute {
		t.Fatalf("pending refresh = (%+v, %v)", pending, ok)
	}
	if !coordinator.request(pending) {
		t.Fatal("completed coordinator did not accept its trailing refresh")
	}
	if _, ok := coordinator.complete(); ok {
		t.Fatal("refresh without in-flight invalidations retained a trailing load")
	}
}

func TestScreenRefreshCoordinatorNavigationSupersedesPendingRefresh(t *testing.T) {
	var coordinator screenRefreshCoordinator
	coordinator.request(screenRefreshRequest{origin: screenRefreshPassive})
	coordinator.request(screenRefreshRequest{origin: screenRefreshOperation, recoverMissingRoute: true})
	coordinator.supersede(screenRefreshRequest{origin: screenRefreshNavigation})
	if pending, ok := coordinator.complete(); ok {
		t.Fatalf("superseding navigation retained stale refresh: %+v", pending)
	}
	coordinator.cancel()
	if !coordinator.request(screenRefreshRequest{origin: screenRefreshExplicit}) {
		t.Fatal("cancelled coordinator did not accept a new refresh")
	}
}

func TestScreenRefreshCoordinatorBoundsAndResetsRetry(t *testing.T) {
	var coordinator screenRefreshCoordinator
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	for i, delay := range want {
		if got := coordinator.nextRetry(); got != delay {
			t.Fatalf("retry %d = %s, want %s", i, got, delay)
		}
	}
	coordinator.resetRetry()
	if got := coordinator.nextRetry(); got != time.Second {
		t.Fatalf("retry after reset = %s", got)
	}
}

func TestQuietPassiveRefreshFailureRequiresCachedDeadline(t *testing.T) {
	wrapped := fmt.Errorf("read native response: %w", context.DeadlineExceeded)
	if !quietPassiveRefreshFailure(screenRefreshRequest{origin: screenRefreshPassive}, wrapped, true) {
		t.Fatal("cached passive deadline was not quiet")
	}
	for _, test := range []struct {
		request screenRefreshRequest
		err     error
		cached  bool
	}{
		{screenRefreshRequest{origin: screenRefreshExplicit}, wrapped, true},
		{screenRefreshRequest{origin: screenRefreshPassive}, wrapped, false},
		{screenRefreshRequest{origin: screenRefreshPassive}, context.Canceled, true},
	} {
		if quietPassiveRefreshFailure(test.request, test.err, test.cached) {
			t.Fatalf("unexpected quiet failure for %+v", test)
		}
	}
}
