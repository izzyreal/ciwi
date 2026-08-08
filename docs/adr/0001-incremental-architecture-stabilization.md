# ADR 0001: Incremental architecture stabilization

Status: accepted

## Context

The declarative shared UI, native protocol, live refresh, pipeline chains, and
update orchestration were introduced in a short period. The inner
domain/application/presentation direction is sound, but several outer adapters
still own overlapping semantic mappings and `internal/server` remains both a
host and a home for feature state.

Ciwi must remain releasable while it is prepared for IAM, authenticated source
repositories, and runner kinds beyond shell scripts.

## Decision

Architecture changes are delivered as vertical, behavior-preserving slices on
main rather than through a flag-day rewrite.

1. Persisted data receives explicit transactional migrations before new schema
   work.
2. Presentation owns renderer-neutral semantics; HTTP, CNP, browser, and Gio
   adapters map or render them without recomputation.
3. Application commands own mutation policy, idempotency, transaction outcome,
   and change invalidation.
4. Runtime feature state moves out of the server host when its next cohesive
   slice is changed.
5. Repository and runner ports are introduced only around behavior that exists
   today. IAM provider APIs are deferred until concrete authentication work.

The first slice therefore keeps Git checkout and shell execution as the sole
implementations behind agent-owned ports. The process-local agent registry owns
agent snapshots, pending controls, deactivation, and rollout scheduling, while
the surrounding adapter coordinates SQLite, artifacts, jobs, and invalidations.

## Consequences

- Pre-1.0 internal APIs may break, while database upgrades remain compatible.
- Boundary tests and cross-adapter fixtures are part of the architecture, not
  optional cleanup.
- Large files are split only along cohesive behavior boundaries; package count
  and line count are not goals by themselves.
- No generic repository layer, generic event bus, UI template language,
  microservice split, or speculative identity-provider framework is introduced.
