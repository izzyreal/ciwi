# giodom

`giodom` is a standalone experiment in placing a small, keyed, reactive tree
above Gio's immediate-mode primitives. It is intentionally not the ciwi native
renderer and does not import ciwi screens, presentation state, transports, or
the shared UI DSL.

The package answers one narrow question: can an immutable application tree be
reconciled into Gio widget state with predictable identity, scrolling,
geometry, and memory use on macOS and iOS?

## Runtime contract

- Application code creates a fresh immutable `Element` tree for each frame.
- Runtime-owned widget state is addressed by parent-scoped element key, state
  slot, and element kind.
- Dynamic siblings require non-empty, unique keys. Their `Revision` changes
  whenever membership or order changes.
- Index identity is permitted only for structurally static children.
- Removed and offscreen state is swept after the frame. Controlled application
  values, not retained widgets, remain authoritative.
- The custom viewport anchors scrolling to a child key and pixel offset, so a
  reorder does not silently attach the position to another row.
- Only visible rows plus bounded overscan are materialized. Cached row
  measurements and runtime state slots have explicit hard limits.
- Geometry outside the configured range is rejected before recording paint
  operations.
- Rounded borders use nested filled shapes. The evaluated runtime does not use
  stroked curves.

`StockList` is deliberately included as a control. It uses Gio's
`layout.List` while retaining every other runtime rule, which makes list
behavior independently comparable to the custom keyed viewport.

The architectural test in `internal/architecture` prevents this package and
its lab from acquiring dependencies on the rest of ciwi.

## Lab

Run the local application without a server:

```bash
go run ./cmd/giodom-lab
```

It provides synthetic main, Job Details, Global Settings, 10,000-row, modal,
loading, ready, and error states. Job Details can be switched between the
custom keyed viewport and the stock-list control. **Start churn** repeatedly
reorders rows and changes lifecycle state. One JSON diagnostic snapshot is
written to stderr each second.

Desktop automation can select an initial scenario with
`GIODOM_LAB_SCENARIO=job-keyed` (or `job-stock`, `stress`, `settings`, `main`),
enable churn with `GIODOM_LAB_STRESS=true`, and request a bounded run with
`GIODOM_LAB_RUN_FOR=10m`.

The lab has a 400 MiB Go-heap watchdog. This is a diagnostic backstop, not a
substitute for observing total process footprint in Instruments: native and
GPU allocations are not all represented by Go heap metrics.

Build separate, co-installable apps with:

```bash
./packaging/build-giodom-lab-macos-app.sh 0.1.0 /tmp/giodom-macos
./packaging/build-giodom-lab-ios-framework.sh 0.1.0 /tmp/giodom-ios/Ciwi.framework
./packaging/build-giodom-lab-ios-host.sh check 0.1.0 \
  /tmp/giodom-ios/Ciwi.framework /tmp/giodom-ios/GioDOMLab.app
```

The iOS host keeps the framework's required internal `Ciwi` name, but uses the
separate `nl.izmar.giodomlab` bundle identifier and **Gio DOM Lab** display
name. It can therefore coexist with the ciwi client.

## Verification

```bash
go test ./internal/giodom/... ./internal/architecture
go test ./internal/giodom -run '^$' -bench . -benchmem
```

These checks characterize the standalone abstraction. They are not ciwi
renderer regression tests and do not make an adoption decision on their own.
The device gate and decision rules live in `docs/gio-dom-viability.md`.
