# ADR 0002: Shared UI authority and adapter boundaries

Status: accepted

Date: 2026-08-10

## Context

The production browser and Gio clients now render the same screen documents.
The native client also uses a keyed Gio DOM runtime, typed CNP views, and the
shared route, action, theme, typography, and control catalogs.

An architectural review found that the overall dependency direction remained
sound, but several migration-era concepts implied more sharing than actually
existed:

- a hard-coded Go command set duplicated `ui/actions.yaml`;
- browser JavaScript and large per-theme CSS blocks duplicated the theme
  documents;
- page and hero gradient declarations were only partially honored by the two
  renderers;
- top-level screen persistence and override declarations had no renderer;
- data-source query names were validated but never selected or executed by
  either adapter;
- legacy browser selectors still affected the declarative UI cascade;
- renderer responsibilities had accumulated in a few very large files.

## Decision

1. `ui/screens/*.yaml` is the authority for screen hierarchy, visible copy,
   binding paths, actions, conditions, semantic layout, and narrow node-level
   platform exceptions.
2. `ui/actions.yaml` is the only command-semantic registry. Screen commands are
   syntax-checked independently and cross-checked against the catalog when the
   shared bundle loads them. Browser and native adapters retain only the
   necessary mapping from a command to HTTP or CNP behavior.
3. `ui/themes/*.yaml`, `ui/typography.yaml`, and `ui/controls.yaml` are the
   visual authorities. Browser theme CSS is generated from the theme
   documents; renderer styles may derive variables but may not enumerate
   themes. Gio continues to convert the same tokens to native drawing values.
   Both renderers consume the declared multi-stop page and hero gradients;
   renderer-owned glow composition uses the same shared glow colors.
4. A data source declares a binding root and optional invalidation topics. It
   does not pretend to be a cross-transport query language. Route-specific
   HTTP and CNP loading remains explicit adapter code.
5. Persistent interaction state is adapter-owned. The DSL supplies stable keys
   for disclosures and graph views, while each renderer owns storage,
   viewport, focus, popup, selection, search, and tailing lifecycle. Action
   recovery persistence remains a distinct catalogued operation policy.
6. Renderer files are split by cohesive runtime responsibility. File size is a
   navigation signal, not a reason to introduce generic services, component
   inheritance, or a UI fragment language.
7. Transitive import-boundary tests, shared bundle validation, cross-adapter
   binding fixtures, command-coverage tests, Gio DOM tests, and real-browser
   DOM tests are architecture checks rather than optional UI tests.

## Consequences

Adding a command or theme starts in one shared catalog. Adding a screen-visible
business concept starts in presentation and the shared screen contract;
adapters supply transport and interaction mechanics only. Invalid catalog
references fail during bundle loading instead of being accepted by one
renderer and rejected by another.

Large screen YAML files remain cohesive product-surface documents. A reusable
fragment/component facility should be added only after repeated structures have
stable parameters and semantics; extracting visually similar but independently
evolving sections now would add indirection and make parity harder to audit.

Some duplication is intentional. HTTP and CNP encode the same presentation
models differently, and browser/Gio output buffering and widget state follow
different lifecycle models. Tests compare their observable binding and action
contracts rather than forcing those adapter details behind a generic layer.

## Remaining risks

- HTTP and protobuf view mappings can drift as presentation models evolve;
  populated binding fixtures and route coverage remain required.
- Output streaming, search, selection, and tailing are interaction-heavy in
  both adapters. Their shared declarations and commands constrain visible
  behavior, but device/browser regression tests are still necessary.
- Native visual and gesture parity cannot be established by Go tests alone;
  physical-device checks remain part of release readiness.
- Backend evolution priorities in `docs/architecture.md` remain valid, notably
  the pipeline-planning seam and centralized execution metadata accessors.
