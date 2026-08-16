# Gio DOM viability experiment

Status: adopted by the production native renderer after standalone macOS and
physical-iOS functional/resource gates passed; broader interaction and
rendered-frame profiling remain release-readiness work

## Question

Can a sustainable, professional mobile and desktop UI with near-browser parity
be built as a keyed reactive layer over Gio, or is there a material mismatch
between Gio and that product goal?

The runtime remains independent of ciwi's renderer: it imports neither the
shared UI DSL nor presentation, transport, or application packages. The
production Gio adapter now compiles shared screen documents into this runtime,
keeping that dependency one-way.

## What is implemented

- An immutable element model with rows, columns, wrapping flow, responsive
  variants, rounded surfaces, selectable text, buttons, controlled editors,
  progress, constraints, overlays, and scrolling collections.
- Parent-scoped keyed identity and bounded Gio widget state.
- A custom key-and-pixel-anchored virtual viewport with bounded overscan and
  row measurements.
- A stock `layout.List` control using the same element/state/paint layer.
- Geometry rejection before paint recording and rounded borders made from
  nested fills rather than stroked curves.
- An offline lab with representative compact and wide main, Job Details, and
  Global Settings shapes, plus modal, lifecycle, 10,000-row, and reorder churn
  scenarios.
- Per-second JSON diagnostics, a Go-heap watchdog, tests, benchmarks, and an
  enforced import boundary.
- Separate macOS and iOS lab packaging; the iOS lab can coexist with ciwi.
- A production UIDSL-to-DOM compiler covering every native screen component,
  keyed widget identity, responsive reflow, overlays, graph interaction, and
  bounded virtual scrolling.

The lab remains offline and separately packaged. Production integration lives
in `internal/adapters/gio`; `internal/giodom` does not know about ciwi screens or
server connections.

## Gates

Run both the custom **Job · keyed** scenario and **Job · stock control**. Test
compact and wide window sizes, then repeat with **Start churn** enabled.

### Functional gate

- Main, Job Details, and Global Settings have no phantom vertical gaps when
  conditional sections are absent.
- Reordered rows preserve their semantic scroll anchor.
- Loading, ready, and error replacements do not retain state for removed
  elements or transfer state to a different key.
- Buttons, focused editing, selection, scrolling, responsive reflow, modal
  presentation, and progress animation remain usable during churn.
- A 10,000-row source materializes only the visible window and overscan.

### Resource gate

- Exercise each list implementation for one to two minutes on macOS and a
  physical iPhone with churn enabled. That window is sufficient for the known
  runaway-growth failure mode; a release candidate still warrants a longer
  soak.
- Observe total process footprint with Instruments or Xcode, not only the Go
  heap. After warm-up, memory must settle into a repeatable band; it must not
  show sustained monotonic growth through the final 30 seconds.
- The runtime must stay within 4,096 live state slots and 2,048 cached row
  measurements in the 10,000-row scenario.
- The process must remain comfortably below the device's memory pressure and
  Jetsam limits. Any Jetsam event is a failure even if Go heap is small.
- No invalid geometry, duplicate keys, or runtime errors may appear in the
  diagnostic stream.

### Performance gate

- On the target iPhone, steady-state layout should fit within an 8 ms p95
  budget; the complete rendered frame must fit the 16.7 ms 60 Hz budget.
- Churn may create transient work, but must not cause a repeated missed-frame
  pattern or interaction stalls.
- Scrolling must remain responsive with 10,000 rows and with nested Job Details
  output.

Use Instruments' frame and allocation tracks for the device result. The lab's
`frame_duration` and heap watchdog are useful corroboration, but they do not
include every Gio, driver, or GPU cost.

## Interpreting the control

| Keyed viewport | Stock list | Interpretation |
|---|---|---|
| Pass | Pass | The standalone reactive model is viable; choose on behavior and complexity. |
| Pass | Fail | Keep the custom viewport and investigate the stock-list interaction independently. |
| Fail | Pass | The custom viewport is at fault; iterate it or prefer Gio's list with keyed reconciliation around it. |
| Fail | Fail | Profile the shared element/state/paint layer and Gio backend before considering ciwi integration. |

## Decision rule

- **Adopt for ciwi** only when both platforms pass the functional and resource
  gates and at least one viewport passes the performance gate.
- **Iterate the standalone layer** when failure is localized, bounded, and
  supported by a profile that identifies a correctable abstraction defect.
