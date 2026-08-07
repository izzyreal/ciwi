package webui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/pkg/uidsl"
	sharedui "github.com/izzyreal/ciwi/ui"
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

func TestDeclarativeRouteContract(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/routes.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var routes uidsl.RouteDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &routes); err != nil {
		t.Fatal(err)
	}
	if match, ok := routes.Match("/projects/7", "web"); !ok || match.Route.Screen != "project-details" {
		t.Fatalf("project route match = %#v, %v", match, ok)
	}
}

func TestEveryWebCommandHasABrowserAdapter(t *testing.T) {
	scriptPayload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	routes, err := sharedui.LoadRoutes()
	if err != nil {
		t.Fatal(err)
	}
	commands := map[string]bool{}
	var visit func(uidsl.Node)
	visit = func(node uidsl.Node) {
		if override, exists := node.Overrides["web"]; exists && override.Hidden {
			return
		}
		for _, action := range node.Actions {
			commands[action.Command] = true
		}
		for _, child := range node.Children {
			visit(child)
		}
		if node.Disclosure != nil {
			for _, child := range node.Disclosure.Summary {
				visit(child)
			}
		}
		if node.GraphView != nil {
			for _, child := range node.GraphView.Details {
				visit(child)
			}
		}
	}
	loaded := map[string]bool{}
	for _, route := range routes.Routes {
		webRoute := false
		for _, platform := range route.Platforms {
			webRoute = webRoute || platform == "web"
		}
		if !webRoute || loaded[route.Screen] {
			continue
		}
		loaded[route.Screen] = true
		screen, loadErr := sharedui.LoadScreen(route.Screen)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		visit(screen.Screen.Root)
	}
	script := string(scriptPayload)
	for command := range commands {
		if !strings.Contains(script, "'"+command+"'") {
			t.Errorf("web-visible command %q has no browser adapter", command)
		}
	}
}

func TestActionCatalogContractAndBrowserRunnerRoutes(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/actions.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("contract status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var catalog uidsl.ActionCatalogDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if spec, ok := catalog.Spec("run-pipeline"); !ok || spec.Class != uidsl.ActionClassMutation || spec.Scope == "" {
		t.Fatalf("run-pipeline action = %#v, %v", spec, ok)
	}

	recorder = httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/actions.js", nil))
	if recorder.Code != 200 {
		t.Fatalf("runner status = %d: %s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{"ciwiRunAction", "Idempotency-Key", "activeByFingerprint", "activeByScope"} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Errorf("browser action runner does not contain %q", expected)
		}
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
	if len(screen.Screen.Root.Children) < 3 {
		t.Fatalf("project screen children = %d", len(screen.Screen.Root.Children))
	}
	structure := screen.Screen.Root.Children[2]
	if structure.Component != "graph-view" || structure.GraphView == nil || structure.GraphView.DefaultMode != "graph" {
		t.Fatalf("project structure = %#v", structure)
	}
	if structure.GraphView.Nodes != "projectDetails.visible_pipelines" || structure.GraphView.Dependencies != "pipeline.depends_on" {
		t.Fatalf("project graph binding = %#v", structure.GraphView)
	}
	filter := screen.Screen.Root.Children[1]
	if webOverride, exists := filter.Overrides["web"]; filter.ID != "project-structure-filter" || (exists && webOverride.Hidden) {
		t.Fatalf("project structure filter is not shared by both renderers: %#v", filter)
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

func TestDeclarativeBrowserPreservesJobInteractionState(t *testing.T) {
	scriptPayload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptPayload)
	for _, expected := range []string{
		"sameJob ? String(previousJob.output_search || '') : ''",
		"sameJob ? !!previousJob.output_tailing : true",
		"data.jobDetails.tailing_tone = data.jobDetails.output_tailing ? 'success' : 'warning'",
		"['running', 'in progress', 'failed'].includes",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("browser job interaction state does not contain %q", expected)
		}
	}
}

func TestDeclarativeBrowserManagedYAMLEditorUsesCorrectCreatePayloadAndFullWidth(t *testing.T) {
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
		"? {revision: args.revision || '', yaml: args.yaml || ''}",
		": {yaml: args.yaml || ''}",
		"element.style.minHeight = String(minimumLines * 24) + 'px'",
		"textarea.dsl-input { width:100%; resize:vertical; }",
	} {
		if !strings.Contains(implementation, expected) {
			t.Errorf("managed YAML browser editor does not contain %q", expected)
		}
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

func TestAgentDetailsDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/agent-details.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "agent-details" {
		t.Fatalf("screen = %#v", screen)
	}
}

func TestVaultDeclarativeScreenContractRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/ui/contracts/screens/vault.json", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var screen uidsl.ScreenDocument
	if err := json.Unmarshal(recorder.Body.Bytes(), &screen); err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "vault" || len(screen.Screen.DataSources) != 1 || screen.Screen.DataSources[0].WatchTopics[0] != "vault" {
		t.Fatalf("screen = %#v", screen)
	}
}

