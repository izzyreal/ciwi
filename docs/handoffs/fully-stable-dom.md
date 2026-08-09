# Handoff: A fully stable browser DOM

Status: implemented

The browser-DOM characterization suite is intentionally isolated under
`integration/browser-dom`; the committed Go unit-test suite does not require
Node, npm, Playwright, or a browser.

The final follow-up also retains the literal graph viewport element (including
nested graph viewports) while rebuilding graph data beneath it. Viewport scroll
is left to the mounted DOM rather than copied into renderer state. The isolated
suite additionally covers periodically refreshed agent-log scroll, selections
that span multiple rendered elements, shared table/badge/control metrics, and
the server-update confirmation/reset flow.

## Objective

Make live view-model updates patch the existing browser UI in place. A semantic
UI node that still exists after an update should retain the same DOM identity,
including through SSE-driven refreshes. Route changes and genuinely incompatible
component changes may replace nodes.

This is browser-renderer work. The shared UI DSL, presentation models, action
semantics, and Gio renderer should not acquire DOM concepts.

## Why this matters

The declarative browser renderer currently builds a complete new screen tree in
`renderNode` and commits it in `renderCurrent` with `root.replaceChildren(...)`.
It captures and restores some scroll and focus state around that replacement,
but the mounted DOM is still new.

That has caused or contributed to several visible problems:

- CSS animations restart during live updates;
- focused inputs, selection, hover, and open controls can be disturbed;
- browser dropdowns may close while unrelated text changes;
- pointer and scrolling interactions can be interrupted;
- state restoration and component-specific patch paths accumulate as fixes for
  behavior the DOM would otherwise retain naturally.

Commit `65b977d` fixes the front-page execution spinner by assigning the status
SVG a stable execution key and transplanting the existing SVG into the newly
rendered tree. That is a safe narrow fix and a useful seam, but it is not a
complete reconciler.

## Current seams

The following pieces already provide useful foundations:

- UI repetitions declare `repeat.key` in the shared screen contracts.
- Disclosures can declare a templated `stateKey`.
- Static nodes may have explicit IDs.
- `renderNode` is the central browser mapping from DSL node and binding data to
  a DOM element.
- `renderCurrent` is the central full-screen commit point.
- `patchJobOutputRegion` proves that a bounded region can already be updated
  without replacing the screen root, although it still replaces group children.
- `ciwiCaptureViewState` and `ciwiRestoreViewState` provide a fallback for route
  transitions and incompatible replacements.
- The progress renderer and Gio loader already separate semantic state from
  animation paint.

The main missing seam is a renderer-owned identity and reconciliation layer.
The browser currently does not use repeat keys to retain repeated DOM nodes, and
event listeners close over the binding data supplied when a node was created.
Retaining an interactive element without also updating its action data would
therefore invoke stale state.

## Target behavior

For a refresh of the same route and screen:

1. Matching nodes keep their DOM identity.
2. Text, classes, attributes, ARIA state, form properties, and styles update in
   place.
3. Keyed repeated children are inserted, removed, or moved without replacing
   unaffected siblings.
4. Event handlers observe the newest binding data and are never duplicated.
5. Native browser state survives naturally: focus, text selection, open
   dropdowns, hover, pointer capture, scroll position, and CSS animation phase.
6. A component or key change replaces only that incompatible subtree.
7. Navigating to another route may replace the screen root while preserving
   explicitly shared shell elements if those are introduced later.

“Stable” refers to the mounted DOM. An implementation may initially build a
detached next tree or lightweight render description, provided matching mounted
nodes are patched rather than replaced. Avoid optimizing allocation until it is
measured.

## Identity rules

Identity must be deterministic and scoped to its parent:

1. A repeated node uses the resolved `repeat.key` plus its repeat scope.
2. A disclosure uses its resolved `stateKey` when available.
3. An explicit DSL node ID identifies a non-repeated node within the screen.
4. Static, unkeyed children use their structural contract path.
5. Component type and expected HTML tag are part of compatibility; changing
   either replaces the node even when the logical key is unchanged.

Dynamic collections should ultimately require a non-empty, unique repeat key.
Validation should reject duplicate resolved sibling keys in development and
tests. Indexes are acceptable only for immutable, non-reordered static content.

DOM identity belongs to the browser adapter. Do not expose generated DOM keys in
presentation view models or CNP/HTTP contracts.

## Recommended implementation

Build a narrow DSL-aware reconciler rather than introducing React, a generic
virtual-DOM framework, or DOM semantics into `pkg/uidsl`.

The most incremental route is:

1. Teach `renderNode` to annotate generated elements with renderer-owned node
   identity and component metadata.
2. Refactor action binding so a retained element reads its current action
   descriptor and current binding scope. A root-delegated handler is suitable
   for ordinary click, input, change, and keyboard actions. Stateful complex
   widgets may keep dedicated handlers behind explicit lifecycle hooks.
