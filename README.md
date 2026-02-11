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

- `CIWI_SERVER_ADDR`: server bind address (default `:8112`)
- `CIWI_DB_PATH`: sqlite database path (default `ciwi.db`)
- `CIWI_ARTIFACTS_DIR`: server artifact storage directory (default `ciwi-artifacts`)
- `CIWI_SERVER_URL`: agent target base URL (default `http://127.0.0.1:8112`)
- `CIWI_AGENT_ID`: override agent ID (default `agent-<hostname>`)
- `CIWI_AGENT_WORKDIR`: local working directory for job execution (default `.ciwi-agent`)
- `CIWI_AGENT_TRACE_SHELL`: enable shell command tracing (`set -x` / `Set-PSDebug`) (default `true`)
- `CIWI_AGENT_GO_BUILD_VERBOSE`: sets `GOFLAGS=-v` when unset (default `true`)

## Server prerequisites

- `git` must be installed on the server host to import projects from git repositories.
- Project import fetches only git metadata + the root config file (no full repo checkout).

## Agent prerequisites

- `git` must be installed on the agent host for pipeline jobs that define `source.repo`.

## First functional API slice

- `GET /` minimal web UI (projects/pipelines/jobs)
- `GET /projects/{projectId}` project page with structure, per-matrix run buttons and execution history
- `GET /healthz` returns `{"status":"ok"}`
- `POST /api/v1/heartbeat` accepts agent heartbeats in JSON
- `GET /api/v1/agents` returns known agents
- `POST /api/v1/projects/import` imports a project from git (`ciwi-project.yaml` by default)
- `POST /api/v1/projects/{projectId}/reload` reloads project definition from saved VCS settings
- `POST /api/v1/jobs` enqueues a job
- `GET /api/v1/jobs` returns all jobs
- `GET /api/v1/jobs/{id}` returns one job
- `DELETE /api/v1/jobs/{id}` removes a queued job
- `POST /api/v1/jobs/clear-queue` removes all queued jobs
- `POST /api/v1/jobs/flush-history` removes all finished jobs from history
- `POST /api/v1/jobs/{id}/status` updates job status (`running`, `succeeded`, `failed`)
- `GET /api/v1/jobs/{id}/artifacts` lists uploaded artifacts for a job
- `POST /api/v1/jobs/{id}/artifacts` uploads artifacts for a job (agent use)
- `GET /api/v1/jobs/{id}/tests` returns parsed test report for a job
- `POST /api/v1/jobs/{id}/tests` uploads parsed test report for a job (agent use)
- `POST /api/v1/agent/lease` leases a matching queued job to an agent
- `GET /api/v1/projects` returns persisted projects with pipelines
- `GET /api/v1/projects/{projectId}` returns full project structure (pipelines/jobs/matrix)
- `POST /api/v1/pipelines/run` loads `ciwi.yaml` and enqueues pipeline jobs
- `POST /api/v1/pipelines/{pipelineDbId}/run` runs a persisted pipeline from sqlite
- `POST /api/v1/pipelines/{pipelineDbId}/run-selection` runs a selected job/matrix include

Pipeline configs (for example root `ciwi-project.yaml`) require:
- `pipelines[].source.repo`: git URL to clone before running job steps
- `pipelines[].source.ref` (optional): branch/tag/ref to checkout

`steps` supports two step types:
- `run`: executes a shell command line.
- `test`: executes a dedicated test command and enables parsed test reports in job UI/API.
  - fields: `name` (optional), `command` (required), `format` (optional, currently `go-test-json`).

## Quick API test

```bash
# 1) Start server and agent in separate terminals.
go run ./cmd/ciwi server
go run ./cmd/ciwi agent

# 2) Open browser UI.
open http://127.0.0.1:8112/

# 3) Import a project from git (loads ciwi-project.yaml by default).
curl -s -X POST http://127.0.0.1:8112/api/v1/projects/import \
  -H 'Content-Type: application/json' \
  -d '{"repo_url":"https://github.com/izzyreal/ciwi.git","repo_ref":"main"}'

# 4) Find pipeline DB IDs.
curl -s http://127.0.0.1:8112/api/v1/projects

# Optional: reload an imported project definition from VCS.
curl -s -X POST http://127.0.0.1:8112/api/v1/projects/1/reload -d '{}'

# 5) Run a persisted pipeline by DB ID.
curl -s -X POST http://127.0.0.1:8112/api/v1/pipelines/1/run -d '{}'

# 6) Check jobs:
curl -s http://127.0.0.1:8112/api/v1/jobs
```