func TestAgentsDeclarativeScreenAndPublicRoute(t *testing.T) {
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
	Handler(recorder, httptest.NewRequest("GET", "/agents", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("public route status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestConnectionDeclarativeScreenIsNativeOnly(t *testing.T) {
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
	Handler(recorder, httptest.NewRequest("GET", "/connection", nil))
	if recorder.Code != 404 {
		t.Fatalf("web connection status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestDeclarativePreviewIsRemoved(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/declarative-preview", nil))
	if recorder.Code != 404 {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestPublicProjectRouteUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/projects/7", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicJobRouteUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/jobs/job-1", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicSettingsRouteUsesSharedRenderer(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest("GET", "/settings", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "declarativeRoot") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, field := range []string{
		"project.is_managed =", "project.has_repo =", "project.repo_ref_label =",
		"project.action_status = ''", "project.action_tone = 'muted'",
	} {
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
	for _, expected := range []string{"semanticTone", "style.toneBinding", "/ui/icons.svg?v=declarative-2#icon-", "node.component === 'select'", "change-theme", "runSelectionFromArguments"} {
		if !strings.Contains(script, expected) {
			t.Errorf("declarative renderer does not contain %q", expected)
		}
	}
}

func TestDeclarativeScreenIconsExistInBrowserSprite(t *testing.T) {
	spritePayload, err := uiAssets.ReadFile("assets/tabler-icons.svg")
	if err != nil {
		t.Fatal(err)
	}
	routes, err := sharedui.LoadRoutes()
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{
		"circle-check": true, "circle-x": true, "clock": true, "loader-2": true,
	}
	var visit func(uidsl.Node)
	visit = func(node uidsl.Node) {
		if node.Icon != "" {
			required[node.Icon] = true
		}
		if node.Disclosure != nil {
			for _, child := range node.Disclosure.Summary {
				visit(child)
			}
		}
		if node.GraphView != nil {
			for _, child := range node.GraphView.Details {
				visit(child)
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	loaded := map[string]bool{}
	for _, route := range routes.Routes {
		if loaded[route.Screen] {
			continue
		}
		loaded[route.Screen] = true
		screen, loadErr := sharedui.LoadScreen(route.Screen)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		visit(screen.Screen.Root)
	}
	sprite := string(spritePayload)
	for icon := range required {
		if !strings.Contains(sprite, `id="icon-`+icon+`"`) {
			t.Errorf("browser icon sprite is missing declarative icon %q", icon)
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
		"element.prepend(declarativeIcon(node.icon))", ".dsl-disclosure > summary::after", ".dsl-code-inline",
		"--ciwi-text-control", "--ciwi-card-background", ".dsl-badge.dsl-muted", "cssLength(layout.gap)",
		"if (imageSource)", "if (!imageSource) return document.createDocumentFragment()",
		".dsl-project-row > summary > .dsl-disclosure-label",
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
		"applyProjectStructureFilter", "projectStructureFilterOptions",
	} {
		if !strings.Contains(implementation, expected) {
			t.Errorf("declarative graph renderer does not contain %q", expected)
		}
	}
}
