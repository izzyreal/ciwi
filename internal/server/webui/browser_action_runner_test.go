package webui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

var browserActionIDPattern = regexp.MustCompile(`^[0-9a-f-]{36}$`)

type browserActionHarness struct {
	runtime *goja.Runtime
}

func newBrowserActionHarness(t *testing.T) *browserActionHarness {
	t.Helper()
	runnerSource, err := uiAssets.ReadFile("assets/js/actions.js")
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	actions := []map[string]any{
		{"command": "mutate", "class": "mutation", "scope": "resource:{{id}}", "pending": "Working…", "refreshOnSuccess": true},
		{"command": "other-mutation", "class": "mutation", "scope": "resource:{{id}}", "pending": "Other work…"},
		{"command": "query-a", "class": "query", "scope": "screen"},
		{"command": "query-b", "class": "query", "scope": "screen"},
	}
	if err := runtime.Set("__ciwiTestActions", actions); err != nil {
		t.Fatal(err)
	}
	harnessSource := `
globalThis.CustomEvent = class CustomEvent {
  constructor(type, options) {
    this.type = type;
    this.detail = options.detail;
  }
};
class CiwiTestAbortSignal {
  constructor() {
    this.aborted = false;
    this.listeners = [];
  }
  addEventListener(type, listener) {
    if (type === 'abort') this.listeners.push(listener);
  }
}
globalThis.AbortController = class AbortController {
  constructor() {
    this.signal = new CiwiTestAbortSignal();
  }
  abort() {
    if (this.signal.aborted) return;
    this.signal.aborted = true;
    for (const listener of this.signal.listeners.slice()) listener();
  }
};
let ciwiTestUUID = 0;
globalThis.crypto = {
  randomUUID: () => '00000000-0000-4000-8000-' + String(++ciwiTestUUID).padStart(12, '0'),
};
globalThis.window = {
  ciwiTestEvents: [],
  ciwiTestSnackbars: [],
  dispatchEvent: event => window.ciwiTestEvents.push(event),
  showSnackbar: value => window.ciwiTestSnackbars.push(value),
};
globalThis.fetch = async () => ({
  ok: true,
  json: async () => ({ actions: globalThis.__ciwiTestActions }),
});
globalThis.ciwiTestDeferred = () => {
  let resolve;
  let reject;
  const promise = new Promise((resolveValue, rejectValue) => {
    resolve = resolveValue;
    reject = rejectValue;
  });
  return { promise, resolve, reject };
};
globalThis.ciwiTestElement = () => {
  const attributes = new Map();
  const classes = new Set();
  return {
    innerHTML: '<span>Run</span>',
    textContent: 'Run',
    disabled: false,
    classList: {
      add: value => classes.add(value),
      remove: value => classes.delete(value),
      contains: value => classes.has(value),
    },
    setAttribute: (key, value) => attributes.set(key, value),
    removeAttribute: key => attributes.delete(key),
    getAttribute: key => attributes.get(key),
  };
};
`
	harness := &browserActionHarness{runtime: runtime}
	harness.run(t, harnessSource+string(runnerSource))
	return harness
}

func (h *browserActionHarness) run(t *testing.T, source string) goja.Value {
	t.Helper()
	value, err := h.runtime.RunString(source)
	if err != nil {
		t.Fatalf("browser JavaScript failed: %v", err)
	}
	return value
}

func (h *browserActionHarness) value(t *testing.T, expression string) goja.Value {
	t.Helper()
	return h.run(t, "("+expression+")")
}

func (h *browserActionHarness) promise(t *testing.T, expression string) *goja.Promise {
	t.Helper()
	exported := h.value(t, expression).Export()
	promise, ok := exported.(*goja.Promise)
	if !ok {
		t.Fatalf("%s exported as %T, want *goja.Promise", expression, exported)
	}
	return promise
}

func requireBrowserPromise(t *testing.T, promise *goja.Promise, state goja.PromiseState, result string) {
	t.Helper()
	if promise.State() != state {
		t.Fatalf("promise state = %v, want %v (result %v)", promise.State(), state, promise.Result())
	}
	if result != "" && !strings.Contains(promise.Result().String(), result) {
		t.Fatalf("promise result = %q, want it to contain %q", promise.Result().String(), result)
	}
}

