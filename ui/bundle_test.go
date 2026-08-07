package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/izzyreal/ciwi/pkg/uidsl"
)

func TestEmbeddedUIBundle(t *testing.T) {
	routes, err := LoadRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if len(routes.Routes) == 0 {
		t.Fatal("embedded route catalog is empty")
	}
	screen, err := LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	if screen.Metadata.Name != "front-page" {
		t.Fatalf("screen name = %q", screen.Metadata.Name)
	}
	projectScreen, err := LoadScreen("project-details")
	if err != nil {
		t.Fatal(err)
	}
	if projectScreen.Metadata.Name != "project-details" {
		t.Fatalf("project screen name = %q", projectScreen.Metadata.Name)
	}
	jobScreen, err := LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	if jobScreen.Metadata.Name != "job-details" {
		t.Fatalf("job screen name = %q", jobScreen.Metadata.Name)
	}
	settingsScreen, err := LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	if settingsScreen.Metadata.Name != "settings" {
		t.Fatalf("settings screen name = %q", settingsScreen.Metadata.Name)
	}
	runOptionsScreen, err := LoadScreen("run-options")
	if err != nil {
		t.Fatal(err)
	}
	if runOptionsScreen.Metadata.Name != "run-options" {
		t.Fatalf("run options screen name = %q", runOptionsScreen.Metadata.Name)
	}
	for _, name := range []string{"agents", "connection"} {
		screen, err := LoadScreen(name)
		if err != nil {
			t.Fatal(err)
		}
		if screen.Metadata.Name != name {
			t.Fatalf("%s screen name = %q", name, screen.Metadata.Name)
		}
	}
	themes, err := LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	typography, err := LoadTypography()
	if err != nil {
		t.Fatal(err)
	}
	if got := typography.Typography.Weights["regular"].Native; got != 450 {
		t.Fatalf("native regular typography weight = %d, want 450", got)
	}
	if got := typography.Typography.Roles["output-meta"].Family; got != "mono" {
		t.Fatalf("output metadata family = %q, want mono", got)
	}
	if got := typography.Typography.Roles["badge"].Weight; got != "regular" {
		t.Fatalf("default badge weight = %q, want regular", got)
	}
	if got := typography.Typography.Roles["empty-state"].Size; got != 13 {
		t.Fatalf("empty-state size = %v, want browser size 13", got)
	}
	if len(themes) != 9 {
		t.Fatalf("theme count = %d, want 9", len(themes))
	}
	for _, theme := range themes {
		for _, token := range []string{
			"background-start", "background-end", "background-glow-a", "background-glow-b",
			"surface-raised", "surface-glow", "pill-background", "pill-text",
			"notice-background", "notice-text", "notice-border",
			"awaiting-surface", "awaiting-border", "awaiting-text",
			"console-background", "console-surface", "console-border", "console-text", "console-muted", "console-accent", "console-success",
		} {
			if theme.Theme.Colors[token] == "" {
				t.Errorf("theme %q is missing shared visual color %q", theme.Metadata.Name, token)
			}
		}
		for _, token := range []string{
			"small", "medium", "large", "page", "page-inset", "section-padding", "card-padding", "hero-padding",
			"surface-radius", "control-radius", "control-padding-x", "control-padding-y",
			"text-body", "text-control", "text-code", "text-badge", "text-subtitle", "text-heading", "text-title",
			"image-brand-width", "image-brand-height",
		} {
			if theme.Theme.Dimensions[token] == "" {
				t.Errorf("theme %q is missing shared visual metric %q", theme.Metadata.Name, token)
			}
		}
	}
	logo, err := Read("assets/ciwi-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(logo, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("embedded ciwi logo is not a PNG")
	}
	for _, name := range []string{"GeistMono-Regular.ttf", "GeistMono-Medium.ttf", "GeistMono-Bold.ttf"} {
		fontData, err := Read("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if len(fontData) < 4 || string(fontData[:4]) != "\x00\x01\x00\x00" {
			t.Fatalf("embedded %s is not a TrueType font", name)
		}
	}
}

func TestJobDetailsSchedulingUsesAwaitingDesign(t *testing.T) {
	screen, err := LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	var schedulingCard *uidsl.Node
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.Component == "card" && node.Style.Role == "scheduling-awaiting" {
			schedulingCard = node
		}
	})
	if schedulingCard == nil {
		t.Fatal("job details screen has no scheduling-awaiting card")
	}
	tones := map[string]bool{}
	walkNodes(*schedulingCard, func(node *uidsl.Node) {
		if node.Style.Tone != "" {
			tones[node.Style.Tone] = true
		}
	})
	if len(tones) != 1 || !tones["awaiting"] {
		t.Fatalf("scheduling card tones = %v, want awaiting only", tones)
	}
}

