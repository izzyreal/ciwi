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
	if spec, ok := catalog.Spec("run-pipeline"); !ok || spec.Class != uidsl.ActionClassMutation || spec.Scope == "" || !spec.RefreshOnSuccess {
		t.Fatalf("run-pipeline action = %#v, %v", spec, ok)
	}
	for _, command := range []string{"run-chain", "clear-queue", "remove-execution"} {
		if spec, ok := catalog.Spec(command); !ok || !spec.RefreshOnSuccess {
			t.Errorf("%s must refresh its active view after success: %#v, %v", command, spec, ok)
		}
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
	declarative, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(declarative), "if (runtime.refreshOnSuccess) await refresh({throwOnError: true})") {
		t.Fatal("declarative renderer does not honor shared post-success refresh semantics")
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
	if len(screen.Screen.Root.Children) < 7 {
		t.Fatalf("project screen children = %d", len(screen.Screen.Root.Children))
	}
	loading := screen.Screen.Root.Children[1]
	loadError := screen.Screen.Root.Children[2]
	if loading.ID != "project-details-loading" || loading.Visible == nil || loading.Visible.Binding != "projectDetails.loading" {
		t.Fatalf("project loading shell = %#v", loading)
	}
	loadingBody := loading.Children[2].Children[1].Children[0]
	if loadingBody.Component != "spacer" || loadingBody.Layout.MinHeight != "160" || loadingBody.Style.Role != "" {
		t.Fatalf("project loading body spacer = %#v", loadingBody)
	}
	if loadError.ID != "project-details-load-error" || loadError.Visible == nil || loadError.Visible.Binding != "projectDetails.load_error" {
		t.Fatalf("project load error = %#v", loadError)
	}
	chains := screen.Screen.Root.Children[4]
	if chains.Component != "graph-view" || chains.GraphView == nil || chains.Visible == nil || chains.Visible.Binding != "projectDetails.show_chain_structure" {
		t.Fatalf("project chain structure = %#v", chains)
	}
	if chains.GraphView.Nodes != "projectDetails.project.pipeline_chains" || chains.GraphView.Dependencies != "" || len(chains.Actions) != 1 || chains.Actions[0].Command != "run-chain" {
		t.Fatalf("project chain graph binding = %#v actions=%#v", chains.GraphView, chains.Actions)
	}
	if root := chains.GraphView.Root; root == nil || root.Binding != "projectDetails.structure_root" || len(root.Actions) != 0 {
		t.Fatalf("project chain graph root = %#v", root)
	}
	structure := screen.Screen.Root.Children[5]
	if structure.Component != "graph-view" || structure.GraphView == nil || structure.GraphView.DefaultMode != "graph" {
		t.Fatalf("project structure = %#v", structure)
	}
	if structure.Visible == nil || structure.Visible.Binding != "projectDetails.show_pipeline_structure" {
		t.Fatalf("project pipeline graph visibility = %#v", structure.Visible)
	}
	if structure.GraphView.Nodes != "projectDetails.visible_pipelines" || structure.GraphView.Dependencies != "pipeline.depends_on" {
		t.Fatalf("project graph binding = %#v", structure.GraphView)
	}
	if root := structure.GraphView.Root; root == nil || root.Binding != "projectDetails.structure_root" || len(root.Actions) != 1 || root.Actions[0].Command != "run-chain" {
		t.Fatalf("project graph root = %#v", root)
	}
	filter := screen.Screen.Root.Children[3]
	if webOverride, exists := filter.Overrides["web"]; filter.ID != "project-structure-filter" || (exists && webOverride.Hidden) {
		t.Fatalf("project structure filter is not shared by both renderers: %#v", filter)
	}
	if len(structure.GraphView.Details) < 2 || structure.GraphView.Details[1].GraphView == nil {
		t.Fatalf("project graph selected-pipeline details = %#v", structure.GraphView.Details)
	}
	if nested := structure.GraphView.Details[1].GraphView; nested.Nodes != "pipeline.jobs" || len(nested.Details) == 0 {
		t.Fatalf("project job graph = %#v", nested)
	} else if nested.Root == nil || nested.Root.Binding != "pipeline" || len(nested.Root.Actions) != 1 || nested.Root.Actions[0].Command != "run-pipeline" {
		t.Fatalf("project job graph root = %#v", nested.Root)
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
	runContextGraph := screen.Screen.Root.Children[3].Children[3].Children[2]
	if runContextGraph.GraphView == nil || runContextGraph.GraphView.Root == nil || len(runContextGraph.GraphView.Root.Actions) != 0 {
		t.Fatalf("job run-context root = %#v", runContextGraph.GraphView)
	}
	nested := runContextGraph.GraphView.Details[1].GraphView
	if nested == nil || nested.Root == nil || nested.Root.Binding != "runPipeline" || len(nested.Root.Actions) != 0 {
		t.Fatalf("job pipeline root = %#v", nested)
	}
	back := screen.Screen.Root.Children[0].Children[4]
	if len(back.Actions) != 1 || back.Actions[0].Command != "navigate" || back.Actions[0].Arguments["section"] != "execution-history" {
		t.Fatalf("job back action = %#v", back.Actions)
	}
	output := screen.Screen.Root.Children[4]
	toolbar := output.Children[2]
	if toolbar.Style.Role != "compact-toolbar" || len(toolbar.Children) < 2 || toolbar.Children[0].Actions[0].Command != "download-job-log" || toolbar.Children[0].Actions[0].Arguments["format"] != "clean" || toolbar.Children[1].Actions[0].Arguments["format"] != "raw" {
		t.Fatalf("job output toolbar = %#v", toolbar)
	}
	groups := output.Children[4]
	if groups.ID != "job-output-groups" || groups.Layout.MaxHeight != "660" || groups.Children[0].Children[0].Style.Role != "floating-collapse" {
		t.Fatalf("job output groups = %#v", groups)
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
		"decorateJobDetails(view)",
		"view.project_icon = Number(view.project_id || 0) > 0",
		"data.jobDetails.tailing_tone = data.jobDetails.output_tailing ? 'success' : 'warning'",
		"['running', 'in progress', 'failed'].includes",
		"renderBrowserOutputText", "ciwi-search-hit-active", "updateDeclarativeOutputCollapseButtons",
		"/log?format=", "options.section", "scrollIntoView({block: 'start'})",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("browser job interaction state does not contain %q", expected)
		}
	}
}

