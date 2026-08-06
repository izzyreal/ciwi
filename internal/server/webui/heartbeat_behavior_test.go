package webui

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/izzyreal/ciwi/internal/protocol"
)

func TestBrowserHeartbeatBehavior(t *testing.T) {
	runtime := goja.New()
	if _, err := runtime.RunString("globalThis.window = globalThis;\n" + heartbeatJS); err != nil {
		t.Fatal(err)
	}
	now := int64(1786000000000)
	number := func(expression string) float64 {
		t.Helper()
		value, err := runtime.RunString(expression)
		if err != nil {
			t.Fatal(err)
		}
		return value.ToFloat()
	}
	text := func(expression string) string {
		t.Helper()
		value, err := runtime.RunString(expression)
		if err != nil {
			t.Fatal(err)
		}
		return value.String()
	}
	if got := time.Duration(number("window.ciwiHeartbeat.durationMilliseconds()")) * time.Millisecond; got != protocol.AgentHeartbeatFadeDuration {
		t.Fatalf("browser heartbeat duration = %v, want %v", got, protocol.AgentHeartbeatFadeDuration)
	}
	for _, test := range []struct {
		name        string
		timestamp   int64
		wantOpacity float64
	}{
		{name: "beat", timestamp: now, wantOpacity: 1},
		{name: "midpoint", timestamp: now - protocol.AgentHeartbeatFadeDuration.Milliseconds()/2, wantOpacity: .59},
		{name: "expired", timestamp: now - protocol.AgentHeartbeatFadeDuration.Milliseconds(), wantOpacity: .18},
	} {
		t.Run(test.name, func(t *testing.T) {
			expression := "window.ciwiHeartbeat.opacity(" + strconv.FormatInt(test.timestamp, 10) + "," + strconv.FormatInt(now, 10) + ")"
			if got := number(expression); math.Abs(got-test.wantOpacity) > .000001 {
				t.Fatalf("opacity = %g, want %g", got, test.wantOpacity)
			}
		})
	}
	labels := map[string]string{
		"window.ciwiHeartbeat.ageLabel(0," + strconv.FormatInt(now, 10) + ")":                                            "never",
		"window.ciwiHeartbeat.ageLabel('broken'," + strconv.FormatInt(now, 10) + ")":                                     "never",
		"window.ciwiHeartbeat.ageLabel(" + strconv.FormatInt(now+1, 10) + "," + strconv.FormatInt(now, 10) + ")":         "just now",
		"window.ciwiHeartbeat.ageLabel(" + strconv.FormatInt(now-2000, 10) + "," + strconv.FormatInt(now, 10) + ")":      "2s ago",
		"window.ciwiHeartbeat.ageLabel(" + strconv.FormatInt(now-120000, 10) + "," + strconv.FormatInt(now, 10) + ")":    "2m ago",
		"window.ciwiHeartbeat.ageLabel(" + strconv.FormatInt(now-7200000, 10) + "," + strconv.FormatInt(now, 10) + ")":   "2h ago",
		"window.ciwiHeartbeat.ageLabel(" + strconv.FormatInt(now-172800000, 10) + "," + strconv.FormatInt(now, 10) + ")": "2d ago",
	}
	for expression, want := range labels {
		if got := text(expression); got != want {
			t.Errorf("%s = %q, want %q", expression, got, want)
		}
	}
}

func TestHeartbeatTimingContractAndAssetsStayShared(t *testing.T) {
	match := regexp.MustCompile(`--ciwi-heartbeat-fade-duration:\s*([0-9.]+)(ms|s)`).FindStringSubmatch(chromeCSS)
	if len(match) != 3 {
		t.Fatal("chrome CSS is missing the heartbeat fade duration")
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		t.Fatal(err)
	}
	duration := time.Duration(value * float64(time.Millisecond))
	if match[2] == "s" {
		duration = time.Duration(value * float64(time.Second))
	}
	if duration != protocol.AgentHeartbeatFadeDuration {
		t.Fatalf("browser CSS heartbeat duration = %v, want %v", duration, protocol.AgentHeartbeatFadeDuration)
	}
	for name, page := range map[string]string{"agents": agentsHTML, "declarative": declarativeHTML} {
		heartbeatAt := strings.Index(page, `src="/ui/heartbeat.js"`)
		clientAt := strings.Index(page, `src="/ui/`+name+`.js"`)
		if heartbeatAt < 0 || clientAt < 0 || heartbeatAt > clientAt {
			t.Fatalf("%s page must load heartbeat.js before its client script", name)
		}
	}
	for name, asset := range map[string]string{"agents.js": agentsJS, "declarative.js": mustTestAsset("assets/js/declarative.js")} {
		if !strings.Contains(asset, "ciwiHeartbeat.opacity") {
			t.Errorf("%s does not use shared heartbeat opacity", name)
		}
	}
	if strings.Contains(agentsCSS, "heartbeat-fade 10s") || strings.Contains(agentsJS, "elapsed < 10000") {
		t.Fatal("legacy ten-second heartbeat timing remains in authoritative browser assets")
	}
	for _, expected := range []string{"data-heartbeat-unix-ms", "setInterval(updateHeartbeatVisuals, 250)"} {
		if !strings.Contains(agentsJS, expected) {
			t.Errorf("agents.js is missing live heartbeat behavior %q", expected)
		}
	}
}
