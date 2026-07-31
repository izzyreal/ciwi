package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIThemeScriptIsServedAndPersistsBrowserChoice(t *testing.T) {
	req := httptest.NewRequest("GET", "/ui/theme.js", nil)
	recorder := httptest.NewRecorder()
	store := &stateStore{}
	store.uiHandler(recorder, req)

	if recorder.Code != 200 {
		t.Fatalf("GET /ui/theme.js status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/javascript; charset=utf-8" {
		t.Fatalf("GET /ui/theme.js Content-Type = %q", got)
	}
	for _, want := range []string{
		"ciwi.ui.theme.v1",
		"'default', 'jungle', 'space'",
		"localStorage.setItem(ciwiThemeStorageKey",
		"data-ciwi-theme",
		"ciwi-theme-change",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Errorf("theme script no longer contains %q", want)
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
		themeIndex := strings.Index(page, `<script src="/ui/theme.js"></script>`)
		styleIndex := strings.Index(page, `<style>`)
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
		`initializeThemeSettings()`,
		`select.onchange = () => update(select.value)`,
	} {
		if !strings.Contains(settingsHTML, want) {
			t.Errorf("settings theme selector no longer contains %q", want)
		}
	}
}

func TestThemesDefineSharedComponentAndGraphTokens(t *testing.T) {
	for _, want := range []string{
		`:root[data-ciwi-theme="jungle"]`,
		`:root[data-ciwi-theme="space"]`,
		`--surface-hover:`,
		`--graph-running-bg:`,
		`--graph-succeeded-border:`,
		`--graph-failed-bg:`,
		`--console-bg:`,
		`--snackbar-bg:`,
	} {
		if !strings.Contains(uiPageChromeCSS, want) {
			t.Errorf("theme tokens no longer contain %q", want)
		}
	}
	for _, want := range []string{
		`background:var(--graph-succeeded-bg)`,
		`border-color:var(--graph-failed-border)`,
		`background:var(--graph-running-bg)`,
	} {
		if !strings.Contains(uiProjectGraphCSS+jobExecutionGraphCSS, want) {
			t.Errorf("graph components no longer consume shared token %q", want)
		}
	}
}

func TestGraphNodeTextSelectionDoesNotTriggerNavigation(t *testing.T) {
	combined := uiSharedCoreJS + uiProjectGraphJS + jobExecutionGraphJS
	for _, want := range []string{
		`function ciwiElementContainsTextSelection(element)`,
		`if (ciwiElementContainsTextSelection(button)) return;`,
		`if (ciwiElementContainsTextSelection(node)) return;`,
		`user-select:text`,
	} {
		if !strings.Contains(combined+uiProjectGraphCSS+jobExecutionGraphCSS, want) {
			t.Errorf("graph text-selection behavior no longer contains %q", want)
		}
	}
}
