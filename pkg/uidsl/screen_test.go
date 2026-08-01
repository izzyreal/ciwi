package uidsl

import (
	"strings"
	"testing"
)

const validScreen = `apiVersion: ciwi.ui/v1
kind: Screen
metadata:
  name: front-page
  title: ciwi
screen:
  dataSources:
    - name: frontPage
      query: get-front-page-view
      watchTopics: [server, projects]
  persistence:
    - name: projectsExpanded
      storageKey: ciwi.front-page.projects-expanded.v1
      defaultValue: "true"
      scope: client
  root:
    component: page
    id: front-page
    children:
      - component: text
        id: title
        text:
          template: "ciwi {{frontPage.server.version}}"
      - component: list
        repeat:
          source: frontPage.projects
          as: project
          key: project.id
        children:
          - component: card
            text:
              binding: project.name
            actions:
              - on: activate
                command: navigate
                arguments:
                  route: /projects/{{project.id}}
`

func TestParseScreen(t *testing.T) {
	document, err := ParseScreen([]byte(validScreen))
	if err != nil {
		t.Fatal(err)
	}
	if document.Metadata.Name != "front-page" || document.Screen.Root.Component != "page" {
		t.Fatalf("document = %#v", document)
	}
}

func TestParseScreenRejectsUnknownField(t *testing.T) {
	_, err := ParseScreen([]byte(strings.Replace(validScreen, "  root:\n", "  browserScript: alert(1)\n  root:\n", 1)))
	if err == nil || !strings.Contains(err.Error(), "browserScript") {
		t.Fatalf("ParseScreen() error = %v", err)
	}
}

func TestParseScreenRejectsDuplicateNodeIDs(t *testing.T) {
	payload := strings.Replace(validScreen, "          - component: card\n", "          - component: card\n            id: title\n", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "duplicate node id") {
		t.Fatalf("ParseScreen() error = %v", err)
	}
}

func TestParseScreenRejectsUnknownBindingRoot(t *testing.T) {
	payload := strings.Replace(validScreen, "project.name", "unknown.name", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "unknown root") {
		t.Fatalf("ParseScreen() error = %v", err)
	}
}

func TestRenderTextAndResolve(t *testing.T) {
	data := map[string]any{
		"frontPage": map[string]any{
			"server":   map[string]any{"version": "v0.2.0"},
			"projects": []map[string]any{{"name": "ciwi"}},
		},
	}
	text, err := RenderText(data, Text{Template: "ciwi {{frontPage.server.version}} · {{frontPage.projects.0.name}}"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "ciwi v0.2.0 · ciwi" {
		t.Fatalf("RenderText() = %q", text)
	}
}
