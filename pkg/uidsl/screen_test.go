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

func TestParseScreenValidatesDynamicToneBinding(t *testing.T) {
	payload := strings.Replace(validScreen, "          - component: card\n", "          - component: card\n            style:\n              toneBinding: project.status\n", 1)
	if _, err := ParseScreen([]byte(payload)); err != nil {
		t.Fatalf("valid tone binding: %v", err)
	}
	payload = strings.Replace(payload, "project.status", "unknown.status", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "unknown root") {
		t.Fatalf("invalid tone binding error = %v", err)
	}
}

func TestParseScreenValidatesSelectBindings(t *testing.T) {
	payload := strings.Replace(validScreen, "      - component: list\n", `      - component: select
        select:
          value: frontPage.server.version
          options: frontPage.projects
          as: projectOption
          optionValue: projectOption.id
          optionLabel: projectOption.name
        actions:
          - on: change
            command: toggle
            arguments:
              value: "{{selection.value}}"
      - component: list
`, 1)
	if _, err := ParseScreen([]byte(payload)); err != nil {
		t.Fatalf("valid select: %v", err)
	}
	payload = strings.Replace(payload, "projectOption.name", "missing.name", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "unknown root") {
		t.Fatalf("invalid select label error = %v", err)
	}
}

func TestParseScreenValidatesInputBindingAndChangeScope(t *testing.T) {
	payload := strings.Replace(validScreen, "      - component: list\n", `      - component: input
        input:
          value: frontPage.server.version
          placeholder: Search output
        actions:
          - on: change
            command: change-output-search
            arguments:
              query: "{{input.value}}"
      - component: list
`, 1)
	if _, err := ParseScreen([]byte(payload)); err != nil {
		t.Fatalf("valid input: %v", err)
	}
	payload = strings.Replace(payload, "frontPage.server.version", "missing.value", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "unknown root") {
		t.Fatalf("invalid input binding error = %v", err)
	}
}

func TestParseScreenValidatesDisclosureStateKey(t *testing.T) {
	payload := strings.Replace(validScreen, "          - component: card\n", `          - component: disclosure
            disclosure:
              defaultExpanded: true
              stateKey: "front-project:{{project.id}}"
`, 1)
	if _, err := ParseScreen([]byte(payload)); err != nil {
		t.Fatalf("valid disclosure: %v", err)
	}
	payload = strings.Replace(payload, "project.id", "missing.id", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "unknown root") {
		t.Fatalf("invalid disclosure state key error = %v", err)
	}
}

func TestParseScreenValidatesDisclosureSummary(t *testing.T) {
	payload := strings.Replace(validScreen, "          - component: card\n", `          - component: disclosure
            disclosure:
              summary:
                - component: badge
                  text:
                    binding: project.pipeline_count_label
`, 1)
	if _, err := ParseScreen([]byte(payload)); err != nil {
		t.Fatalf("valid disclosure summary: %v", err)
	}
	payload = strings.Replace(payload, "project.pipeline_count_label", "missing.pipeline_count_label", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "unknown root") {
		t.Fatalf("invalid disclosure summary error = %v", err)
	}
}

func TestParseScreenValidatesGraphViewBindingsAndNodeActionScope(t *testing.T) {
	payload := strings.Replace(validScreen, "      - component: list\n", `      - component: graph-view
        graphView:
          stateKey: "project-structure:{{frontPage.server.version}}"
          defaultMode: graph
          nodes: frontPage.projects
          as: graphProject
          nodeKey: graphProject.id
          nodeLabel:
            binding: graphProject.name
          nodeMeta:
            literal: Project
          dependencies: graphProject.pipeline_ids
        actions:
          - on: activate
            command: navigate
            arguments:
              route: /projects/{{graphProject.id}}
        children:
          - component: text
            text:
              literal: List fallback
      - component: list
`, 1)
	if _, err := ParseScreen([]byte(payload)); err != nil {
		t.Fatalf("valid graph view: %v", err)
	}
	payload = strings.Replace(payload, "graphProject.pipeline_ids", "missing.pipeline_ids", 1)
	_, err := ParseScreen([]byte(payload))
	if err == nil || !strings.Contains(err.Error(), "unknown root") {
		t.Fatalf("invalid graph dependency error = %v", err)
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

func TestResolveUsesJSONNamesForStructInputs(t *testing.T) {
	value, err := Resolve(struct {
		Project struct {
			Name string `json:"name"`
		} `json:"project"`
	}{Project: struct {
		Name string `json:"name"`
	}{Name: "ciwi"}}, "project.name")
	if err != nil {
		t.Fatal(err)
	}
	if value != "ciwi" {
		t.Fatalf("value = %#v", value)
	}
}
