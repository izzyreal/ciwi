package webui

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	sharedUI "github.com/izzyreal/ciwi/ui"
)

func TestUIThemeScriptIsServedAndPersistsBrowserChoice(t *testing.T) {
	req := httptest.NewRequest("GET", "/ui/theme.js", nil)
	recorder := httptest.NewRecorder()
	Handler(recorder, req)

	if recorder.Code != 200 {
		t.Fatalf("GET /ui/theme.js status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/javascript; charset=utf-8" {
		t.Fatalf("GET /ui/theme.js Content-Type = %q", got)
	}
	for _, want := range []string{
		"ciwi.ui.theme.v1",
		"cache: 'no-store'",
		"'default'",
		"'jungle'",
		"'space'",
		"'pina-colada'",
		"'mango-kent'",
		"'mango-chaunsa'",
		"'mango-alphonso'",
		"'yellow-dragon-fruit'",
		"'dragon-fruit'",
		"localStorage.setItem(ciwiThemeStorageKey",
		"/ui/contracts/themes.json",
		"ciwiApplyContractTheme(normalized)",
		"'awaiting-surface': '--awaiting-bg'",
		"'awaiting-border': '--awaiting-line'",
		"'awaiting-text': '--awaiting-ink'",
		"'console-success': '--console-green'",
		"data-ciwi-theme",
		"ciwi-theme-change",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("theme script no longer contains %q", want)
		}
	}
}

func TestBrowserUsesTheBundledNativeMonospaceFace(t *testing.T) {
	for path, nativePath := range map[string]string{
		"/ui/fonts/ciwi-mono-regular.ttf": "assets/GeistMono-Regular.ttf",
		"/ui/fonts/ciwi-mono-medium.ttf":  "assets/GeistMono-Medium.ttf",
		"/ui/fonts/ciwi-mono-bold.ttf":    "assets/GeistMono-Bold.ttf",
	} {
		recorder := httptest.NewRecorder()
		Handler(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 || recorder.Header().Get("Content-Type") != "font/ttf" || recorder.Body.Len() < 1000 {
			t.Fatalf("GET %s status=%d contentType=%q bytes=%d", path, recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.Len())
		}
		nativeFont, err := sharedUI.Read(nativePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(recorder.Body.Bytes(), nativeFont) {
			t.Errorf("browser font %s and native font %s differ", path, nativePath)
		}
	}
	if !strings.Contains(chromeCSS, `@import url("/ui/css/typography.css")`) {
		t.Error("shared browser chrome no longer loads the typography contract stylesheet")
	}
	for _, stylesheet := range []string{mustTestAsset("assets/css/declarative.css"), jobExecutionCSS, projectCSS} {
		if !strings.Contains(stylesheet, `var(--ciwi-font-mono)`) && !strings.Contains(stylesheet, `"Ciwi Mono"`) {
			t.Error("monospace UI surface does not consume the shared monospace family")
		}
	}
}

func TestTypographyContractAndStylesheetAreServed(t *testing.T) {
	contract := httptest.NewRecorder()
	Handler(contract, httptest.NewRequest("GET", "/ui/contracts/typography.json", nil))
	if contract.Code != 200 || contract.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("typography contract status=%d contentType=%q", contract.Code, contract.Header().Get("Content-Type"))
	}
	for _, want := range []string{`"native":450`, `"output-meta"`, `"output-label"`, `"output-code"`} {
		if !strings.Contains(contract.Body.String(), want) {
			t.Errorf("typography contract no longer contains %q", want)
		}
	}

	stylesheet := httptest.NewRecorder()
	Handler(stylesheet, httptest.NewRequest("GET", "/ui/css/typography.css", nil))
	if stylesheet.Code != 200 || stylesheet.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("typography stylesheet status=%d contentType=%q", stylesheet.Code, stylesheet.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		`ciwi-mono-medium.ttf`,
		`--ciwi-font-body:`,
		`--ciwi-type-output-meta-family:var(--ciwi-font-mono)`,
		`--ciwi-type-output-code-size:12px`,
	} {
		if !strings.Contains(stylesheet.Body.String(), want) {
			t.Errorf("typography stylesheet no longer contains %q", want)
		}
	}
}

func TestMutableBrowserAssetsAreNotCachedAcrossServerUpdates(t *testing.T) {
	for _, path := range []string{
		"/ui/css/chrome.css",
		"/ui/theme.js",
		"/ui/contracts/themes.json",
		"/ui/contracts/typography.json",
	} {
		recorder := httptest.NewRecorder()
		Handler(recorder, httptest.NewRequest("GET", path, nil))
		if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", path, got)
		}
	}
}

func TestEveryUIPageLoadsThemeBeforeStyles(t *testing.T) {
	pages := map[string]string{
		"index":         indexHTML,
		"project":       projectHTML,
		"job execution": jobExecutionHTML,
		"settings":      settingsHTML,
		"agents":        agentsHTML,
		"agent":         agentHTML,
		"vault":         vaultHTML,
	}
	for name, page := range pages {
		themeIndex := strings.Index(page, `<script src="/ui/theme.js?v=2"></script>`)
		styleIndex := strings.Index(page, `<link rel="stylesheet"`)
		if themeIndex < 0 || styleIndex < 0 || themeIndex > styleIndex {
			t.Errorf("%s page does not load theme script before styles", name)
		}
	}
}

func TestGlobalSettingsOffersAllThemes(t *testing.T) {
	for _, want := range []string{
		`id="themeSelect"`,
		`value="default"`,
		`value="jungle"`,
		`value="space"`,
		`value="pina-colada"`,
		`value="mango-kent"`,
		`value="mango-chaunsa"`,
		`value="mango-alphonso"`,
		`value="yellow-dragon-fruit"`,
		`value="dragon-fruit"`,
		`Piña Colada`,
		`Fruit themes`,
		`initializeThemeSettings()`,
		`select.onchange = () => update(select.value)`,
	} {
		if !strings.Contains(settingsHTML+settingsJS, want) {
			t.Errorf("settings theme selector no longer contains %q", want)
		}
	}
}

func TestGlobalSettingsReadsDescriptionsFromThemeContracts(t *testing.T) {
	for _, want := range []string{
		`ciwiThemeContracts()`,
		`metadata.description || ''`,
		`descriptions[select.value] || ''`,
	} {
		if !strings.Contains(themeJS+settingsJS, want) {
			t.Errorf("settings theme descriptions no longer use the shared contract: missing %q", want)
		}
	}
	if strings.Contains(settingsJS, `space: 'Midnight blue`) {
		t.Fatal("settings must not duplicate theme descriptions in JavaScript")
	}
}

func TestThemesDefineSharedComponentAndGraphTokens(t *testing.T) {
	for _, want := range []string{
		`:root[data-ciwi-theme="jungle"]`,
		`:root[data-ciwi-theme="space"]`,
		`:root[data-ciwi-theme="pina-colada"]`,
		`:root[data-ciwi-theme="mango-kent"]`,
		`:root[data-ciwi-theme="mango-chaunsa"]`,
		`:root[data-ciwi-theme="mango-alphonso"]`,
		`:root[data-ciwi-theme="yellow-dragon-fruit"]`,
		`:root[data-ciwi-theme="dragon-fruit"]`,
		`--surface-hover:`,
		`--graph-running-bg:`,
		`--graph-succeeded-border:`,
		`--graph-failed-bg:`,
		`--console-bg:`,
		`--snackbar-bg:`,
	} {
		if !strings.Contains(chromeCSS, want) {
			t.Errorf("theme tokens no longer contain %q", want)
		}
	}
	for _, want := range []string{
		`background:var(--graph-succeeded-bg)`,
		`border-color:var(--graph-failed-border)`,
		`background:var(--graph-running-bg)`,
	} {
		if !strings.Contains(projectCSS+jobExecutionCSS, want) {
			t.Errorf("graph components no longer consume shared token %q", want)
		}
	}
}

func TestBrowserThemeContractMapsNoticePalette(t *testing.T) {
	declarativeJS := mustTestAsset("assets/js/declarative.js")
	for _, source := range []string{themeJS, declarativeJS} {
		for _, mapping := range []string{
			`'notice-background': '--snackbar-bg'`,
			`'notice-text': '--snackbar-ink'`,
			`'notice-border': '--snackbar-line'`,
		} {
			if !strings.Contains(source, mapping) {
				t.Errorf("browser theme contract is missing %q", mapping)
			}
		}
	}
}

func TestThemesUseSharedLayeredGradientSurfaces(t *testing.T) {
	for _, want := range []string{
		`--page-background: radial-gradient(`,
		`radial-gradient(circle at 90% 8%`,
		`linear-gradient(145deg, var(--bg2)`,
		`--card-background: radial-gradient(`,
		`--graph-background: radial-gradient(`,
		`--console-background: radial-gradient(`,
		`background: var(--page-background);`,
		`background: var(--card-background);`,
	} {
		if !strings.Contains(chromeCSS, want) {
			t.Errorf("layered theme surfaces no longer contain %q", want)
		}
	}
	if !strings.Contains(projectCSS, `background:var(--graph-background)`) {
		t.Error("project graph no longer consumes the shared gradient background")
	}
	if !strings.Contains(jobExecutionCSS, `background: var(--console-background)`) {
		t.Error("job log no longer consumes the shared gradient background")
	}
}

func TestSharedSelectLayoutCoversSettingsAndProjectGraph(t *testing.T) {
	combined := chromeCSS + settingsHTML + projectJS
	for _, want := range []string{
		`select.ciwi-select`,
		`background-position: right 11px center`,
		`class="ciwi-select version-select"`,
		`select.className = 'ciwi-select project-graph-select';`,
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("shared select styling no longer contains %q", want)
		}
	}
}

