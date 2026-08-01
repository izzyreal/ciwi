# Architecture review — August 2026

This review follows the addition of managed YAML projects, pipeline and execution
graphs, richer job-history cards, server update selection, and browser themes.
It focuses on keeping ciwi easy to evolve without trading away its useful
single-binary deployment model.

## Executive assessment

ciwi's foundations are healthy. The package graph is acyclic and understandable,
the server/agent protocol is explicit, SQLite access is isolated in `store`, and
the server already uses consumer-owned store interfaces as extraction seams.
The recent features have not created a need for a repository-wide layering
exercise.

There are, however, two clear pressure points:

1. The browser UI is embedded as Go string constants in the root server package.
   It is now the largest part of that package and is difficult to validate with
   normal frontend tooling.
2. Pipeline-run orchestration and its metadata conventions are spread across the
   root server package. The behavior is well tested, but the package boundary no
   longer communicates ownership clearly.

The recommended direction is incremental extraction at those seams, not a broad
rename or a generic `services`/`models` layer.

## Snapshot

Measurements at the time of review:

- `internal` production Go: about 35,380 lines.
- root `internal/server` production Go: about 19,562 lines across 88 files.
- root server UI sources: about 11,276 lines (roughly 58% of that package).
- root server pipeline sources: about 3,253 lines.
- `internal/agent`: about 6,427 lines.
- `internal/store`: about 2,864 lines.
- execution behavior currently relies on roughly 36 distinct string metadata
  keys, while only rerun/attempt keys are centralized as protocol constants.
- full statement coverage after this review: 77.4% (75.7% before the focused
  additions).

Line count is not itself a defect. Here it identifies places where unrelated
changes share a compilation and ownership boundary.

## What is working well

### Deployment shape

The server UI and runtime ship as one binary, with no separate web deployment.
That is a strong fit for ciwi and should remain a constraint during UI extraction.

### Package direction

`config`, `protocol`, `requirements`, `store`, and the server/agent entry packages
have clear roles. Existing focused server packages (`jobexecution`, `jobhistory`,
`jobprogress`, `project`, `update`, and `vault`) show that incremental extraction
works in this codebase.

### Dependency seams

`server_module_deps.go` defines interfaces at the point of consumption. This is
idiomatic Go and provides a practical path for moving behavior without making the
SQLite store implement artificial repository abstractions.

### Behavioral tests

The suite covers HTTP behavior, SQLite migration and lifecycle behavior, agent
execution, update flows, pipeline chains, matrix jobs, and UI invariants. Design
guard tests already prevent some regressions such as raw status literals and
untyped JSON responses.

## Main design risks

### 1. UI source ownership

The UI is divided into many sensibly named Go files, but all of them still compile
as package `server`. JavaScript and CSS are inside raw strings, so Go can verify
that strings exist but cannot parse, lint, import, or unit-test them as browser
source. Page behavior is increasingly stateful: graphs, local-storage preferences,
tooltips, themes, live job updates, and update selection now interact.

This is the strongest case for adding a directory level.

### 2. Pipeline application logic in the HTTP composition package

The root server package owns request routing, pipeline dependency resolution,
version lookup, dry-run preview, pending-job construction, chain scheduling,
reconciliation, Vault resolution, and persistence coordination. File names keep
this navigable, but `stateStore` is required by behavior that does not inherently
belong to HTTP or global server state.

The risk is not current correctness. The risk is that preview, enqueue, rerun,
inspection, and graph behavior acquire subtly different interpretations of the
same pipeline.

### 3. Stringly typed execution metadata

Execution metadata is carrying an implicit domain model: project/pipeline/run
identity, chain position and dependencies, source refs, versions, matrix identity,
dry-run state, blocking state, runtime options, and rerun identity. Direct map
access is distributed across server and store code.

Misspellings and partial writes are compile-time invisible. It also makes schema
and API evolution harder because required and optional fields are undocumented in
the type system.

### 4. Database migration growth

The current idempotent migration code is tested and appropriate for the early
schema. As more persisted UI/project/update features arrive, conditional column
addition and special-case migrations will become harder to order and audit.

