# Operations

## Update behavior

### Server self-update

- Check/apply/rollback via Global Settings and `/api/v1/update/*`.
- Controls are enabled only when server runs under supported service manager.
- In dev mode or standalone mode, controls are visible but disabled with guidance.

### Agent self-update policy

- Policy: **drain queue**.
- Agent finishes running + already-queued eligible jobs, then applies pending update.
- No immediate preemption mode is exposed.

### Non-service agents

- Agents not running as a service report self-update disabled failures.
- UI surfaces feedback and links to installer docs.

## Job history and cleanup

- **Flush History** removes non-active execution records from sqlite.
- Artifact files are not automatically GC’d by history flush.

## Offline-cached execution

- `execution_mode=offline_cached` can be used on pipeline/chain run APIs for cached-source execution.
- Guardrails: source must resolve to a pinned cached commit; non-dry offline runs are blocked for jobs containing `skip_dry_run` steps.
- Use preview endpoints first: `POST /api/v1/pipelines/{id}/dry-run-preview` and `POST /api/v1/pipeline-chains/{id}/dry-run-preview`.

UI flow:
- use `Preview Dry Run`
- optionally enable `offline_cached_only` in preview modal
- use `Execute Offline` to enqueue with `execution_mode=offline_cached`

## Tool refresh

- `/agents` -> **Refresh Tools** triggers an on-demand tool rescan on agent.

## Agent activation/deactivation

- `/agents` and `/agents/{id}` expose **Activate** / **Deactivate** controls.
- `/agents` also exposes **Delete** to remove a stored agent snapshot.
- New agents start unauthorized and must be explicitly authorized before leasing jobs.
- In Agents overview:
  - unauthorized agent rows show only **Authorize** in Actions
  - authorized agent rows show normal actions plus **Unauthorize**
- Deactivation is enforced on the server:
  - deactivated agents remain visible and continue heartbeat updates
  - deactivated agents do not lease jobs
- Deactivating an agent with an active leased/running job triggers the same server-side effect as job **Cancel**:
  - job becomes `failed`
  - error is `cancelled by user`
  - output gets `[control] job cancelled by user`
- Activation state is persisted by the server and survives server restart.
- Deleting a snapshot removes the agent from server state/UI immediately; if the real agent is still running, it reappears on the next heartbeat.

## Troubleshooting quick checks

- Server health: `GET /healthz`
- Runtime mode (normal/degraded_offline): `GET /api/v1/runtime-state`
- Server identity: `GET /api/v1/server-info`
- Agent service status via system service manager
- Job detail page for runtime capabilities, cache stats, unmet requirements

## Project import/reload behavior

- Identity key is `(repo_url, repo_ref, config_file)`.
- Reload updates the same identity in place.
- Importing same repo with a different branch creates/keeps a distinct project identity; it does not overwrite another branch project.