func TestBrowserActionRunnerBehavior(t *testing.T) {
	t.Run("confirmation policy honors cancellation and message fallback", func(t *testing.T) {
		harness := newBrowserActionHarness(t)
		harness.run(t, `
window.ciwiTestConfirmations = [];
window.confirm = message => {
  window.ciwiTestConfirmations.push(message);
  return message === 'Proceed';
};
window.ciwiNoConfirmation = window.ciwiConfirmAction(null);
window.ciwiAcceptedConfirmation = window.ciwiConfirmAction({message: 'Proceed'});
window.ciwiCancelledConfirmation = window.ciwiConfirmAction({title: 'Cancel this'});
`)
		for expression, expected := range map[string]bool{
			"window.ciwiNoConfirmation":                                        true,
			"window.ciwiAcceptedConfirmation":                                  true,
			"window.ciwiCancelledConfirmation":                                 false,
			"window.ciwiTestConfirmations.join('|') === 'Proceed|Cancel this'": true,
		} {
			if actual := harness.value(t, expression).ToBoolean(); actual != expected {
				t.Errorf("%s = %v, want %v", expression, actual, expected)
			}
		}
	})

	t.Run("mutation coalesces duplicates without replacing or restoring rendered children", func(t *testing.T) {
		harness := newBrowserActionHarness(t)
		harness.run(t, `
globalThis.probeButton = ciwiTestElement();
globalThis.probeGate = ciwiTestDeferred();
globalThis.probeExecutions = 0;
globalThis.probeKey = '';
globalThis.probeRefreshOnSuccess = false;
globalThis.probeFirst = window.ciwiRunAction('mutate', { id: 7 }, probeButton, runtime => {
  probeExecutions += 1;
  probeKey = runtime.idempotencyKey;
  probeRefreshOnSuccess = runtime.refreshOnSuccess;
  return probeGate.promise;
});
globalThis.probeSecond = window.ciwiRunAction('mutate', { id: 7 }, probeButton, () => {
  probeExecutions += 1;
  return Promise.resolve('unexpected');
});
`)
		if executions := harness.value(t, "probeExecutions").ToInteger(); executions != 1 {
			t.Fatalf("executions = %d, want 1", executions)
		}
		if key := harness.value(t, "probeKey").String(); !browserActionIDPattern.MatchString(key) {
			t.Fatalf("idempotency key = %q", key)
		}
		for expression, expected := range map[string]bool{
			"probeButton.disabled":                                               true,
			"probeButton.getAttribute('aria-busy') === 'true'":                   true,
			"probeButton.classList.contains('ciwi-action-pending')":              true,
			"probeButton.textContent === 'Run'":                                  true,
			"probeButton.innerHTML === '<span>Run</span>'":                       true,
			"probeButton.getAttribute('data-ciwi-pending-label') === 'Working…'": true,
			"window.ciwiActiveOperations().length === 1":                         true,
			"probeRefreshOnSuccess":                                              true,
		} {
			if actual := harness.value(t, expression).ToBoolean(); actual != expected {
				t.Errorf("%s = %v, want %v", expression, actual, expected)
			}
		}
		harness.run(t, `
probeButton.innerHTML = '<span>Updated by render</span>';
probeButton.textContent = 'Updated by render';
probeButton.__ciwiRenderedDisabled = true;
probeGate.resolve('done');
`)
		requireBrowserPromise(t, harness.promise(t, "probeFirst"), goja.PromiseStateFulfilled, "done")
		requireBrowserPromise(t, harness.promise(t, "probeSecond"), goja.PromiseStateFulfilled, "done")
		for _, expression := range []string{
			"probeButton.disabled === true",
			"probeButton.getAttribute('aria-busy') === undefined",
			"!probeButton.classList.contains('ciwi-action-pending')",
			"probeButton.getAttribute('data-ciwi-pending-label') === undefined",
			"probeButton.innerHTML === '<span>Updated by render</span>'",
			"window.ciwiActiveOperations().length === 0",
		} {
			if !harness.value(t, expression).ToBoolean() {
				t.Errorf("restored state assertion failed: %s", expression)
			}
		}
	})

	t.Run("conflicting mutations are rejected and reported", func(t *testing.T) {
		harness := newBrowserActionHarness(t)
		harness.run(t, `
globalThis.probeGate = ciwiTestDeferred();
globalThis.probeFirst = window.ciwiRunAction('mutate', { id: 7 }, ciwiTestElement(), () => probeGate.promise);
globalThis.probeConflict = window.ciwiRunAction('other-mutation', { id: 7 }, ciwiTestElement(), () => Promise.resolve('unexpected'));
`)
		requireBrowserPromise(t, harness.promise(t, "probeConflict"), goja.PromiseStateRejected, "Working")
		if count := harness.value(t, "window.ciwiTestSnackbars.length").ToInteger(); count != 0 {
			t.Fatalf("snackbar count = %d, want 0; the caller owns failure presentation", count)
		}
		harness.run(t, `probeGate.resolve('done')`)
		requireBrowserPromise(t, harness.promise(t, "probeFirst"), goja.PromiseStateFulfilled, "done")
	})

	t.Run("new query supersedes the previous query in its scope", func(t *testing.T) {
		harness := newBrowserActionHarness(t)
		harness.run(t, `
globalThis.probeFirstAborted = false;
globalThis.probeFirst = window.ciwiRunAction('query-a', {}, ciwiTestElement(), ({ signal }) => new Promise((_, reject) => {
  signal.addEventListener('abort', () => {
    probeFirstAborted = true;
    reject(new Error('aborted'));
  });
}));
`)
		harness.run(t, `
globalThis.probeSecond = window.ciwiRunAction('query-b', {}, ciwiTestElement(), () => Promise.resolve('fresh'));
`)
		requireBrowserPromise(t, harness.promise(t, "probeFirst"), goja.PromiseStateRejected, "aborted")
		requireBrowserPromise(t, harness.promise(t, "probeSecond"), goja.PromiseStateFulfilled, "fresh")
		if !harness.value(t, "probeFirstAborted").ToBoolean() {
			t.Fatal("superseded query was not aborted")
		}
	})

	t.Run("failure restores state and headers carry idempotency identity", func(t *testing.T) {
		harness := newBrowserActionHarness(t)
		harness.run(t, `
globalThis.probeButton = ciwiTestElement();
globalThis.probeFailure = window.ciwiRunAction('mutate', { id: 9 }, probeButton, async () => {
  throw new Error('failed');
});
globalThis.probeHeaders = window.ciwiActionHeaders(
  { idempotencyKey: 'command-1' },
  { Accept: 'application/json' },
);
`)
		requireBrowserPromise(t, harness.promise(t, "probeFailure"), goja.PromiseStateRejected, "failed")
		for _, expression := range []string{
			"probeButton.disabled === false",
			"window.ciwiActiveOperations().length === 0",
			"probeHeaders.Accept === 'application/json'",
			"probeHeaders['Idempotency-Key'] === 'command-1'",
		} {
			if !harness.value(t, expression).ToBoolean() {
				t.Errorf("failure/header assertion failed: %s", expression)
			}
		}
	})
}
