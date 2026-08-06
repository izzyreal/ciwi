# Backend API

ciwi has two first-class client protocol families. The browser uses the HTTP API
described below. Native clients use CNP v1 over QUIC or multiplexed TLS/TCP,
described in [`native-client.md`](native-client.md). All adapters call the same
application and presentation services; CNP is not an HTTP wrapper, and its
transport adapters do not contain application behavior.

## Shared across consumers

- Agent + Frontend:
  - `GET /api/v1/jobs/{id}`
  - `GET /api/v1/jobs/{id}/artifacts`
  - `GET /api/v1/jobs/{id}/artifacts/download-all`
- Frontend + installer:
  - `GET /healthz`
  - `GET /api/v1/server-info`
- Frontend runtime state:
  - `GET /api/v1/runtime-state`

## Consumed by agent runtime

- `POST /api/v1/heartbeat`
- `POST /api/v1/agent/lease`
- `POST /api/v1/jobs/{id}/status`
- `POST /api/v1/jobs/{id}/artifacts/upload-zip`
- `POST /api/v1/jobs/{id}/tests`

## Consumed by frontend UI

- Presentation views:
  - `GET /api/v1/views/front-page`
  - `GET /api/v1/views/projects/{projectId}`
  - `GET /api/v1/views/jobs/{jobExecutionId}`
  - `GET /api/v1/views/jobs/{jobExecutionId}/output?after_event_id={cursor}`
  - `GET /api/v1/views/run-options/pipelines/{pipelineDbId}`
  - `GET /api/v1/views/run-options/projects/{projectId}/chains/{chainId}`
  - `GET /api/v1/views/agents`
  - `GET /api/v1/views/agents/{agentId}`

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
  - `GET /api/v1/projects/{projectId}/inspect`
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
  - `GET /api/v1/job-queue/layout`
  - `GET /api/v1/job-queue/cards`
  - `GET /api/v1/job-history/layout`
  - `GET /api/v1/job-history/cards`
  - `POST /api/v1/jobs/clear-queue`
  - `POST /api/v1/jobs/flush-history`
  - `POST /api/v1/jobs/{id}/cancel`
  - `POST /api/v1/jobs/{id}/rerun`
  - `GET /api/v1/jobs/{id}/events?after_id={cursor}`
  - `GET /api/v1/jobs/{id}/log?format=clean|raw`
  - `GET /api/v1/jobs/{id}/blocked-by`
  - `GET /api/v1/jobs/{id}/artifacts/download?prefix={path}`
  - `GET /api/v1/jobs/{id}/artifacts/download-all`
  - `GET /api/v1/jobs/{id}/tests`
- Command recovery:
  - `GET /api/v1/command-receipts/{idempotencyKey}`
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
  - `{"action":"refresh-tools"}`: asks the agent to rescan host/runtime capabilities.
  - `{"action":"restart"}`: asks a service-managed agent to restart after active work finishes.
  - `{"action":"update"}`: requests an update to the server's current release target.
  - `{"action":"wipe-cache"}`: removes the agent cache after active work finishes.
  - `{"action":"flush-job-history"}`: removes this agent's terminal server history/artifacts and queues local workspace-history cleanup.
  - `{"action":"run-script","shell":"posix","script":"...","timeout_seconds":600}`: queues an ad-hoc job pinned to that agent; `cmd` and `powershell` are also accepted when advertised by the agent.
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
- Mutating application-backed endpoints accept `Idempotency-Key` where the
  corresponding command supports retries. The command-receipt endpoint reports
  `pending`, `completed`, `failed`, or `outcome_unknown` for a found receipt;
  an unknown key returns `found=false`.