func TestDeclarativeBrowserProcessesInitialChangeStreamResync(t *testing.T) {
	scriptPayload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptPayload)
	if !strings.Contains(script, "if (change.resync_required ||") {
		t.Fatal("browser change stream does not refresh for resync markers")
	}
	if strings.Contains(script, "if (!initialized)") {
		t.Fatal("browser still discards the first change event unconditionally")
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

func TestDeclarativeJobOutputRefreshPreservesStreamState(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, expected := range []string{
		"initializeJobOutputView(view, sameJob ? previousJob : null)",
		"previousView.system_output",
		"previousView.output_after_event_id",
		"previousGroups.get(String(group.id || ''))",
		"currentJob.output_after_event_id = nextEventID",
		"if (generation !== outputWatchGeneration) return",
		"if (mergeJobOutputBatch(currentJob, batch))",
		"if (!changed) return false",
		"window.ciwiCaptureViewState(root)",
		"window.ciwiRestoreViewState(root, viewState)",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("browser output refresh does not contain %q", expected)
		}
	}
}

func TestDeclarativeBrowserConsumesAuthoritativePresentationLabels(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, duplicatedRule := range []string{
		"card.summary_label =", "card.summary_tone =", "job.created_label =", "job.duration_label =",
		"pipeline.summary_label =", "pipeline.graph_summary_label =", "job.summary_label =",
		"job.timeout_label =", "job.matrix_label =", "step.environment_label =",
	} {
		if strings.Contains(script, duplicatedRule) {
			t.Errorf("browser still derives authoritative presentation field %q", duplicatedRule)
		}
	}
}

func TestDeclarativeRendererSupportsSemanticTonesAndIcons(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, expected := range []string{"semanticTone", "style.toneBinding", "/ui/icons.svg?v=declarative-4#icon-", "node.component === 'select'", "change-theme", "runSelectionFromArguments"} {
		if !strings.Contains(script, expected) {
			t.Errorf("declarative renderer does not contain %q", expected)
		}
	}
}

func TestThemeBootstrapRecognizesEverySharedTheme(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/theme.js")
	if err != nil {
		t.Fatal(err)
	}
	themes, err := sharedui.LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, theme := range themes {
		quotedName := "'" + theme.Metadata.Name + "'"
		if !strings.Contains(script, quotedName) {
			t.Errorf("theme bootstrap does not recognize %q", theme.Metadata.Name)
		}
		if theme.Theme.Dark {
			darkCatalog := script[strings.Index(script, "const ciwiDarkThemeNames"):]
			if !strings.Contains(darkCatalog, quotedName) {
				t.Errorf("theme bootstrap does not mark %q dark", theme.Metadata.Name)
			}
		}
	}
}

