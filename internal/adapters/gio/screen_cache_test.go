//go:build darwin || ios || linux || windows

package gio

import "testing"

func TestNativeScreenCacheUsesStaleDataWhileKeepingEntriesIsolated(t *testing.T) {
	cache := newNativeScreenCache()
	cache.SetServerInstallationID("server-a")
	front := navigationState{screen: "front-page"}
	original := map[string]any{"frontPage": map[string]any{
		"projects": []any{map[string]any{"name": "ciwi"}},
	}}
	cache.Put(front, original)

	// Neither later mutations of the live renderer data nor mutations of a
	// returned cached snapshot may alter the retained stale view.
	original["frontPage"].(map[string]any)["projects"].([]any)[0].(map[string]any)["name"] = "changed-live"
	first, ok := cache.Get(front)
	if !ok {
		t.Fatal("front-page cache entry was not retained")
	}
	firstProject := first["frontPage"].(map[string]any)["projects"].([]any)[0].(map[string]any)
	if firstProject["name"] != "ciwi" {
		t.Fatalf("cached project name = %q, want ciwi", firstProject["name"])
	}
	firstProject["name"] = "changed-snapshot"
	second, _ := cache.Get(front)
	if got := second["frontPage"].(map[string]any)["projects"].([]any)[0].(map[string]any)["name"]; got != "ciwi" {
		t.Fatalf("retained cache was mutated through Get: %q", got)
	}
}

func TestNativeScreenCacheScopesEntriesByRouteAndServerIdentity(t *testing.T) {
	cache := newNativeScreenCache()
	cache.SetServerInstallationID("server-a")
	projectOne := navigationState{screen: "project-details", projectID: 1}
	projectTwo := navigationState{screen: "project-details", projectID: 2}
	cache.Put(projectOne, map[string]any{"projectDetails": map[string]any{"id": int64(1)}})
	cache.Put(projectTwo, map[string]any{"projectDetails": map[string]any{"id": int64(2)}})
	if first, _ := cache.Get(projectOne); first["projectDetails"].(map[string]any)["id"] != int64(1) {
		t.Fatalf("project 1 cache = %#v", first)
	}
	if second, _ := cache.Get(projectTwo); second["projectDetails"].(map[string]any)["id"] != int64(2) {
		t.Fatalf("project 2 cache = %#v", second)
	}
	if !cache.SetServerInstallationID("server-b") {
		t.Fatal("server identity change was not reported")
	}
	if _, ok := cache.Get(projectOne); ok {
		t.Fatal("cache survived a server identity change")
	}
}

func TestNativeScreenCacheExcludesFormsAndIncompleteDetailRoutes(t *testing.T) {
	cache := newNativeScreenCache()
	cache.SetServerInstallationID("server-a")
	tests := []navigationState{
		{screen: "run-options", pipelineDBID: 1},
		{screen: "managed-yaml", projectID: 1},
		{screen: "agent-script", agentScriptID: "agent-1"},
		{screen: "project-details"},
		{screen: "job-details"},
		{screen: "agent-details"},
	}
	for _, navigation := range tests {
		cache.Put(navigation, map[string]any{"unexpected": true})
		if cache.Has(navigation) {
			t.Errorf("%+v unexpectedly became cacheable", navigation)
		}
	}
}

func TestNativeScreenCacheCanPatchAVisibleJobAfterCancellation(t *testing.T) {
	cache := newNativeScreenCache()
	cache.SetServerInstallationID("server-a")
	navigation := navigationState{screen: "job-details", jobID: "job-1"}
	cache.Put(navigation, map[string]any{"jobDetails": map[string]any{"can_cancel": true}})
	cache.SetRootBinding(navigation, "jobDetails", "can_cancel", false)
	data, _ := cache.Get(navigation)
	if data["jobDetails"].(map[string]any)["can_cancel"] != false {
		t.Fatalf("patched cache = %#v", data)
	}
}