### 5. Time and process boundaries in tests

The suite contains real timing and external-process behavior. That is valuable for
integration confidence, but the full server test package takes tens of seconds and
many tests rely on elapsed time. Continuing to add real waits will make feedback
slower and increase flake risk.

## Recommended evolution

### Phase 1: extract the embedded web UI

Create `internal/server/webui` and make it own a handler plus embedded assets:

```text
internal/server/webui/
  handler.go
  assets/
    css/
    js/
      shared/
      pages/
    pages/
    images/
```

Keep `go:embed`, plain JavaScript, and the single binary. Do not require a Node
build just to perform the move. The root router should mount a `webui.Handler`;
the package should depend on browser assets and `net/http`, not on `stateStore`.
Dynamic data should continue to arrive through the existing JSON APIs.

Once the files are real `.js` and `.css` sources, shared graph/status/theme/storage
logic can be imported instead of copied. A small browser smoke suite can then test
the highest-value interactions: graph navigation, job-log live updates, managed
YAML editing, and theme persistence.

### Phase 2: centralize execution metadata

First introduce constants and typed read/write helpers for every recognized
metadata field in one place shared by server and store code. Then define grouped
views such as `RunIdentity`, `SourceContext`, `ChainContext`, and `MatrixContext`
that encode to the backward-compatible map.

This can be done without an immediate database migration. Once all callers use the
typed facade, decide whether stable fields deserve columns or a versioned structured
JSON document. Preserve unknown map entries for compatibility during transition.

### Phase 3: extract pipeline-run planning

Create a focused application package, preferably `internal/server/pipelinerun`,
rather than generic `service` or `model` packages. Give it explicit inputs and a
consumer-owned repository interface. Migrate in this order:

1. version and source-ref resolution;
2. pending-job and matrix planning;
3. dry-run/offline preview;
4. dependency and chain reconciliation.

HTTP handlers should decode requests and translate results. Persistence adapters
should translate store records into planner inputs. The planner should not know
about `http.ResponseWriter` or the full `stateStore`.

Keep inspection and execution on the same planner path so rendered scripts and
secret mappings cannot drift from what will actually run.

### Phase 4: version database migrations

Add an ordered migration ledger (`PRAGMA user_version` or a migrations table),
with each migration applied transactionally and tested from representative older
schemas. Do this before the next substantial schema feature, not as a prerequisite
for the UI extraction.

### Phase 5: decompose in-memory server state when touched

`stateStore` is a composition root containing the database, agent registry,
update rollout, icon cache, Vault token cache, and progress estimator. This is
acceptable at the outer boundary, but the name understates its role. As agent or
update behavior changes, move their maps and locks behind `AgentRegistry` and
`UpdateController`. Rename the outer object to `serverState` or `app` only after
those components exist; a rename alone has little value.

## Testing policy

Do not optimize for one global coverage number. Suggested targets are:

- 85%+ for pure planning, metadata, config, and store lifecycle code;
- endpoint contract tests for handlers;
- focused integration tests for Git, Vault, updater, and agent process behavior;
- a small browser suite for interaction-heavy UI flows;
- race testing for server/store/agent state before releases.

Inject clocks and command runners into newly touched scheduling/update logic. This
lets unit tests advance time without sleeping while retaining a smaller number of
real integration tests.

## Explicit non-recommendations

- Do not add a generic `internal/models`, `internal/services`, or `internal/utils`
  hierarchy. It would hide dependencies without clarifying ownership.
- Do not split `store` merely because it has several files; its cohesive SQLite
  boundary and consumer-owned interfaces are currently a strength.
- Do not introduce a frontend framework as part of the UI extraction. Real asset
  files and module boundaries provide most of the immediate benefit.
- Do not rewrite the pipeline engine in one change. Preserve the existing tests
  and move one behavior seam at a time.

## Next architectural slice

The best next slice is Phase 1: move the current UI verbatim into embedded asset
files behind `internal/server/webui`, prove byte-equivalent routing where practical,
then start deduplicating shared browser modules. It is independently valuable,
low-risk, and removes more than half of the root server package without changing
ciwi's deployment model.