func TestSettingsHeaderMatchesAuthoritativeNavigation(t *testing.T) {
	screen, err := LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	var header *uidsl.Node
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.ID == "settings-header" {
			header = node
		}
	})
	if header == nil {
		t.Fatal("settings header is missing")
	}
	var labels []string
	for index := range header.Children {
		child := &header.Children[index]
		if child.Component == "button" && child.Text != nil {
			labels = append(labels, child.Text.Literal)
		}
	}
	if got, want := strings.Join(labels, ","), "Back to Main,Agents,Vault,Restart Server"; got != want {
		t.Fatalf("settings header buttons = %q, want %q", got, want)
	}
	walkNodes(*header, func(node *uidsl.Node) {
		if node.Text != nil && node.Text.Literal == "Native client appearance and connection" {
			t.Fatal("settings header still contains the native-only subtitle")
		}
	})
}

func TestFrontPagePipelineProgressUsesSectionHeaderSurface(t *testing.T) {
	screen, err := LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	var matches []*uidsl.Node
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.Progress != nil && node.Progress.Binding == "section.progress" {
			matches = append(matches, node)
		}
	})
	if len(matches) != 1 {
		t.Fatalf("section progress surfaces = %d, want 1", len(matches))
	}
	header := matches[0]
	if header.Component != "row" || header.Style.Role != "execution-section-header" {
		t.Fatalf("section progress target = component %q role %q, want execution-section-header row", header.Component, header.Style.Role)
	}
	if header.Layout.Padding != "small" || len(header.Children) != 1 {
		t.Fatalf("section header layout = %#v with %d children", header.Layout, len(header.Children))
	}
	label := header.Children[0]
	if label.Component != "text" || label.Progress != nil || label.Text == nil || label.Text.Template != "pipeline: {{section.label}}" {
		t.Fatalf("section header label = %#v", label)
	}
	if label.Style.Role != "detail-small" || label.Style.Emphasis != "strong" || label.Style.Tone != "muted" {
		t.Fatalf("section header label style = %#v", label.Style)
	}
}

func TestSettingsAppearanceMatchesAuthoritativeStructure(t *testing.T) {
	screen, err := LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	var appearance *uidsl.Node
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.ID == "appearance" {
			appearance = node
		}
	})
	if appearance == nil {
		t.Fatal("settings appearance section is missing")
	}
	if len(appearance.Children) != 3 {
		t.Fatalf("appearance children = %d, want heading, description, and controls row", len(appearance.Children))
	}
	if appearance.Children[1].Component != "text" || appearance.Children[2].Component != "row" {
		t.Fatalf("appearance structure = %q, %q, want text followed by row", appearance.Children[1].Component, appearance.Children[2].Component)
	}
	controls := appearance.Children[2]
	if !controls.Layout.Wrap || len(controls.Children) != 3 {
		t.Fatalf("appearance controls must be a wrapping Theme/select/description row: %#v", controls.Layout)
	}
	if controls.Children[0].Text == nil || controls.Children[0].Text.Literal != "Theme" || controls.Children[1].Component != "select" {
		t.Fatal("appearance controls no longer start with the Theme label and selector")
	}
}

func TestSettingsProjectsMatchesAuthoritativeStructure(t *testing.T) {
	screen, err := LoadScreen("settings")
	if err != nil {
		t.Fatal(err)
	}
	var projects *uidsl.Node
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.ID == "projects" {
			projects = node
		}
	})
	if projects == nil || len(projects.Children) < 4 {
		t.Fatal("settings Projects section is missing")
	}
	if projects.Children[2].Component != "row" {
		t.Fatalf("project import controls use %q, want the web-style unboxed row", projects.Children[2].Component)
	}
	labels := map[string]bool{}
	var projectRowFound bool
	walkNodes(*projects, func(node *uidsl.Node) {
		if node.Component == "button" && node.Text != nil {
			labels[node.Text.Literal] = true
		}
		if node.Style.Role == "settings-project-row" {
			projectRowFound = true
		}
	})
	for _, label := range []string{"Add Repository Project", "Add Managed YAML", "Reload project definition from VCS", "Edit YAML", "Delete Project"} {
		if !labels[label] {
			t.Errorf("settings Projects section is missing %q", label)
		}
	}
	if !projectRowFound || labels["View"] || labels["Reload from VCS"] || labels["Delete"] {
		t.Fatal("settings Projects rows no longer match the authoritative action set")
	}
	if _, err := LoadScreen("managed-yaml"); err != nil {
		t.Fatalf("managed YAML editor screen is unavailable: %v", err)
	}
	if _, err := LoadScreen("vault"); err != nil {
		t.Fatalf("Vault screen is unavailable: %v", err)
	}
}

