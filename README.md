# ciwi
Simple CI/CD and build automation system

## Getting started

```bash
go run ./cmd/ciwi --help
go run ./cmd/ciwi server
go run ./cmd/ciwi agent
go run ./cmd/ciwi all-in-one
```

## Environment variables

- `CIWI_SERVER_ADDR`: server bind address (default `:8080`)
- `CIWI_DB_PATH`: sqlite database path (default `ciwi.db`)
- `CIWI_SERVER_URL`: agent target base URL (default `http://127.0.0.1:8080`)
- `CIWI_AGENT_ID`: override agent ID (default `agent-<hostname>`)
- `CIWI_AGENT_WORKDIR`: local working directory for job execution (default `.ciwi-agent`)

## Server prerequisites

- `git` must be installed on the server host to import projects from git repositories.
- Project import fetches only git metadata + the root config file (no full repo checkout).

## Agent prerequisites

- `git` must be installed on the agent host for pipeline jobs that define `source.repo`.

## First functional API slice

- `GET /` minimal web UI (projects/pipelines/jobs)
- `GET /healthz` returns `{"status":"ok"}`
- `POST /api/v1/heartbeat` accepts agent heartbeats in JSON
- `GET /api/v1/agents` returns known agents
- `POST /api/v1/projects/import` imports a project from git (`ciwi-project.yaml` by default)
- `POST /api/v1/jobs` enqueues a job
- `GET /api/v1/jobs` returns all jobs
- `GET /api/v1/jobs/{id}` returns one job
- `POST /api/v1/jobs/{id}/status` updates job status (`running`, `succeeded`, `failed`)
- `POST /api/v1/agent/lease` leases a matching queued job to an agent
- `GET /api/v1/projects` returns persisted projects with pipelines
- `POST /api/v1/pipelines/run` loads `ciwi.yaml` and enqueues pipeline jobs
- `POST /api/v1/pipelines/{pipelineDbId}/run` runs a persisted pipeline from sqlite

Pipeline configs (for example root `ciwi-project.yaml`) require:
- `pipelines[].source.repo`: git URL to clone before running job steps
- `pipelines[].source.ref` (optional): branch/tag/ref to checkout

## Quick API test

```bash
# 1) Start server and agent in separate terminals.
go run ./cmd/ciwi server
go run ./cmd/ciwi agent

# 2) Open browser UI.
open http://127.0.0.1:8080/

# 3) Import a project from git (loads ciwi-project.yaml by default).
curl -s -X POST http://127.0.0.1:8080/api/v1/projects/import \
  -H 'Content-Type: application/json' \
  -d '{"repo_url":"https://github.com/izzyreal/ciwi.git","repo_ref":"main"}'

# 4) Find pipeline DB IDs.
curl -s http://127.0.0.1:8080/api/v1/projects

# 5) Run a persisted pipeline by DB ID.
curl -s -X POST http://127.0.0.1:8080/api/v1/pipelines/1/run -d '{}'

# 6) Check jobs:
curl -s http://127.0.0.1:8080/api/v1/jobs
```