3. Render the next tree detached, then reconcile it with the mounted tree:
   patch compatible elements, reconcile keyed children, and mount only new or
   incompatible nodes.
4. Give complex components (`graph-view`, `tree-view`, custom select, output
   scroller) explicit update/dispose hooks where ordinary attribute and child
   reconciliation is insufficient.
5. Once behavior is established, consider rendering lightweight descriptions
   instead of detached DOM if profiling shows the allocations matter.

The reconciler should be small and renderer-specific. It only needs the HTML
shapes produced by ciwi's DSL; it does not need component functions, effects,
server rendering, hydration, or a public extension API.

## Delivery slices

### Slice 1: Characterization and identity

- Add browser tests that retain references to representative DOM nodes across
  a same-route data refresh.
- Cover a focused text input, an open select, an active execution spinner, an
  expanded disclosure, and keyed repeated rows.
- Generate and validate stable browser node keys without changing commit
  behavior yet.
- Inventory repeats without reliable keys and fix their contracts.

### Slice 2: Fresh action data

- Stop ordinary action listeners from permanently closing over render-time
  binding objects.
- Store or resolve the latest action descriptor and binding scope for retained
  controls.
- Verify that a retained button acts on newly refreshed IDs and values and that
  handlers do not fire more than once after many updates.

This slice must precede retaining interactive elements broadly.

### Slice 3: Primitive and keyed reconciliation

- Patch text, icons, images, badges, layout containers, buttons, inputs, and
  native select elements in place.
- Reconcile keyed list children, including moves and deletions.
- Preserve form value and selection when the backing value has not changed;
  apply an authoritative backing-value change deliberately when it has.
- Replace only incompatible component/tag pairs.

### Slice 4: Stateful browser components

- Reconcile disclosures without toggling them as a side effect.
- Keep the custom dropdown open while unrelated bindings update; update its
  choices and selected description in place.
- Preserve graph viewport and gesture state while graph data changes.
- Preserve output scroll intent, selection, and search focus while appending
  streamed output.
- Add disposal for removed component listeners, observers, and transient
  portals.

### Slice 5: Remove compensating paths

- Make `renderCurrent` reconcile by default for same-screen refreshes.
- Keep full root replacement for route/screen incompatibility and fatal render
  recovery.
- Fold or narrow `patchJobOutputRegion` so there is one normal update model.
- Remove the spinner-only `data-ciwi-stable-key` transplant once general keyed
  reconciliation preserves it.
- Reduce capture/restore to route transitions and unavoidable subtree
  replacements.
- Revisit animation-delay workarounds individually; keep any that intentionally
  preserve phase across actual unmount/remount rather than removing them
  mechanically.

## Verification

Automated browser tests should establish these invariants:

- `previousElement === currentElement` for a retained semantic node;
- keyed rows retain identity when siblings are inserted, deleted, or reordered;
- a status change replaces the loader with the correct terminal icon;
- an open dropdown remains open after an unrelated model update;
- a focused input retains focus and selection and accepts continued typing;
- actions use the newest model data and execute exactly once;
- disclosure, page, graph, and output scroll positions remain stable;
- CSS animation time continues monotonically across SSE refreshes;
- removed components release listeners, observers, and pointer state;
- route navigation still replaces incompatible screens and restores sensible
  initial focus/scroll behavior.

Keep the existing Go contract tests and add a browser DOM harness capable of
asserting node identity and dispatching real events. String-presence tests are
useful guards for small seams but are insufficient for reconciliation behavior.

Manual smoke testing should include the front page under frequent execution
updates, theme selection, Run Options inputs, expanded execution history, job
output search/tailing, node-graph gestures, browser back/forward navigation, and
both narrow and desktop widths.

## Risks and constraints

- Stale action closures are the highest correctness risk.
- Incorrect list keys can attach state or actions to the wrong execution.
- Updating `input.value` on every refresh can overwrite in-progress user edits.
- Moving or replacing focused elements can synthesize blur/change behavior.
- Complex components need cleanup when removed to avoid leaked listeners and
  animation frames.
- Reconciliation must not mutate shared UI contracts or create renderer policy
  in presentation models.
- Do not combine this migration with cosmetic redesigns; behavior-preserving
  slices make regressions diagnosable.

## Non-goals

- Replacing the shared declarative UI DSL.
- Making Gio use DOM-like reconciliation.
- Adding a third-party frontend framework.
- Eliminating all detached allocations before profiling.
- Moving server state or transport behavior into the browser renderer.

## Suggested starting point

Begin at `renderCurrent`, `renderNode`, and `bindActions` in
`internal/server/webui/assets/js/declarative.js`. First add a real DOM identity
test around a repeated execution row, then introduce renderer node keys and
fresh action-data binding. Do not broaden the current stable-spinner transplant
until interactive retained nodes cannot observe stale data.
