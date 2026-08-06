package webui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/izzyreal/ciwi/internal/presentation"
)

func TestBrowserNoticeDefaultsMatchSharedPresentationContract(t *testing.T) {
	for _, want := range []string{
		fmt.Sprintf("const ciwiSnackbarCapacity = %d;", presentation.TransientNoticeCapacity),
		fmt.Sprintf("options.timeoutMs || %d", presentation.TransientNoticeDuration.Milliseconds()),
	} {
		if !strings.Contains(sharedJS, want) {
			t.Errorf("browser notice implementation is missing shared contract %q", want)
		}
	}
}

func TestChangedBrowserAssetsCompile(t *testing.T) {
	for _, path := range []string{
		"assets/js/shared.js", "assets/js/actions.js", "assets/js/agent.js", "assets/js/settings.js", "assets/js/theme.js", "assets/js/declarative.js",
	} {
		source, err := uiAssets.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := goja.Compile(path, string(source), false); err != nil {
			t.Errorf("%s does not compile: %v", path, err)
		}
	}
}

func newSnackbarHarness(t *testing.T) *goja.Runtime {
	t.Helper()
	runtime := goja.New()
	source := `
let ciwiNow = 1000;
let ciwiTimerID = 0;
const ciwiTimers = new Map();
globalThis.Date.now = () => ciwiNow;
globalThis.setTimeout = (callback, delay) => {
  const id = ++ciwiTimerID;
  ciwiTimers.set(id, { callback, delay, cleared: false });
  return id;
};
globalThis.clearTimeout = id => {
  const timer = ciwiTimers.get(id);
  if (timer) timer.cleared = true;
};
function ciwiElement(tagName) {
  return {
    tagName: String(tagName || '').toUpperCase(), id: '', className: '', textContent: '', innerHTML: '',
    children: [], parentNode: null, attributes: {}, listeners: {},
    appendChild(child) { child.parentNode = this; this.children.push(child); return child; },
    removeChild(child) { this.children = this.children.filter(value => value !== child); child.parentNode = null; },
    setAttribute(name, value) { this.attributes[name] = String(value); },
    getAttribute(name) { return this.attributes[name]; },
    addEventListener(name, callback) { (this.listeners[name] ||= []).push(callback); },
    contains(candidate) {
      if (candidate === this) return true;
      return this.children.some(child => child.contains(candidate));
    },
  };
}
const ciwiHead = ciwiElement('head');
const ciwiBody = ciwiElement('body');
function ciwiFindByID(node, id) {
  if (node.id === id) return node;
  for (const child of node.children) {
    const match = ciwiFindByID(child, id);
    if (match) return match;
  }
  return null;
}
globalThis.document = {
  head: ciwiHead, body: ciwiBody, activeElement: null,
  createElement: ciwiElement,
  getElementById(id) { return ciwiFindByID(ciwiHead, id) || ciwiFindByID(ciwiBody, id); },
  querySelectorAll() { return []; },
  addEventListener() {},
};
globalThis.window = {
  location: { pathname: '/', hash: '', href: '', assign(value) { this.href = value; } },
  setInterval() { return 1; },
  addEventListener() {},
};
`
	sharedSource, err := uiAssets.ReadFile("assets/js/shared.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunString(source + string(sharedSource)); err != nil {
		t.Fatalf("load snackbar browser harness: %v", err)
	}
	return runtime
}

func snackbarValue(t *testing.T, runtime *goja.Runtime, expression string) goja.Value {
	t.Helper()
	value, err := runtime.RunString("(" + expression + ")")
	if err != nil {
		t.Fatalf("evaluate %s: %v", expression, err)
	}
	return value
}

func snackbarRun(t *testing.T, runtime *goja.Runtime, source string) {
	t.Helper()
	if _, err := runtime.RunString(source); err != nil {
		t.Fatalf("run snackbar behavior: %v", err)
	}
}

func TestSnackbarQueuesOneNoticeAtATime(t *testing.T) {
	runtime := newSnackbarHarness(t)
	snackbarRun(t, runtime, `
showSnackbar({message:'First'});
showSnackbar({message:'Second'});
showSnackbar({message:'Third'});
showSnackbar({message:'Second'});
`)
	for expression, want := range map[string]int64{
		"document.getElementById('ciwiSnackbarHost').children.length": 1,
		"ciwiSnackbarState.queue.length":                              2,
	} {
		if got := snackbarValue(t, runtime, expression).ToInteger(); got != want {
			t.Errorf("%s = %d, want %d", expression, got, want)
		}
	}
	if got := snackbarValue(t, runtime, "ciwiSnackbarState.active.message").String(); got != "First" {
		t.Fatalf("active message = %q, want First", got)
	}
	snackbarRun(t, runtime, `ciwiSnackbarState.active.node.children[1].children[0].onclick()`)
	if got := snackbarValue(t, runtime, "ciwiSnackbarState.active.message").String(); got != "Second" {
		t.Fatalf("active message after dismiss = %q, want Second", got)
	}
}

func TestSnackbarActionAndAccessibilityContract(t *testing.T) {
	runtime := newSnackbarHarness(t)
	snackbarRun(t, runtime, `
globalThis.ciwiNoticeActionCount = 0;
showSnackbar({message:'Queued', actionLabel:'Show queued jobs', onAction:() => { ciwiNoticeActionCount += 1; }});
showSnackbar({message:'Following'});
`)
	for expression, want := range map[string]string{
		"document.getElementById('ciwiSnackbarHost').getAttribute('aria-live')": "polite",
		"ciwiSnackbarState.active.node.getAttribute('role')":                    "status",
	} {
		if got := snackbarValue(t, runtime, expression).String(); got != want {
			t.Errorf("%s = %q, want %q", expression, got, want)
		}
	}
	snackbarRun(t, runtime, `ciwiSnackbarState.active.node.children[1].children[0].onclick()`)
	if got := snackbarValue(t, runtime, "ciwiNoticeActionCount").ToInteger(); got != 1 {
		t.Fatalf("action count = %d, want 1", got)
	}
	if got := snackbarValue(t, runtime, "ciwiSnackbarState.active.message").String(); got != "Following" {
		t.Fatalf("active message after action = %q, want Following", got)
	}
}

func TestSnackbarQueueIsBounded(t *testing.T) {
	runtime := newSnackbarHarness(t)
	snackbarRun(t, runtime, `
['One','Two','Three','Four','Five','Six'].forEach(message => showSnackbar({message}));
`)
	if got := snackbarValue(t, runtime, "1 + ciwiSnackbarState.queue.length").ToInteger(); got != 4 {
		t.Fatalf("total retained notices = %d, want 4", got)
	}
	if got := snackbarValue(t, runtime, "ciwiSnackbarState.queue.map(value => value.message).join(',')").String(); got != "Four,Five,Six" {
		t.Fatalf("queued notices = %q, want latest waiting notices", got)
	}
}