func TestDeclarativeRendererSupportsSharedReportsAndArtifactDownloads(t *testing.T) {
	scriptPayload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	stylePayload, err := uiAssets.ReadFile("assets/css/declarative.css")
	if err != nil {
		t.Fatal(err)
	}
	script, styles := string(scriptPayload), string(stylePayload)
	for _, expected := range []string{
		"renderTreeView", "set-report-filter", "download-artifact",
		"/artifacts/download-all", "/artifacts/download?prefix=", "'/artifacts/'", "anchor.target = '_blank'",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("declarative report renderer does not contain %q", expected)
		}
	}
	for _, expected := range []string{".dsl-report-card", ".dsl-report-stack", ".dsl-tree-children", "margin-left:20px", "overflow-wrap:anywhere", "word-break:break-word"} {
		if !strings.Contains(styles, expected) {
			t.Errorf("declarative report styles do not contain %q", expected)
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
	if !strings.Contains(sprite, `<symbol id="icon-arrow-bar-to-down" viewBox="0 0 24 24"><path d="M4 20l16 0"/>`) {
		t.Error("browser arrow-bar-to-down icon does not use the Tabler bottom-bar geometry")
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
		"element.textContent = ''", "element.prepend(declarativeIcon(node.icon))", ".dsl-disclosure > summary::after", ".dsl-code-inline",
		"--ciwi-text-control", "--ciwi-card-background", ".dsl-badge.dsl-muted", "cssLength(layout.gap)",
		"--dsl-layout-padding", ".dsl-output-group > summary.ciwi-progress-surface", "var(--console-green) 18%",
		"#job-output-groups > * { flex:0 0 auto; }", "overflow-y:auto", ".dsl-output-group:not([open]) > summary",
		"'section-padding': 'var(--ciwi-section-padding)'", ".dsl-output-group > summary { color:var(--console-accent)",
		"element.style.flexBasis = '0'", ".dsl-cache-statistics { white-space:pre-line",
		"if (imageSource)", "if (!imageSource) return document.createDocumentFragment()",
		".dsl-project-row > summary > .dsl-disclosure-label",
		"if (summary) event.preventDefault()", "flex-wrap:wrap", "touch-action:pan-x pan-y", "overscroll-behavior:contain",
		".dsl-project-header-metadata", "overflow-wrap:anywhere",
		".dsl-execution-row:not([open]) > summary.ciwi-progress-surface", "border-radius:var(--ciwi-surface-radius",
		".dsl-execution-section-header", ".dsl-agent-header,.dsl-agent-record { display:grid !important;",
	} {
		if !strings.Contains(script+style, expected) {
			t.Errorf("declarative renderer does not contain %q", expected)
		}
	}
	if strings.Contains(style+chromeCSS, "text-decoration: underline") {
		t.Fatal("browser text hover styling still introduces underlines")
	}
}

func TestDeclarativeNavigationCommitsTargetBeforeRemoteData(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	for _, expected := range []string{
		"navigateBrowser", "window.history.pushState", "window.addEventListener('popstate'", "aria-busy",
		"browserLoadingBinding", "browserViewCache", "loadingCommitted = true", "loadingRoot.load_error", "showLoading: true",
		"settingsProjectsPromise", "settingsUpdateStatusPromise",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("declarative navigation does not contain %q", expected)
		}
	}
	loadingRender := strings.Index(script, "loadingCommitted = true")
	remoteValidation := strings.Index(script, "if (!viewResponse.ok)")
	if loadingRender < 0 || remoteValidation < 0 || loadingRender > remoteValidation {
		t.Fatal("target loading shell is not committed before the remote response is validated")
	}
	if strings.Contains(script, "window.location.assign") {
		t.Fatal("declarative navigation still performs full document reloads")
	}
}

func TestDeclarativeSettingsUsesRESTProjectUpdateTimestamp(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	updatedUTC := strings.Index(script, "project.updated_utc")
	updatedMilliseconds := strings.Index(script, "project.updated_unix_ms")
	if updatedUTC < 0 || updatedMilliseconds < 0 || updatedUTC > updatedMilliseconds {
		t.Fatal("Settings does not prefer the REST updated_utc field with an updated_unix_ms fallback")
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
		"requestAnimationFrame(fit)", "bindActions(play, actions, graphNode.data)",
		"node.graphView.details", "selection.onChange(graphNode.id)", "dsl-definition-graph-viewport",
		"dsl-definition-graph-node-play", "dsl-definition-graph-details", ".dsl-definition-graph-node.selectable:hover",
		"applyProjectStructureFilter", "projectStructureFilterOptions", "dsl-definition-graph-root", "graphRootActionVisible",
	} {
		if !strings.Contains(implementation, expected) {
			t.Errorf("declarative graph renderer does not contain %q", expected)
		}
	}
}
