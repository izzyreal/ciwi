package webui

import (
	"math"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestBrowserProgressInterpolationKeepsServerSemanticState(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	constantStart := strings.Index(script, "  const determinateProgressLimit")
	functionStart := strings.Index(script, "  function semanticProgressAt")
	if constantStart < 0 || functionStart < 0 {
		t.Fatal("browser semantic progress implementation is unavailable")
	}
	constantEnd := strings.Index(script[constantStart:], "\n  const disclosureStates")
	functionEnd := strings.Index(script[functionStart:], "\n  function updateSemanticProgress")
	if constantEnd < 0 || functionEnd < 0 {
		t.Fatal("browser semantic progress implementation is incomplete")
	}
	constant := script[constantStart : constantStart+constantEnd]
	function := script[functionStart : functionStart+functionEnd]
	runtime := goja.New()
	if _, err := runtime.RunString(constant + "\n" + function); err != nil {
		t.Fatal(err)
	}

	assertProgress := func(expression, wantState string, wantFraction float64) {
		t.Helper()
		value, err := runtime.RunString(expression)
		if err != nil {
			t.Fatal(err)
		}
		model := value.ToObject(runtime)
		if state := model.Get("state").String(); state != wantState {
			t.Fatalf("state = %q, want %q", state, wantState)
		}
		if fraction := model.Get("fraction").ToFloat(); math.Abs(fraction-wantFraction) > .000001 {
			t.Fatalf("fraction = %g, want %g", fraction, wantFraction)
		}
	}

	assertProgress("semanticProgressAt({state:'determinate',fraction:.2,snapshot_unix_ms:1000,rate_per_ms:.0001},4000)", "determinate", .5)
	assertProgress("semanticProgressAt({state:'determinate',fraction:.9,snapshot_unix_ms:1000,rate_per_ms:.0001},4000)", "determinate", .999)
	assertProgress("semanticProgressAt({state:'overrun',fraction:1,snapshot_unix_ms:1000,rate_per_ms:0},4000)", "overrun", 1)

	assertDelay := func(expression, want string) {
		t.Helper()
		value, err := runtime.RunString(expression)
		if err != nil {
			t.Fatal(err)
		}
		if got := value.String(); got != want {
			t.Fatalf("delay = %q, want %q", got, want)
		}
	}
	assertDelay("semanticProgressAnimationDelay('indeterminate', 4500)", "-500ms")
	assertDelay("semanticProgressAnimationDelay('overrun', 4500)", "-500ms")
	assertDelay("semanticProgressAnimationDelay('determinate', 4500)", "")
}

func TestBrowserProgressUpdatesDoNotRestartAnimationClasses(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(payload)
	if strings.Contains(script, "classList.remove('ciwi-progress-indeterminate'") {
		t.Fatal("semantic progress removes an active animation class during periodic updates")
	}
	for _, expected := range []string{
		"classList.toggle('ciwi-progress-indeterminate', model.state === 'indeterminate')",
		"semanticProgressAnimationDelay(model.state, nowMs)",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("browser progress continuity is missing %q", expected)
		}
	}
}

func TestBrowserStatusSpinnerKeepsDOMIdentityAcrossRenders(t *testing.T) {
	javascript, err := uiAssets.ReadFile("assets/js/declarative.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"status.dataset.ciwiStableKey = 'execution-status:' + disclosureKey + ':' + statusIcon",
		"preserveStableElements(nextRoot)",
		"element.replaceWith(retained)",
	} {
		if !strings.Contains(string(javascript), expected) {
			t.Fatalf("browser status spinner continuity is missing %q", expected)
		}
	}
}
