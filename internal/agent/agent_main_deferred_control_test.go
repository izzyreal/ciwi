package agent

import (
	"reflect"
	"testing"
)

func TestDeferredControlRequestsAndFlushOrdering(t *testing.T) {
	c := &deferredControl{}
	events := make([]string, 0)
	runUpdate := func(target, repo, api string) {
		events = append(events, "update:"+target+"|"+repo+"|"+api)
	}
	runRestart := func() { events = append(events, "restart") }
	runCache := func() { events = append(events, "cache") }
	runHistory := func() { events = append(events, "history") }

	// Immediate execution when no job is in progress.
	c.requestUpdate("v1.2.3", "repo", "api", nil, runUpdate)
	c.requestRestart(nil, runRestart)
	c.requestCacheWipe(nil, runCache)
	c.requestJobHistoryWipe(nil, runHistory)
	want := []string{"update:v1.2.3|repo|api", "restart", "cache", "history"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("unexpected immediate events: got=%v want=%v", events, want)
	}

	// While busy, actions should defer (and only latest update request should survive).
	events = events[:0]
	c.setJobInProgress(true)
	deferred := make([]string, 0)
	c.requestUpdate("v2.0.0", "repoA", "apiA", func() { deferred = append(deferred, "defer-update-a") }, runUpdate)
	c.requestUpdate("v2.0.1", "repoB", "apiB", func() { deferred = append(deferred, "defer-update-b") }, runUpdate)
	c.requestRestart(func() { deferred = append(deferred, "defer-restart") }, runRestart)
	c.requestCacheWipe(func() { deferred = append(deferred, "defer-cache") }, runCache)
	c.requestJobHistoryWipe(func() { deferred = append(deferred, "defer-history") }, runHistory)
	if got := len(events); got != 0 {
		t.Fatalf("expected no immediate events while busy, got %v", events)
	}
	wantDeferred := []string{"defer-update-a", "defer-update-b", "defer-restart", "defer-cache", "defer-history"}
	if !reflect.DeepEqual(deferred, wantDeferred) {
		t.Fatalf("unexpected deferred markers: got=%v want=%v", deferred, wantDeferred)
	}

	c.flushDeferred(runUpdate, runRestart, runCache, runHistory)
	want = []string{"update:v2.0.1|repoB|apiB", "restart", "cache", "history"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("unexpected flush order/events: got=%v want=%v", events, want)
	}
	if c.jobInProgress || c.pendingUpdate != nil || c.pendingRestart || c.pendingCacheWipe || c.pendingJobHistoryWipe {
		t.Fatalf("expected deferred state to be fully cleared after flush: %+v", c)
	}
}

func TestDeferredControlIgnoresEmptyUpdateTarget(t *testing.T) {
	c := &deferredControl{}
	c.setJobInProgress(true)
	c.requestUpdate("   ", "repo", "api", nil, func(string, string, string) {
		t.Fatalf("runNow should not be called for empty update target")
	})
	if c.pendingUpdate != nil {
		t.Fatalf("expected empty target to not be queued")
	}
}
