# Web and native UI parity

The established web UI is the current interaction reference. The shared
`ui/screens/*.yaml` documents and presentation bindings are the long-term UI
contract; renderer differences belong in the browser and Gio adapters rather
than in duplicated screen definitions.

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
| Global settings | every theme; project action feedback; update/rollback empty/loading/available/error |
| Connection | discovery/explicit endpoint; connecting/connected/reconnecting/error |

On macOS, `scripts/capture-ui-parity-macos.sh <surface>` captures a native and
browser window interactively into `/tmp/ciwi-ui-parity`. Screenshots are review
artifacts, not pixel-exact golden tests.

## Implementation order

1. Shared visual metrics and renderer primitives.
2. Front page structure and execution cards.
3. Project details and graph/list behavior.
4. Job details, execution timeline, and structured output.
5. Settings, agents, run options, connection, and exceptional states.
6. Promote shared declarative screens from browser preview to established
   routes once behavior is complete enough to remove the duplicated view.

For every slice, test the shared contract, both renderers' binding behavior,
text selection, keyboard focus, scrolling, disclosure persistence, dialogs,
and live refresh behavior before relying on screenshot review.

## Current checkpoint

The project-details screen declares its pipeline dependency graph once through
the shared `graph-view` component. Browser and native renderers both provide a
persistent Graph/List switch, fit/reset/zoom controls, two-dimensional overflow,
selectable node copy, dependency edges, and per-pipeline run actions. The List
mode remains the complete pipeline/job/step hierarchy while graph drill-down to
jobs and steps is developed as a later parity slice.
