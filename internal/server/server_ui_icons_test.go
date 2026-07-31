package server

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestUIIconSpriteIsServedFromEmbeddedAssets(t *testing.T) {
	req := httptest.NewRequest("GET", "/ui/icons.svg", nil)
	recorder := httptest.NewRecorder()
	store := &stateStore{}

	store.uiHandler(recorder, req)

	if recorder.Code != 200 {
		t.Fatalf("GET /ui/icons.svg status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("GET /ui/icons.svg Content-Type = %q, want image/svg+xml", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("GET /ui/icons.svg Cache-Control = %q, want public, max-age=3600", got)
	}
	for _, name := range []string{
		"arrow-left",
		"arrow-up",
		"chevron-down",
		"chevron-right",
		"chevron-up",
		"chevrons-down",
		"chevrons-up",
		"circle-check",
		"circle-x",
		"clock",
		"device-desktop",
		"info-circle",
		"loader-2",
		"player-play",
		"refresh",
		"settings",
		"zoom-in",
		"zoom-out",
	} {
		id := "icon-" + name
		if !strings.Contains(recorder.Body.String(), `id="`+id+`"`) {
			t.Errorf("icon sprite does not contain %q", id)
		}
		if !strings.Contains(uiSharedIconsJS, `'`+name+`'`) {
			t.Errorf("shared icon allowlist does not contain %q", name)
		}
	}
}

func TestUIProvidesSharedAllowlistedIconRenderers(t *testing.T) {
	for _, want := range []string{
		"const ciwiIconNames = new Set",
		"function ciwiIconHTML(name, options)",
		"function ciwiIconElement(name, options)",
		"ciwiIconNames.has(normalized)",
		`aria-hidden="true" focusable="false"`,
		"/ui/icons.svg#icon-",
	} {
		if !strings.Contains(uiSharedJS, want) {
			t.Errorf("shared icon renderer no longer contains %q", want)
		}
	}
}

func TestUIStaticIconOnlyButtonsHaveAccessibleNames(t *testing.T) {
	buttonPattern := regexp.MustCompile(`(?s)<button[^>]*class="[^"]*ciwi-icon-only[^"]*"[^>]*>`)
	combined := jobExecutionHTML + projectHTML + settingsHTML + indexHTML
	buttons := buttonPattern.FindAllString(combined, -1)
	if len(buttons) == 0 {
		t.Fatal("expected at least one static icon-only button")
	}
	for _, button := range buttons {
		if !strings.Contains(button, `aria-label="`) {
			t.Errorf("icon-only button has no accessible name: %s", button)
		}
	}
}

func TestUIDynamicIconOnlyButtonsSetAccessibleNames(t *testing.T) {
	combined := uiProjectGraphJS + jobExecutionGraphJS
	for _, want := range []string{
		"play.setAttribute('aria-label'",
		"rerun.setAttribute('aria-label'",
		"outBtn.setAttribute('aria-label', 'Zoom out')",
		"inBtn.setAttribute('aria-label', 'Zoom in')",
	} {
		if !strings.Contains(combined, want) {
			t.Errorf("dynamic icon-only controls no longer contain %q", want)
		}
	}
}

func TestQueueCardsUseIconsForAggregateStatus(t *testing.T) {
	for _, want := range []string{
		`icon: 'circle-x'`,
		`icon: 'loader-2'`,
		`icon: 'clock'`,
		`icon: 'circle-check'`,
		`ciwiIconHTML(status.icon`,
	} {
		if !strings.Contains(uiIndexJobExecutionsJS, want) {
			t.Errorf("queue aggregate status no longer contains %q", want)
		}
	}
	for _, unwanted := range []string{"⏳", "❌", "✅", "🟡"} {
		if strings.Contains(uiIndexJobExecutionsJS, unwanted) {
			t.Errorf("queue aggregate status still contains emoji %q", unwanted)
		}
	}
}

func TestCollapsibleCardFocusUsesKeyboardFocusIndicatorOnly(t *testing.T) {
	for _, want := range []string{
		`.project-group > summary:focus:not(:focus-visible)`,
		`.ciwi-job-group-details > summary:focus:not(:focus-visible)`,
		`.project-group > summary:focus-visible`,
		`.ciwi-job-group-details > summary:focus-visible`,
	} {
		if !strings.Contains(uiIndexCSS, want) {
			t.Errorf("collapsible card focus styling no longer contains %q", want)
		}
	}
}
