# Web and native UI parity

The shared `ui/screens/*.yaml` documents, `ui/routes.yaml`, and presentation
bindings are the authoritative UI contract. Renderer differences belong in
the browser and Gio adapters rather than in duplicated screen definitions.

Parity means matching information hierarchy, responsive layout, interaction
semantics, state feedback, and visual rhythm. It does not require identical
font rasterization or replacing useful platform-native behavior.

## Comparison setup

Use the same server, theme, and disclosure state in both clients. The native
window defaults to 1180 × 780. Give the browser an approximately 1180 × 780
content viewport and compare these states:

| Surface | Required states |
| --- | --- |
| Front page | no projects; project collapsed/expanded; queued/waiting/running; successful/failed history; confirmation dialog |
| Project details | pipeline list/graph; pipeline and job collapsed/expanded; run options |
| Job details | queued/running/succeeded/failed; reached/unreached output; search and tailing; confirmation dialog |
| Agents | empty/offline/online; agent collapsed/expanded; destructive confirmation |
| Global settings | every theme; repository/managed-YAML actions; direct/SSH connection; update/rollback empty/loading/available/error |
| Agent details/script | authorized/deactivated/offline/busy; every advertised shell; queued/error notice |
| Connection | discovery/explicit endpoint; connecting/connected/reconnecting/error; untrusted/changed SSH host key |

On macOS, `scripts/capture-ui-parity-macos.sh <surface>` captures a native and
browser window interactively into `/tmp/ciwi-ui-parity`. Screenshots are review
artifacts, not pixel-exact golden tests.

## Maintenance order

1. Shared visual metrics and renderer primitives.
2. Front page structure and execution cards.
3. Project details and graph/list behavior.
4. Job details, execution timeline, and structured output.
5. Settings, agents, run options, connection, and exceptional states.
6. Verify binding conformance, topic-scoped invalidation, and both renderers
   before changing an established screen.

For every slice, test the shared contract, both renderers' binding behavior,
text selection, keyboard focus, scrolling, disclosure persistence, dialogs,
and live refresh behavior before relying on screenshot review.

## Current state

The established browser routes and the Gio client now consume the shared
declarations. Vault is available over both HTTP and typed CNP operations. The
former hand-authored browser pages and `/declarative-preview` route have been
removed.

Routes, screens, action semantics, theme tokens, typography, control geometry,
and presentation labels each have one shared authority. Browser theme CSS is
generated from the shared theme documents, and screen commands are checked
against the shared action catalog during bundle loading. Data-source
declarations contain only binding roots and invalidation topics; transport
loaders remain explicit adapter code.

The project-details screen declares its nested pipeline and job dependency
graphs once through the shared `graph-view` component. Browser and native
renderers both provide persistent Graph/List switches, fit/reset/zoom controls,
two-dimensional overflow, selectable node copy, dependency edges, per-node run
actions, and selected-job step details. The List mode remains the complete
pipeline/job/step fallback.

The front-page contract now uses the same link and disclosure semantics in both
renderers: project names and execution job names navigate directly, surrounding
summary rows disclose their content, and selecting text does neither. Queue and
history details use compact horizontal job rows rather than renderer-specific
cards. Project icons and pipeline content are ordinary shared row/column/image
nodes rather than renderer-composed project bodies. Browser and native
code/identifier text share the bundled Geist Mono face; ordinary UI and
control text share the bundled Geist Sans faces.

Job output uses theme-owned console tokens in both renderers. Its controls,
execution path, system messages, and grouped phase/step output now live in one
`Output / Error` section; selecting a path item reveals its corresponding output
instead of maintaining a second selected-item details card. Timeline cards use
the same fixed outer geometry in both adapters, and collapsed output rows use
the shared passive-chevron and border-box contracts. The complete collapsed
phase/step row expands and collapses by mouse or touch, including while nested
inside the output scroller. Timeline selection changes selection and disclosure
state without creating a snackbar; bounded queue notices remain part of running
jobs, chains, pipelines, and ad-hoc scripts.

Job reports use one recursive `tree-view` declaration and one presentation
model in both clients. Artifact directories expose ZIP downloads at every
level, individual files expose direct downloads, test suites retain package and
case drill-down plus repository source links and status filters, and coverage
paths wrap within their card. Browser downloads use the established HTTP
endpoints; native downloads use bounded chunks over typed CNP and save without
overwriting an existing file.

Settings, managed-YAML editing, agent details, and ad-hoc agent scripts now use
the same presentation/action contracts. Both adapters expose asynchronous busy
state and bounded success/error notices; agent heartbeat icons use the shared
event-timestamp pulse binding. Select triggers and options use the exact shared
outer heights and menu metrics, and both adapters allow only one select popup
at a time. Popup width follows the widest trigger/option content within the
viewport, and any outside press dismisses it even when another component owns
that press. A successful agent deletion replaces the removed details route with
the shared Agents route; its expected change invalidation is not reported as a
failed stale-data refresh. Compact overrides provide narrow-width reflow
and touch scrolling without separate mobile screen definitions. Their width
boundaries are declared once in `ui/controls.yaml` and apply identically in a
browser, desktop window, tablet, or phone; platform and orientation do not
participate in responsive classification.

Tests enforce the renderer boundary: populated browser binding fixtures cover
every shared route, every web-visible command must have a browser adapter, and
platform-specific overrides are restricted to the native connection panel and
the browser's sticky output-collapse affordance. This prevents cosmetic layout
or content forks from being added silently.

Renderer-independent labels and semantic states for execution cards and
project structure are produced by `internal/presentation` and carried by the
HTTP view contract or derived from the same helpers by the typed CNP adapter.
The browser no longer recomputes those rules in JavaScript. Adapter-owned state
is deliberately limited to transport representation and interaction state such
as project-icon bytes versus URLs, graph/list selection, output search and
tailing, connection settings, and pending notices.

Lifecycle regression tests exercise browser page and bidirectional nested
scroll chaining, focused-control selection, persistent disclosures,
confirmation cancellation, exact shared card/collapsed-row geometry,
width-boundary reflow, and incremental output refresh. Gio DOM tests cover
keyed state retention, state pruning, bounded virtualization, width-only
responsive geometry, interactive child taps and touch drags, bidirectional
boundary handoff, and scroll-anchor reconciliation independently of ciwi
screens. These deterministic regressions run once; repetition is reserved for
an explicitly diagnosed timing or ordering problem. Native interaction and
visual parity remain device-level checks.

The browser adapter is divided into the screen renderer, view-binding
decoration, select control, graph/tree renderers, and DOM reconciler. The Gio
adapter separates application control flow and bindings from core DOM
compilation, element/control compilation, and disclosure/graph/scroller
compilation. Shared screen documents remain whole by product surface: their
size is preferable to introducing a parameterized fragment language before a
stable repeated component actually warrants one.

## Cutover footprint and performance

As recorded at the original cutover, relative to `main` at `4493cf2`, the
declarative branch deleted 31 files and removed approximately 9,300 net lines.
Five-run Apple M1 benchmarks
show unchanged allocations and no CPU regression: the native collapsed front
page is about 13% faster, native collapsed Project Details about 1% faster, and
the declarative browser route benchmark is effectively unchanged from the first
cutover checkpoint. These are microbenchmarks rather than a process-RSS claim;
the renderer allocation counts are the repeatable memory signal covered here.
