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
code/identifier text share the bundled Geist Mono face.

Job output uses theme-owned console tokens in both renderers. Its controls,
execution path, system messages, and grouped phase/step output now live in one
`Output / Error` section; selecting a path item reveals its corresponding output
instead of maintaining a second selected-item details card.

Settings, managed-YAML editing, agent details, and ad-hoc agent scripts now use
the same presentation/action contracts. Both adapters expose asynchronous busy
state and bounded success/error notices; agent heartbeat icons use the shared
event-timestamp pulse binding. Compact overrides provide phone-width layouts,
touch scrolling, and full-screen disclosure sheets without separate mobile
screen definitions.

Tests enforce the renderer boundary: populated browser binding fixtures cover
every shared route, every web-visible command must have a browser adapter, and
platform-specific overrides are restricted to the native connection panel and
the browser's sticky output-collapse affordance. This prevents cosmetic layout
or content forks from being added silently.

## Cutover footprint and performance

Relative to pre-cutover `main` at `4493cf2`, the declarative branch deletes 31
files and removes approximately 9,660 net lines. Five-run Apple M1 benchmarks
show unchanged allocations and no CPU regression: the native collapsed front
page is about 13% faster, native collapsed Project Details about 1% faster, and
the declarative browser route benchmark is effectively unchanged from the first
cutover checkpoint. These are microbenchmarks rather than a process-RSS claim;
the renderer allocation counts are the repeatable memory signal covered here.
