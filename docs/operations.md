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

## Tool refresh

- `/agents` -> **Refresh Tools** triggers an on-demand tool rescan on agent.

## Troubleshooting quick checks

- Server health: `GET /healthz`
- Server identity: `GET /api/v1/server-info`
- Agent service status via system service manager
- Job detail page for runtime capabilities, cache stats, unmet requirements
