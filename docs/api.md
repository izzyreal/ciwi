# Backend API

ciwi has two first-class client transports. The browser uses the HTTP API
described below. Native clients use CNP v1 (Protocol Buffers over QUIC),
described in [`native-client.md`](native-client.md). Both adapters call the same
application and presentation services; CNP is not an HTTP wrapper.

## Shared across consumers

- Agent + Frontend:
  - `GET /api/v1/jobs/{id}`
  - `GET /api/v1/jobs/{id}/artifacts`
- Frontend + installer:
  - `GET /healthz`
  - `GET /api/v1/runtime-state`

## Consumed by agent runtime

- `POST /api/v1/heartbeat`
- `POST /api/v1/agent/lease`
- `POST /api/v1/jobs/{id}/status`
- `POST /api/v1/jobs/{id}/artifacts`
- `POST /api/v1/jobs/{id}/tests`

## Consumed by frontend UI

- Presentation views:
  - `GET /api/v1/views/front-page`
  - `GET /api/v1/views/projects/{projectId}`
  - `GET /api/v1/views/jobs/{jobExecutionId}`
  - `GET /api/v1/views/jobs/{jobExecutionId}/output?after_event_id={cursor}`

- Agents:
  - `GET /api/v1/agents`
  - `GET /api/v1/agents/{agentId}`
  - `POST /api/v1/agents/{agentId}/actions`
- Projects/pipelines:
  - `GET /api/v1/projects`
  - `POST /api/v1/projects/import`
  - `POST /api/v1/projects/managed-yaml/validate`
  - `POST /api/v1/projects/managed-yaml`
  - `GET /api/v1/projects/{projectId}`
  - `DELETE /api/v1/projects/{projectId}`
  - `GET /api/v1/projects/{projectId}/managed-yaml`
  - `PUT /api/v1/projects/{projectId}/managed-yaml`
  - `GET /api/v1/projects/{projectId}/icon`
  - `POST /api/v1/projects/{projectId}/reload`
  - `POST /api/v1/pipelines/{pipelineDbId}/run-selection`
  - `POST /api/v1/pipelines/{pipelineDbId}/dry-run-preview`
  - `GET /api/v1/pipelines/{pipelineDbId}/source-refs`
  - `POST /api/v1/pipelines/{pipelineDbId}/eligible-agents`
  - `POST /api/v1/projects/{projectId}/pipeline-chains/{chainId}/run`
  - `POST /api/v1/projects/{projectId}/pipeline-chains/{chainId}/dry-run-preview`
  - `GET /api/v1/projects/{projectId}/pipeline-chains/{chainId}/source-refs`
  - `POST /api/v1/projects/{projectId}/pipeline-chains/{chainId}/eligible-agents`
  - `GET /api/v1/pipelines/{pipelineDbId}/version-resolve` (SSE)
- Jobs:
  - `GET /api/v1/jobs`
  - `DELETE /api/v1/jobs/{id}`
  - `POST /api/v1/jobs/clear-queue`
  - `POST /api/v1/jobs/flush-history`
  - `POST /api/v1/jobs/{id}/cancel`
  - `POST /api/v1/jobs/{id}/rerun`
  - `GET /api/v1/jobs/{id}/tests`
- Vault:
  - `GET /api/v1/vault/connections`
  - `POST /api/v1/vault/connections`
  - `DELETE /api/v1/vault/connections/{id}`
  - `POST /api/v1/vault/connections/{id}/test`
- Updates/server control:
  - `POST /api/v1/update/check`
  - `POST /api/v1/update/apply`
  - `POST /api/v1/update/rollback`
  - `GET /api/v1/update/tags`
  - `GET /api/v1/update/status`
  - `POST /api/v1/server/restart`

## Consumed by installers/provisioning

- `GET /healthz`
- `GET /api/v1/server-info`

## API behavior notes

- Config parsing uses strict YAML field validation.
- Managed YAML definitions are stored verbatim in SQLite together with their parsed execution snapshot.
- Managed YAML updates require the current SHA-256 `revision`; stale updates return `409 Conflict` without changing the project.
- Managed YAML project names are case-insensitively unique among managed projects, and request bodies are limited to 2 MiB.
- Machine behavior should rely on structured API payloads, not output log scraping.
- `POST /api/v1/pipelines/{id}/run-selection` accepts an optional
  `Idempotency-Key`. Repeating the same command and payload returns the stored
  result without enqueuing duplicate executions; reusing a key for a different
  payload returns `409 Conflict`.
- `POST /api/v1/agents/{agentId}/actions` supports:
  - `{"action":"authorize"}`: allows the agent to lease jobs.
  - `{"action":"unauthorize"}`: prevents the agent from leasing new jobs.
  - `{"action":"activate"}`: marks agent active; leasing is allowed when also authorized.
  - `{"action":"deactivate"}`: marks agent deactivated; leasing is blocked.
  - `{"action":"delete"}`: deletes server-side agent snapshot/state; agent disappears from list until next heartbeat.
- Deactivation is server-side only (agent protocol is unchanged).
- New/unknown agents are unauthorized until explicitly authorized.
- `POST /api/v1/jobs/flush-history` removes non-active job execution records and deletes artifact directories for the flushed job IDs.
- `POST /api/v1/agent/lease` requires a known + authorized + non-deactivated agent snapshot.
- While deactivated, `POST /api/v1/agent/lease` returns `assigned=false` with message `agent is deactivated`.
- If deactivation occurs while the agent has an active leased/running job, server applies the same terminal mutation as `POST /api/v1/jobs/{id}/cancel`:
  - `status=failed`
  - `error="cancelled by user"`
  - append `[control] job cancelled by user` to output
- `POST /api/v1/pipelines/{id}/run-selection` and `POST /api/v1/projects/{projectId}/pipeline-chains/{chainId}/run` accept optional `execution_mode`:
  - `offline_cached` executes from cached pinned source context with safety guardrails.
- Run payload fields (pipeline/chain run and preview family) may include:
  - `source_ref`: one-off source branch/tag/SHA override
  - `agent_id`: pin execution to a specific eligible agent
  - `dry_run`: preview/non-writing mode
  - `offline_cached_only`: preview-time filter for cached-source feasibility