- **Reject this approach** when safe, bounded input still causes unbounded
  native memory; ordinary parity features require unbounded retained state or
  geometry; or the frame budget cannot be met without discarding required
  interaction behavior.
- **Reject Gio for this product** only when the failing constraint is shown to
  be inherent to Gio/backend behavior rather than the custom runtime. The stock
  control, allocation profiles, and minimal reproducer should support that
  conclusion.

The physical-iOS soak remains a first-class gate because the original failure
was an iOS total-footprint OOM. Passing the standalone gate authorized the
integration; release readiness still requires production-device verification.

## Current evidence

The automated suite establishes keyed-state retention, state removal, dynamic
key validation, hard state and measurement bounds, absence of phantom flex
gaps, geometry rejection, and visible-only materialization of a 10,000-row
source. Benchmarks report allocations and layout cost for keyed reorders and
worst-case 10,000-row key churn.

The measurements below combine automated invariants, Go diagnostics,
Instruments total-footprint traces, and physical-device visual inspection.

Evidence recorded on 2026-08-09:

- The complete Go suite and the standalone architecture checks pass.
- On an Apple M1, the six-element keyed reorder benchmark measured about
  7.8 µs/op. Worst-case 10,000-row key rotation measured about 0.71 ms/op,
  448 KiB/op, and 92 allocations/op. The allocation figure includes validating
  and reindexing all 10,000 changed keys. An unchanged 10,000-row frame measured
  about 7.7 µs/op, 8.5 KiB/op, and 46 allocations/op because it reuses the
  revision-aware prefix index.
- Two-minute macOS churn smokes of both Job Details implementations completed
  without runtime or geometry errors. Each warmed to roughly 91.5 MiB
  `HeapSys`; live Go heap continued cycling rather than growing monotonically.
- A subsequent one-minute macOS smoke of the optimized 10,000-row churn path
  likewise settled at roughly 91.5 MiB `HeapSys`, retained 29–30 state slots,
  materialized 18–19 rows, and reported no runtime or geometry errors. Sampled
  layout times remained below the 8 ms target in that run.
- A two-minute macOS Activity Monitor trace of the 10,000-row churn path rose
  during startup and reached about 170.1 MiB after 40–50 seconds. It then stayed
  between roughly 170.1 and 170.3 MiB through the end of the trace.
- The universal macOS app built successfully. The arm64 iOS framework and
  unsigned device-host check also built successfully with bundle identifier
  `nl.izmar.giodomlab` and display name **Gio DOM Lab**.
- On an iPhone SE running iOS 26.6, a two-minute Activity Monitor trace of the
  keyed 10,000-row churn path rose from 103.17 MiB during warm-up to 115.06 MiB.
  It was flat at 115.06 MiB from 90 seconds through 120 seconds, with a
  115.09 MiB maximum.
- A one-minute physical-iPhone trace of the stock Job Details control rose from
  102.61 MiB to 109.69 MiB, peaked at 110.67 MiB, and ended below that peak.
- The physical-iPhone lab UI was manually confirmed to be the standalone lab
  and visually behaved correctly. The packager now gives the lab framework a
  distinct `GioDOMLab` name, uses isolated Xcode DerivedData, and rejects an
  archive that does not contain the lab payload marker.

These results ruled out an inherent short-run Gio memory mismatch for the
tested reactive model and supported production integration.

Integration evidence recorded on 2026-08-10:

- Every native screen now compiles through the keyed DOM. The former direct
  renderer, global path-indexed widget maps, geometry-op cache, surface cache,
  icon cache, and loader texture cache were removed.
- The compact layout bug was traced to desktop-axis `grow` being retained after
  a horizontal row became a vertical phone layout. Clearing that axis-specific
  growth during reflow removes the large gaps without special-casing screens.
- The complete Go suite, universal macOS app build, arm64 iOS framework build,
  and unsigned production iOS host build pass.
- An untouched production macOS Job Details run against the live CNP server
  used roughly 133 MiB after startup, 136 MiB at one minute, and 140 MiB at
  2:13. It showed no accelerating growth or recurrence of the prior 2 GiB OOM.

Follow-up regression evidence recorded on 2026-08-11:

- Compact and condensed reflow use the same width-only thresholds as the
  browser, sourced from the shared control contract rather than platform or
  orientation checks.
- Page viewports preserve pixel-continuous upward and downward anchors through
  inter-card gaps, accept touch drags over controls, and hand outward
  nested-scroll gestures to the page at both boundaries.

The integration result is still not a release claim. Direct interaction checks
and a short physical-iPhone production run remain before release readiness.
