# Backend API

## Shared across consumers

- Agent + Frontend:
  - `GET /api/v1/jobs/{id}`
  - `GET /api/v1/jobs/{id}/artifacts`
- Frontend + installer:
  - `GET /healthz`

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
  - `GET /api/v1/projects/{projectId}/icon`
  - `POST /api/v1/projects/{projectId}/reload`
  - `POST /api/v1/pipelines/{pipelineDbId}/run-selection`
  - `POST /api/v1/pipeline-chains/{chainDbId}/run`
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
