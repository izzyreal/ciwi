package webui

import (
	"testing"

	"github.com/dop251/goja"
)

func TestBrowserViewStateRestoresNestedScrollPageScrollAndFocus(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/view-state.js")
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	_, err = runtime.RunString(`
const oldScroller = {id: 'output', scrollTop: 240, scrollLeft: 11};
const oldEditor = {id: 'editor', selectionStart: 7, selectionEnd: 12, selectionDirection: 'forward'};
const newScroller = {id: 'output', scrollTop: 0, scrollLeft: 0};
const newEditor = {
  id: 'editor', focused: false, selection: null,
  focus: options => { newEditor.focused = options && options.preventScroll === true; },
  setSelectionRange: (start, end, direction) => { newEditor.selection = {start, end, direction}; },
};
const oldRoot = {
  querySelectorAll: () => [oldScroller, oldEditor],
  contains: element => element === oldScroller || element === oldEditor,
};
const newRoot = {contains: element => element === newScroller || element === newEditor};
const elements = {output: newScroller, editor: newEditor};
globalThis.document = {activeElement: oldEditor, getElementById: id => elements[id]};
globalThis.window = {
  scrollX: 19, scrollY: 470, restoredPage: null,
  scrollTo: (left, top) => { window.restoredPage = {left, top}; },
};
` + string(payload) + `
const snapshot = window.ciwiCaptureViewState(oldRoot);
window.ciwiRestoreViewState(newRoot, snapshot);
`)
	if err != nil {
		t.Fatal(err)
	}
	assertions := []string{
		"newScroller.scrollTop === 240 && newScroller.scrollLeft === 11",
		"newEditor.focused === true",
		"newEditor.selection.start === 7 && newEditor.selection.end === 12 && newEditor.selection.direction === 'forward'",
		"window.restoredPage.left === 19 && window.restoredPage.top === 470",
	}
	for _, assertion := range assertions {
		value, evalErr := runtime.RunString("(" + assertion + ")")
		if evalErr != nil {
			t.Fatal(evalErr)
		}
		if !value.ToBoolean() {
			t.Errorf("browser view-state assertion failed: %s", assertion)
		}
	}
}

func TestBrowserViewStateIgnoresRemovedFocusedAndScrollableElements(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/view-state.js")
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	_, err = runtime.RunString(`
const oldElement = {id: 'removed', scrollTop: 10, scrollLeft: 0, selectionStart: 1, selectionEnd: 1};
const oldRoot = {querySelectorAll: () => [oldElement], contains: element => element === oldElement};
const newRoot = {contains: () => false};
globalThis.document = {activeElement: oldElement, getElementById: () => null};
globalThis.window = {scrollX: 0, scrollY: 0, scrollTo: () => {}};
` + string(payload) + `
const snapshot = window.ciwiCaptureViewState(oldRoot);
window.ciwiRestoreViewState(newRoot, snapshot);
`)
	if err != nil {
		t.Fatalf("removed elements must not break view-state restoration: %v", err)
	}
}

func TestBrowserDisclosureStatePersistsAcrossRendererReplacement(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/view-state.js")
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	_, err = runtime.RunString(`
const stored = new Map([['ciwi.declarative.disclosures.v1', '{"step:1":true}']]);
globalThis.localStorage = {
  getItem: key => stored.get(key),
  setItem: (key, value) => stored.set(key, value),
};
globalThis.document = {};
globalThis.window = {};
` + string(payload) + `
const initiallyExpanded = window.ciwiDisclosureState.get('step:1', false);
const defaultExpanded = window.ciwiDisclosureState.get('step:2', true);
window.ciwiDisclosureState.set('step:1', false);
const persistedSnapshot = JSON.parse(stored.get('ciwi.declarative.disclosures.v1'));
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, assertion := range []string{
		"initiallyExpanded === true",
		"defaultExpanded === true",
		"window.ciwiDisclosureState.has('step:1') === true",
		"window.ciwiDisclosureState.get('step:1', true) === false",
		"persistedSnapshot['step:1'] === false",
	} {
		value, evalErr := runtime.RunString("(" + assertion + ")")
		if evalErr != nil {
			t.Fatal(evalErr)
		}
		if !value.ToBoolean() {
			t.Errorf("browser disclosure assertion failed: %s", assertion)
		}
	}
}
