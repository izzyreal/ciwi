# Backend API

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

- Agents:
  - `GET /api/v1/agents`
  - `GET /api/v1/agents/{agentId}`
  - `POST /api/v1/agents/{agentId}/actions`
- Projects/pipelines:
  - `GET /api/v1/projects`
  - `POST /api/v1/projects/import`
  - `GET /api/v1/projects/{projectId}`
  - `DELETE /api/v1/projects/{projectId}`
  - `GET /api/v1/projects/{projectId}/icon`
  - `POST /api/v1/projects/{projectId}/reload`
  - `POST /api/v1/pipelines/{pipelineDbId}/run-selection`
  - `POST /api/v1/pipelines/{pipelineDbId}/dry-run-preview`
  - `GET /api/v1/pipelines/{pipelineDbId}/source-refs`
  - `POST /api/v1/pipelines/{pipelineDbId}/eligible-agents`
  - `POST /api/v1/pipeline-chains/{chainDbId}/run`
  - `POST /api/v1/pipeline-chains/{chainDbId}/dry-run-preview`
  - `GET /api/v1/pipeline-chains/{chainDbId}/source-refs`
  - `POST /api/v1/pipeline-chains/{chainDbId}/eligible-agents`
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
- Machine behavior should rely on structured API payloads, not output log scraping.
- `POST /api/v1/agents/{agentId}/actions` supports:
  - `{"action":"activate"}`: marks agent active; leasing is allowed.
  - `{"action":"deactivate"}`: marks agent deactivated; leasing is blocked.
- Deactivation is server-side only (agent protocol is unchanged).
- While deactivated, `POST /api/v1/agent/lease` returns `assigned=false` with message `agent is deactivated`.
- If deactivation occurs while the agent has an active leased/running job, server applies the same terminal mutation as `POST /api/v1/jobs/{id}/cancel`:
  - `status=failed`
  - `error="cancelled by user"`
  - append `[control] job cancelled by user` to output
- `POST /api/v1/pipelines/{id}/run-selection` and `POST /api/v1/pipeline-chains/{id}/run` accept optional `execution_mode`:
  - `offline_cached` executes from cached pinned source context with safety guardrails.
- Run payload fields (pipeline/chain run and preview family) may include:
  - `source_ref`: one-off source branch/tag/SHA override
  - `agent_id`: pin execution to a specific eligible agent
  - `dry_run`: preview/non-writing mode
  - `offline_cached_only`: preview-time filter for cached-source feasibility