func TestUpdateRestartWatcherRecognizesRequestedVersion(t *testing.T) {
	for _, want := range []string{
		`waitForServerRestartAndReload(target)`,
		`async function waitForServerRestartAndReload(expectedVersion)`,
		`const targetReached = expected !== '' && currentNormalized === expected;`,
		`finished = expected ? targetReached : (upToDate || success);`,
	} {
		if !strings.Contains(settingsJS, want) {
			t.Fatalf("update restart watcher no longer contains %q", want)
		}
	}
}

func TestAllPagesUseProjectPageWidthFromSharedChrome(t *testing.T) {
	if !strings.Contains(chromeCSS, `main { max-width: 1600px;`) {
		t.Fatal("shared page width no longer matches the project details page")
	}
	if strings.Contains(projectCSS, `main { max-width:`) {
		t.Fatal("project page must not override the shared page width")
	}
}

func TestGraphNodeTextSelectionDoesNotTriggerNavigation(t *testing.T) {
	combined := sharedJS + projectJS + jobExecutionJS
	for _, want := range []string{
		`function ciwiElementContainsTextSelection(element)`,
		`if (ciwiElementContainsTextSelection(button)) return;`,
		`if (ciwiElementContainsTextSelection(node)) return;`,
		`user-select:text`,
	} {
		if !strings.Contains(combined+projectCSS+jobExecutionCSS, want) {
			t.Errorf("graph text-selection behavior no longer contains %q", want)
		}
	}
}
