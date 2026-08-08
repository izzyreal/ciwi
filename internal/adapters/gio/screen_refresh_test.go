//go:build darwin || linux || windows

package gio

import "testing"

func TestScreenRefreshCoordinatorCoalescesInvalidations(t *testing.T) {
	var coordinator screenRefreshCoordinator
	if !coordinator.request(false) {
		t.Fatal("first refresh request did not start")
	}
	if coordinator.request(false) || coordinator.request(true) {
		t.Fatal("in-flight refresh was superseded instead of coalesced")
	}
	startTrailing, recoverMissingRoute := coordinator.complete()
	if !startTrailing || !recoverMissingRoute {
		t.Fatalf("trailing refresh = (%v, %v), want (true, true)", startTrailing, recoverMissingRoute)
	}
	if coordinator.request(false) {
		t.Fatal("trailing refresh did not retain the active slot")
	}
	startTrailing, recoverMissingRoute = coordinator.complete()
	if !startTrailing || recoverMissingRoute {
		t.Fatalf("second trailing refresh = (%v, %v), want (true, false)", startTrailing, recoverMissingRoute)
	}
	if start, _ := coordinator.complete(); start {
		t.Fatal("refresh without in-flight invalidations started another load")
	}
	if !coordinator.request(false) {
		t.Fatal("coordinator did not become idle after the final refresh")
	}
}

func TestScreenRefreshCoordinatorNavigationSupersedesPendingRefresh(t *testing.T) {
	var coordinator screenRefreshCoordinator
	if !coordinator.request(false) {
		t.Fatal("initial refresh did not start")
	}
	coordinator.request(true)
	coordinator.supersede()
	if start, recover := coordinator.complete(); start || recover {
		t.Fatalf("superseding navigation retained stale refresh: (%v, %v)", start, recover)
	}
	coordinator.cancel()
	if !coordinator.request(false) {
		t.Fatal("cancelled coordinator did not accept a new refresh")
	}
}
