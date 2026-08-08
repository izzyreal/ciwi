package webui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/izzyreal/ciwi/internal/presentation"
)

func TestBrowserNoticeContractMatchesSharedPresentation(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/notices.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(payload)
	for _, expected := range []string{
		fmt.Sprintf("const capacity = %d;", presentation.TransientNoticeCapacity),
		fmt.Sprintf("raw.timeout_ms || %d", presentation.TransientNoticeDuration.Milliseconds()),
	} {
		if !strings.Contains(source, expected) {
			t.Errorf("browser notice implementation is missing %q", expected)
		}
	}
}

func TestBrowserNoticesQueueDeduplicateAndAdvance(t *testing.T) {
	payload, err := uiAssets.ReadFile("assets/js/notices.js")
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	harness := `
function element(tag) {
  return {
    tagName: tag, id: '', className: '', textContent: '', children: [], parentNode: null,
    attributes: {}, listeners: {},
    append(...nodes) { nodes.forEach(node => { node.parentNode = this; this.children.push(node); }); },
    appendChild(node) { this.append(node); return node; },
    remove() { if (this.parentNode) this.parentNode.children = this.parentNode.children.filter(node => node !== this); },
    setAttribute(name, value) { this.attributes[name] = String(value); },
    addEventListener(name, listener) { (this.listeners[name] ||= []).push(listener); },
    contains(node) { return node === this || this.children.some(child => child.contains(node)); },
  };
}
const body = element('body');
function find(node, id) { if (node.id === id) return node; for (const child of node.children) { const match = find(child, id); if (match) return match; } return null; }
globalThis.document = {body, activeElement:null, createElement:element, getElementById:id => find(body,id)};
let timerID = 0;
globalThis.window = {
  setTimeout: () => ++timerID, clearTimeout: () => {},
  location: {assign: () => {}},
};
`
	if _, err := runtime.RunString(harness + string(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunString(`
window.ciwiShowNotice({message:'First'});
window.ciwiShowNotice({message:'Second'});
window.ciwiShowNotice({message:'Second'});
`); err != nil {
		t.Fatal(err)
	}
	for expression, want := range map[string]int64{
		"window.ciwiNoticeState.queue.length":                         1,
		"document.getElementById('ciwiSnackbarHost').children.length": 1,
	} {
		value, evalErr := runtime.RunString(expression)
		if evalErr != nil {
			t.Fatal(evalErr)
		}
		if got := value.ToInteger(); got != want {
			t.Errorf("%s = %d, want %d", expression, got, want)
		}
	}
	if _, err := runtime.RunString(`window.ciwiNoticeState.active.node.children[1].children[0].listeners.click[0]()`); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString("window.ciwiNoticeState.active.message")
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); got != "Second" {
		t.Fatalf("active notice after dismiss = %q, want Second", got)
	}
}
