# Handoff record: A fully stable browser DOM

Status: completed; retained as a historical implementation record

Completed: 2026-08-10

## Outcome

Same-route view-model updates now reconcile the mounted browser DOM instead of
replacing the screen tree. Compatible semantic nodes retain identity, keyed
children can move without being recreated, and incompatible component or icon
changes replace only the affected subtree.

The implementation remains browser-adapter code. It does not add DOM concepts
to the shared UI DSL, presentation models, or Gio renderer. The reconciler now
lives in `internal/server/webui/assets/js/dom-reconciler.js`; renderer-owned
identity is produced by the main declarative renderer.

The completed design includes:

- deterministic parent-scoped identity from repeat keys, disclosure state
  keys, explicit node IDs, and structural paths;
- delegated ordinary action events backed by the newest render session, so a
  retained control cannot invoke stale binding data;
- in-place text, attribute, ARIA, style, controlled-value, and keyed-child
  reconciliation;
- explicit update/disposal behavior for the custom select portal and graph
  viewport;
- preservation of focus, text selection, dropdown state, disclosure state,
  scroll position, and semantic progress animation phase where compatible;
- route-level replacement and view-state restoration for genuinely
  incompatible screens;
- one general reconciliation path instead of a spinner-specific transplant.

## Verification

The isolated Playwright module under `integration/browser-dom` exercises the
production browser adapter in a real Chromium DOM. It covers:

- stable status-icon identity and incompatible icon replacement;
- duplicate and empty repeat-key rejection;
- fresh action scopes and removal of stale actions;
- focused input value/selection retention;
- open custom-select retention and portal disposal;
- disclosure, tree branch, keyed row, and graph viewport identity;
- output and agent-log scroll retention plus single- and multi-node text
  selections;
- shared control/table geometry, responsive execution summaries, pending
  labels, and server-update confirmation behavior.

The browser suite is intentionally separate from `go test ./...`; setup and
commands are documented in `integration/browser-dom/README.md`.

## Ongoing rules

- New dynamic collections require non-empty stable keys.
- Retained interactive components must read current action/binding state and
  provide cleanup for external listeners, animation frames, or portals.
- DOM identity remains an adapter concern and must not enter presentation or
  transport contracts.
- New stateful components need real-browser identity/lifecycle coverage; source
  string assertions alone are not sufficient.
