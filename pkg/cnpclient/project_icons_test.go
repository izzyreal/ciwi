package cnpclient

import "testing"

func TestProjectIconCacheSurvivesClientsAndNamespacesServers(t *testing.T) {
	cache := NewProjectIconCache()
	cache.put("server-a", 7, projectIcon{contentType: "image/png", data: []byte("a"), loadedCommit: "commit-a"})
	icon, found := cache.get("server-a", 7)
	if !found || string(icon.data) != "a" || icon.loadedCommit != "commit-a" {
		t.Fatalf("cached icon = %+v, found=%v", icon, found)
	}
	icon.data[0] = 'x'
	again, _ := cache.get("server-a", 7)
	if string(again.data) != "a" {
		t.Fatal("cache returned mutable icon storage")
	}
	if _, found := cache.get("server-b", 7); found {
		t.Fatal("project ID collided across server installations")
	}
}
