package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dop251/goja"
)

func TestBrowserChangeRefreshSchedulerCoalescesWhileRefreshIsActive(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/change-refresh.js")
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	_, err = runtime.RunString(`
globalThis.window = {};
` + string(payload) + `
const timers = [];
let resolveRefresh;
const refreshGate = new Promise(resolve => { resolveRefresh = resolve; });
let refreshCalls = 0;
let scheduler;
scheduler = window.ciwiCreateChangeRefreshScheduler({
  delay: 100,
  setTimer: callback => { timers.push(callback); return timers.length; },
  refresh: () => {
    refreshCalls += 1;
    scheduler.beginRefresh();
    return refreshGate.finally(() => scheduler.endRefresh());
  },
});

// Model an action or navigation refresh that is already crossing a slow link.
scheduler.beginRefresh();
scheduler.schedule();
scheduler.schedule();
const timersDuringExplicitRefresh = timers.length;
scheduler.endRefresh();
const timersAfterExplicitRefresh = timers.length;

// Start the single queued change refresh, then deliver more invalidations
// while its response is in flight.
timers.shift()();
const callsAfterTimer = refreshCalls;
scheduler.schedule();
scheduler.schedule();
const timersDuringChangeRefresh = timers.length;
resolveRefresh();
`)
	if err != nil {
		t.Fatal(err)
	}
	assertions := map[string]int64{
		"timersDuringExplicitRefresh": 0,
		"timersAfterExplicitRefresh":  1,
		"callsAfterTimer":             1,
		"timersDuringChangeRefresh":   0,
		"timers.length":               1,
	}
	for expression, expected := range assertions {
		value, evalErr := runtime.RunString(expression)
		if evalErr != nil {
			t.Fatal(evalErr)
		}
		if actual := value.ToInteger(); actual != expected {
			t.Errorf("%s = %d, want %d", expression, actual, expected)
		}
	}
}

func TestBrowserChangeRefreshSchedulerAssetUsesImmutableVersionedCache(t *testing.T) {
	recorder := httptest.NewRecorder()
	path := "/ui/change-refresh.js?v=" + currentBrowserUIRevision()
	Handler(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable versioned caching", cacheControl)
	}
}