func TestOnlyVersionBadgesUseStrongWeight(t *testing.T) {
	for _, screenName := range []string{"front-page", "settings", "project-details", "job-details", "agents", "agent-details"} {
		screen, err := LoadScreen(screenName)
		if err != nil {
			t.Fatal(err)
		}
		walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
			if node.Component != "badge" || node.Text == nil {
				return
			}
			versionBadge := node.Text.Binding == "frontPage.server.version" || node.Text.Binding == "settings.server_version"
			if versionBadge != (node.Style.Emphasis == "strong") {
				t.Errorf("%s badge %#v emphasis = %q, version badge = %v", screenName, *node.Text, node.Style.Emphasis, versionBadge)
			}
		})
	}
}

func TestRoutesReferenceExistingScreensAndBindingRoots(t *testing.T) {
	routes, err := LoadRoutes()
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range routes.Routes {
		screen, err := LoadScreen(route.Screen)
		if err != nil {
			t.Errorf("route %q: %v", route.Name, err)
			continue
		}
		found := false
		for _, source := range screen.Screen.DataSources {
			if source.Name == route.BindingRoot {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("route %q binding root %q is not a data source of screen %q", route.Name, route.BindingRoot, route.Screen)
		}
	}
}

func TestButtonsUseSharedControlTypography(t *testing.T) {
	routes, err := LoadRoutes()
	if err != nil {
		t.Fatal(err)
	}
	loaded := map[string]bool{}
	for _, route := range routes.Routes {
		if loaded[route.Screen] {
			continue
		}
		loaded[route.Screen] = true
		screen, loadErr := LoadScreen(route.Screen)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
			if node.Component == "button" && node.Style.Emphasis != "" {
				t.Errorf("%s button %q overrides shared control emphasis with %q", route.Screen, node.ID, node.Style.Emphasis)
			}
		})
	}
}

func TestThemeDescriptionsMatchAuthoritativeWebCopy(t *testing.T) {
	themes, err := LoadThemes()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"default": "Bright mint with stronger color and contrast.",
		"jungle":  "Deep forest greens with vivid tropical accents.",
		"space":   "Midnight blue with cyan and violet highlights.",
	}
	for _, theme := range themes {
		if description, ok := want[theme.Metadata.Name]; ok && theme.Metadata.Description != description {
			t.Errorf("%s description = %q, want %q", theme.Metadata.Name, theme.Metadata.Description, description)
		}
	}
}

func TestJobOutputGroupsUseAuthoritativeStepContentTypography(t *testing.T) {
	screen, err := LoadScreen("job-details")
	if err != nil {
		t.Fatal(err)
	}
	var outputGroup *uidsl.Node
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.Component == "disclosure" && node.Style.Role == "output-group" {
			outputGroup = node
		}
	})
	if outputGroup == nil {
		t.Fatal("job details screen has no output-group disclosure")
	}
	rolesByLiteral := map[string]string{}
	for index := range outputGroup.Children {
		child := &outputGroup.Children[index]
		if child.Component == "badge" {
			t.Error("expanded output group must not duplicate the job-step status pill")
		}
		if child.Text == nil {
			continue
		}
		if child.Text.Binding == "outputGroup.command_summary" {
			t.Error("expanded output group must not duplicate the raw command before its YAML section")
		}
		if child.Text.Literal != "" {
			rolesByLiteral[child.Text.Literal] = child.Style.Role
		}
	}
	for literal, wantRole := range map[string]string{
		"YAML literal":     "output-label",
		"Expanded command": "output-label",
	} {
		if got := rolesByLiteral[literal]; got != wantRole {
			t.Errorf("%q typography role = %q, want %q", literal, got, wantRole)
		}
	}
}

func TestHistoryExecutionRowsOwnJobNavigation(t *testing.T) {
	for _, screenName := range []string{"front-page", "project-details"} {
		screen, err := LoadScreen(screenName)
		if err != nil {
			t.Fatal(err)
		}
		rows := 0
		walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
			if node.Style.Role != "history-execution-job-row" {
				return
			}
			rows++
			if len(node.Actions) != 1 || node.Actions[0].Command != "navigate" || node.Actions[0].Arguments["route"] != "/jobs/{{job.id}}" {
				t.Errorf("%s history row action = %#v, want row-level job navigation", screenName, node.Actions)
			}
			for _, child := range node.Children {
				if len(child.Actions) != 0 {
					t.Errorf("%s history row child retains its own action: %#v", screenName, child.Actions)
				}
			}
		})
		if rows == 0 {
			t.Errorf("%s has no history execution rows", screenName)
		}
	}
}

