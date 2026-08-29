package webui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestBrowserLiveJobDurationMatchesNativeFormatting(t *testing.T) {
	runtime := goja.New()
	if _, err := runtime.RunString("globalThis.window = globalThis;\n" + mustTestAsset("assets/js/view-bindings.js") + `
globalThis.durationBindings = window.ciwiCreateBrowserViewBindings({getCurrentData: () => null});
`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name                                       string
		start, serverSnapshot, clientSnapshot, now int64
		want                                       string
	}{
		{name: "invalid", start: 0, serverSnapshot: 1, clientSnapshot: 1, now: 1, want: ""},
		{name: "clock clamp", start: 2_000, serverSnapshot: 1_000, clientSnapshot: 5_000, now: 4_000, want: "0s"},
		{name: "whole seconds", start: 1_000, serverSnapshot: 6_900, clientSnapshot: 20_000, now: 20_999, want: "6s"},
		{name: "minute", start: 1_000, serverSnapshot: 61_000, clientSnapshot: 20_000, now: 25_000, want: "1m5s"},
		{name: "hour", start: 1_000, serverSnapshot: 3_662_000, clientSnapshot: 20_000, now: 20_000, want: "1h1m1s"},
	} {
		t.Run(test.name, func(t *testing.T) {
			expression := fmt.Sprintf("durationBindings.liveJobDuration(%d, %d, %d, %d)", test.start, test.serverSnapshot, test.clientSnapshot, test.now)
			value, err := runtime.RunString(expression)
			if err != nil {
				t.Fatal(err)
			}
			if got := value.String(); got != test.want {
				t.Fatalf("duration = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBrowserLiveJobDurationUsesExistingPulseInterval(t *testing.T) {
	script := string(mustTestAsset("assets/js/declarative.js"))
	for _, expected := range []string{".dsl-live-job-duration", "updateLiveJobDuration(element, Date.now())", "}, 250);"} {
		if !strings.Contains(script, expected) {
			t.Fatalf("browser duration updater does not contain %q", expected)
		}
	}
}
