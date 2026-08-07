package uidsl

import "testing"

func TestRouteDocumentMatchesPlatformRoutes(t *testing.T) {
	document, err := ParseRoutes([]byte(`apiVersion: ciwi.ui/v1
kind: Routes
routes:
  - {name: agent-script, pattern: "/agents/{agentId}/script", screen: agent-script, bindingRoot: agentScript, platforms: [web, gio]}
  - {name: connection, pattern: /connection, screen: connection, bindingRoot: connection, platforms: [gio]}
`))
	if err != nil {
		t.Fatal(err)
	}
	match, ok := document.Match("/agents/worker-1/script/", "web")
	if !ok || match.Route.Screen != "agent-script" || match.Params["agentId"] != "worker-1" {
		t.Fatalf("match = %#v, %v", match, ok)
	}
	if _, ok := document.Match("/connection", "web"); ok {
		t.Fatal("native-only connection route matched the browser")
	}
}

func TestRouteDocumentRejectsInvalidContracts(t *testing.T) {
	for _, payload := range []string{
		"apiVersion: ciwi.ui/v1\nkind: Routes\nroutes: []\n",
		"apiVersion: ciwi.ui/v1\nkind: Routes\nroutes: [{name: home, pattern: home, screen: home, bindingRoot: home, platforms: [web]}]\n",
		"apiVersion: ciwi.ui/v1\nkind: Routes\nroutes: [{name: home, pattern: /, screen: home, bindingRoot: home, platforms: [browser]}]\n",
	} {
		if _, err := ParseRoutes([]byte(payload)); err == nil {
			t.Fatalf("ParseRoutes(%q) succeeded", payload)
		}
	}
}