func TestFrontPageMatchesBrowserProjectAndEmptyTableSummaries(t *testing.T) {
	payload, err := Read("screens/front-page.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(payload)
	for _, want := range []string{
		"literal: Managed YAML",
		"binding: project.source_kind",
		"equals: managed_yaml",
		`template: "{{execution.kind}}: {{execution.title}}"`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("front-page screen no longer contains %q", want)
		}
	}
	header := strings.Index(source, "role: queued-execution-header")
	empty := strings.Index(source, "literal: No queued jobs.")
	if header < 0 || empty < 0 || header > empty {
		t.Errorf("queued table header must precede its empty row: header=%d empty=%d", header, empty)
	}
}

func TestFrontPageExecutionSummariesDeclareRightAlignedContent(t *testing.T) {
	screen, err := LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	var executionRows int
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.Component != "disclosure" || node.Style.Role != "execution-row" {
			return
		}
		executionRows++
		if node.Disclosure == nil || len(node.Disclosure.Summary) < 2 {
			t.Errorf("execution summary = %#v, want a grow spacer followed by right-aligned content", node.Disclosure)
			return
		}
		spacer := node.Disclosure.Summary[0]
		if spacer.Component != "spacer" || !spacer.Layout.Grow {
			t.Errorf("execution summary first child = %#v, want growing spacer", spacer)
		}
	})
	if executionRows != 2 {
		t.Fatalf("execution disclosures = %d, want queued and history rows", executionRows)
	}
}

func TestFrontPageProjectBodyUsesOrdinaryDeclarativeNodes(t *testing.T) {
	screen, err := LoadScreen("front-page")
	if err != nil {
		t.Fatal(err)
	}
	var project *uidsl.Node
	walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
		if node.Component == "disclosure" && node.Style.Role == "project-row" {
			project = node
		}
	})
	if project == nil {
		t.Fatal("front page project disclosure is missing")
	}
	if project.Image != nil {
		t.Fatal("project disclosure still asks renderers to compose its body image")
	}
	if len(project.Children) != 1 || project.Children[0].Component != "row" {
		t.Fatalf("project body = %#v, want one ordinary row", project.Children)
	}
	body := project.Children[0]
	if body.Layout.Gap != "large" || body.Layout.MinWidth != "0" || len(body.Children) != 2 {
		t.Fatalf("project body layout = %#v with %d children", body.Layout, len(body.Children))
	}
	leading, content := body.Children[0], body.Children[1]
	if leading.Component != "column" || len(leading.Children) != 1 || leading.Children[0].Component != "image" || leading.Children[0].Style.Role != "project-icon" {
		t.Fatalf("project leading content = %#v", leading)
	}
	if content.Component != "column" || !content.Layout.Grow || len(content.Children) != 2 {
		t.Fatalf("project main content = %#v", content)
	}
}

func TestPlatformOverridesAreLimitedToPlatformMechanics(t *testing.T) {
	routes, err := LoadRoutes()
	if err != nil {
		t.Fatal(err)
	}
	loaded := map[string]bool{}
	for _, route := range routes.Routes {
		if loaded[route.Screen] {
			continue
		}
		loaded[route.Screen] = true
		screen, loadErr := LoadScreen(route.Screen)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		walkNodes(screen.Screen.Root, func(node *uidsl.Node) {
			for platform, override := range node.Overrides {
				if platform != "web" && platform != "gio" {
					continue
				}
				allowed := route.Screen == "settings" && node.ID == "native-connection" && platform == "web" && override.Hidden
				allowed = allowed || route.Screen == "job-details" && node.Style.Role == "floating-collapse" && platform == "gio" && override.Hidden
				if !allowed {
					t.Errorf("%s node %q role %q has non-mechanical %s override: %#v", route.Screen, node.ID, node.Style.Role, platform, override)
				}
			}
		})
	}
}

func walkNodes(node uidsl.Node, visit func(*uidsl.Node)) {
	visit(&node)
	for index := range node.Children {
		walkNodes(node.Children[index], visit)
	}
	if node.Disclosure != nil {
		for index := range node.Disclosure.Summary {
			walkNodes(node.Disclosure.Summary[index], visit)
		}
	}
	if node.GraphView != nil {
		for index := range node.GraphView.Details {
			walkNodes(node.GraphView.Details[index], visit)
		}
	}
}
