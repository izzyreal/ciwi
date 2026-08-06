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

## 3. Open the UI, authorize the agent, and import a project

Open `http://127.0.0.1:8112/`.

From the UI:

- Open **Agents** and authorize the new agent. Unknown agents are intentionally
  unable to lease jobs until authorized.
- Open **Global Settings**, import a repository project, and let ciwi load its
  `ciwi-project.yaml`; or create a self-contained **Managed YAML** project.

API equivalent:

```bash
curl -s -X POST http://127.0.0.1:8112/api/v1/projects/import \
  -H 'Content-Type: application/json' \
  -d '{"repo_url":"https://github.com/izzyreal/ciwi.git","repo_ref":"main"}'
```

## 4. Run a pipeline

From the browser UI, open a project and run a pipeline or chain.

- **Run** / **Dry Run** starts immediately.
- Hold `Shift` while clicking to open custom run options, including source ref
  and an optional eligible agent.
- **Preview Dry Run** shows planned jobs and capabilities without enqueueing.

In the native client, use **Options** for source-ref and agent selection, then
run normally or as a dry run.

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
go run ./cmd/ciwi-desktop -addr tcp://127.0.0.1:8113
```

The desktop command is supported on macOS, Windows, and Linux. The server
enables both CNP transports on port 8113 by default. In manual mode, read
[`configuration.md`](configuration.md) for environment variables and
prerequisites, and [`native-client.md`](native-client.md) for discovery and SSH
connection options.

## Next

- Config format and pipeline behavior: [`docs/pipelines.md`](pipelines.md)
- Progress indicators and historical duration estimates: [`docs/progress-indicators.md`](progress-indicators.md)
- Host/container tool requirements: [`docs/configuration.md`](configuration.md)
- Runtime architecture and flows: [`docs/architecture.md`](architecture.md)
- Runtime operations (including agent activate/deactivate): [`docs/operations.md`](operations.md)
