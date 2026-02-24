# Getting Started

This guide gets a private ciwi setup running quickly.

## 1. Install server and at least one agent

Use the automated installers from [`docs/installation.md`](installation.md).

Recommended defaults:
- Linux server: `install_server_linux.sh`
- Linux/macOS/Windows agents: respective installer scripts

## 2. Verify server health

```bash
curl -s http://127.0.0.1:8112/healthz
curl -s http://127.0.0.1:8112/api/v1/server-info
```

## 3. Open UI and import a project

Open `http://127.0.0.1:8112/`.

From UI:
- Import project from git
- ciwi loads `ciwi-project.yaml`

API equivalent:

```bash
curl -s -X POST http://127.0.0.1:8112/api/v1/projects/import \
  -H 'Content-Type: application/json' \
  -d '{"repo_url":"https://github.com/izzyreal/ciwi.git","repo_ref":"main"}'
```

## 4. Run a pipeline

From UI: open project, run pipeline/chain.
- `Run` / `Dry Run` starts immediately.
- Hold `Shift` while clicking to open the custom run modal (choose branch and optional eligible agent).
- `Preview Dry Run` shows the planned jobs/capabilities without enqueueing.

API equivalent:

```bash
curl -s http://127.0.0.1:8112/api/v1/projects
curl -s -X POST http://127.0.0.1:8112/api/v1/pipelines/1/run-selection -d '{}'
curl -s http://127.0.0.1:8112/api/v1/jobs
```

Offline cached execution example:

```bash
curl -s -X POST http://127.0.0.1:8112/api/v1/pipelines/1/run-selection \
  -H 'Content-Type: application/json' \
  -d '{"execution_mode":"offline_cached","dry_run":true}'
```

## 5. Optional: manual runtime modes

If you do not want service-managed installs:

```bash
go run ./cmd/ciwi --help
go run ./cmd/ciwi server
go run ./cmd/ciwi agent
go run ./cmd/ciwi all-in-one
```

In manual mode, read [`docs/configuration.md`](configuration.md) for env vars and prerequisites.

## Next

- Config format and pipeline behavior: [`docs/pipelines.md`](pipelines.md)
- Host/container tool requirements: [`docs/configuration.md`](configuration.md)
- Runtime architecture and flows: [`docs/architecture.md`](architecture.md)
