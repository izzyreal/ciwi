package webui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/front-page.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.APIVersion != uidsl.APIVersion || screen.Metadata.Name != "front-page" {
		t.Fatalf("screen = %#v", screen)
	}
}

func TestProjectDetailsDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/project-details.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "project-details" {
		t.Fatalf("screen = %#v", screen)
	}
	if len(screen.Screen.Root.Children) < 2 {
		t.Fatalf("project screen children = %d", len(screen.Screen.Root.Children))
	}
	structure := screen.Screen.Root.Children[1]
	if structure.Component != "graph-view" || structure.GraphView == nil || structure.GraphView.DefaultMode != "graph" {
		t.Fatalf("project structure = %#v", structure)
	}
	if structure.GraphView.Nodes != "projectDetails.pipelines" || structure.GraphView.Dependencies != "pipeline.depends_on" {
		t.Fatalf("project graph binding = %#v", structure.GraphView)
	}
	if len(structure.GraphView.Details) < 2 || structure.GraphView.Details[1].GraphView == nil {
		t.Fatalf("project graph selected-pipeline details = %#v", structure.GraphView.Details)
	}
	if nested := structure.GraphView.Details[1].GraphView; nested.Nodes != "pipeline.jobs" || len(nested.Details) == 0 {
		t.Fatalf("project job graph = %#v", nested)
	}
}

func TestJobDetailsDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/job-details.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "job-details" {
		t.Fatalf("screen = %#v", screen)
	}
}

func TestSettingsDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/settings.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "settings" {
		t.Fatalf("screen = %#v", screen)
	}
}

func TestAgentsDeclarativeScreenAndPreviewRoutes(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/agents.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("contract status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "agents" {
		t.Fatalf("screen = %#v", screen)
	}
	recorder = httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview/agents", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("preview status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectionDeclarativeScreenAndPreviewRoutes(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/connection.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("contract status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "connection" {
		t.Fatalf("screen = %#v", screen)
	}
	recorder = httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview/connection", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("preview status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeclarativePreviewUsesSharedContractRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	page := recorder.Body.String()
	for _, expected := range []string{"declarativeRoot", "/ui/declarative.js", "/ui/css/declarative.css"} {
		if !strings.Contains(page, expected) {
			t.Errorf("preview page does not contain %q", expected)
		}
	}
}

func TestDeclarativeProjectPreviewUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview/projects/7", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeclarativeJobPreviewUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview/jobs/job-1", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeclarativeSettingsPreviewUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview/settings", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, field := range []string{"project.action_status = ''", "project.action_tone = 'muted'"} {
		if !strings.Contains(script, field) {
			t.Errorf("settings preview does not initialize %q", field)
		}
	}
}

func TestDeclarativeJobPreviewUsesIncrementalOutputView(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, expected := range []string{"/api/v1/views/jobs/", "/output?after_event_id=", "maxOutputCharacters"} {
		if !strings.Contains(script, expected) {
			t.Errorf("declarative renderer does not contain %q", expected)
		}
	}
}

func TestDeclarativeRendererSupportsSemanticTonesAndIcons(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, expected := range []string{"semanticTone", "style.toneBinding", "/ui/icons.svg#icon-", "node.component === 'select'", "change-theme", "runSelectionFromArguments"} {
		if !strings.Contains(script, expected) {
			t.Errorf("declarative renderer does not contain %q", expected)
		}
	}
}

func TestDeclarativeRendererUsesSharedVisualMetricsAndDisclosureSummaries(t *testing.T) {
	scriptPayload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	stylePayload, err := uiAssets.ReadFile("assets/css/declarative.css")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptPayload)
	style := string(stylePayload)
	for _, expected := range []string{
		"dimensionVariables", "--ciwi-page-max", "node.disclosure.summary", "dsl-icon-button",
		"element.append(declarativeIcon(node.icon))", ".dsl-disclosure > summary::after", ".dsl-code-inline",
		"--ciwi-text-control", "--ciwi-card-background", ".dsl-badge.dsl-muted",
	} {
		if !strings.Contains(script+style, expected) {
			t.Errorf("declarative renderer does not contain %q", expected)
		}
	}
}

func TestDeclarativeRendererSupportsPersistentInteractiveDefinitionGraphs(t *testing.T) {
	scriptPayload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	stylePayload, err := uiAssets.ReadFile("assets/css/declarative.css")
	if err != nil {
		t.Fatal(err)
	}
	implementation := string(scriptPayload) + string(stylePayload)
	for _, expected := range []string{
		"ciwi.declarative.views.v1", "renderGraphView", "layoutDefinitionGraph", "renderDefinitionGraph",
		"requestAnimationFrame(fit)", "bindActions(play, node.actions, graphNode.data)",
		"node.graphView.details", "selection.onChange(graphNode.id)", "dsl-definition-graph-viewport",
		"dsl-definition-graph-node-play", "dsl-definition-graph-details", ".dsl-definition-graph-node.selectable:hover",
	} {
		if !strings.Contains(implementation, expected) {
			t.Errorf("declarative graph renderer does not contain %q", expected)
		}
	}
}
