# ciwi
Simple CI/CD and build automation system

![ciwi logo](internal/server/assets/ciwi-logo.png)

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
- `CIWI_UPDATE_REPO`: GitHub repo for update checks (default `izzyreal/ciwi`)
- `CIWI_UPDATE_API_BASE`: GitHub API base URL (default `https://api.github.com`)
- `CIWI_LOG_LEVEL`: log verbosity (`debug`, `info`, `warn`, `error`; default `info`)

Build-time version embedding:
- Version is embedded via linker flag `-X github.com/izzyreal/ciwi/internal/version.Version=<value>`.
- If not set at build time, ciwi reports `dev`.

## Server prerequisites

- `git` must be installed on the server host to import projects from git repositories.
- Project import fetches only git metadata + the root config file (no full repo checkout).

## Agent prerequisites

- `git` must be installed on the agent host for pipeline jobs that define `source.repo`.
- `gh` must be installed on the agent host for GitHub release steps that use GitHub CLI.

## macOS agent installer (LaunchAgent)

For macOS build/signing workflows, run ciwi agent as a **LaunchAgent** (user session), not a LaunchDaemon.

One-line install (no options, tries auto-discovery first, prompts if needed):

```bash
curl -fsSL -o /tmp/install_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_macos.sh && \
sh /tmp/install_ciwi_agent_macos.sh
```

Installer behavior:
- Tries mDNS/Bonjour discovery first (`_ciwi._tcp`), then falls back to probing `http://<local-ip>:8112`.
- Prefers hostname-based URLs when resolvable (for example `http://bhakti.local:8112`) and deduplicates same host seen via multiple IPs/adapters.
- If multiple servers are found, prompts you to choose one.
- If none are found, prompts for server URL.
- Prompts for sudo only if needed to install into `/usr/local/bin`; otherwise falls back to `~/.local/bin`.

Server identity validation during install checks:
- `GET /healthz` returns `{"status":"ok"}`
- `GET /api/v1/server-info` returns `{"name":"ciwi","api_version":1,...}`

After install:

```bash
launchctl print gui/$(id -u)/nl.izmar.ciwi.agent
tail -f "$HOME/Library/Logs/ciwi/agent.out.log" "$HOME/Library/Logs/ciwi/agent.err.log"
```

### macOS agent uninstall

One-line uninstall (no options):

```bash
curl -fsSL -o /tmp/uninstall_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_macos.sh && \
sh /tmp/uninstall_ciwi_agent_macos.sh
```

Uninstaller behavior:
- Stops/unloads LaunchAgent `nl.izmar.ciwi.agent`
- Removes `~/Library/LaunchAgents/nl.izmar.ciwi.agent.plist`
- Removes ciwi binary from `~/.local/bin/ciwi` and `/usr/local/bin/ciwi` (with sudo if needed)
- Leaves logs/workdir by default (`~/Library/Logs/ciwi`, `~/.ciwi-agent`) and prints cleanup command

## Linux server installer (systemd)

One-line install (no options):

```bash
curl -fsSL -o /tmp/install_ciwi_server_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_server_linux.sh && \
sh /tmp/install_ciwi_server_linux.sh
```

One-line uninstall (no options):

```bash
curl -fsSL -o /tmp/uninstall_ciwi_server_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_server_linux.sh && \
sh /tmp/uninstall_ciwi_server_linux.sh
```

Installer behavior:
- Downloads latest `ciwi-linux-<arch>` release asset.
- Verifies SHA256 using `ciwi-checksums.txt`.
- Installs `/usr/local/bin/ciwi`.
- Creates system user `ciwi` and data/log directories under `/var/lib/ciwi` and `/var/log/ciwi`.
- Installs and starts `ciwi.service` via systemd.
- Installs `ciwi-updater.service` (oneshot, root) for staged self-updates.
- Installs `/etc/polkit-1/rules.d/90-ciwi-updater.rules` so user `ciwi` can start only the updater unit.

Default paths:
- Binary: `/usr/local/bin/ciwi`
- Env file: `/etc/default/ciwi`
- SQLite DB: `/var/lib/ciwi/ciwi.db`
- Artifacts: `/var/lib/ciwi/artifacts`
- Update staging: `/var/lib/ciwi/updates`
- Logs: `/var/log/ciwi/server.out.log`, `/var/log/ciwi/server.err.log`

After install:

```bash
sudo systemctl status ciwi
sudo journalctl -u ciwi -f
curl -s http://127.0.0.1:8112/healthz
```

## First functional API slice

- `GET /` minimal web UI (projects/pipelines/jobs)
- `GET /projects/{projectId}` project page with structure, per-matrix run buttons and execution history
- `GET /healthz` returns `{"status":"ok"}`
- `POST /api/v1/heartbeat` accepts agent heartbeats in JSON
- `GET /api/v1/agents` returns known agents
- `POST /api/v1/projects/import` imports a project from git (`ciwi-project.yaml` by default)
- `POST /api/v1/projects/{projectId}/reload` reloads project definition from saved VCS settings
- `GET/PUT /api/v1/projects/{projectId}/vault` gets/updates project Vault settings
- `POST /api/v1/projects/{projectId}/vault-test` tests project Vault access + mapped secrets
- `POST /api/v1/jobs` enqueues a job execution
- `GET /api/v1/jobs` returns all job executions
- `GET /api/v1/jobs/{id}` returns one job execution
- `DELETE /api/v1/jobs/{id}` removes a queued/leased job execution
- `POST /api/v1/jobs/clear-queue` removes all queued/leased job executions
- `POST /api/v1/jobs/flush-history` removes all finished job executions from history
- `POST /api/v1/jobs/{id}/status` updates job execution status (`running`, `succeeded`, `failed`)
- `GET /api/v1/jobs/{id}/artifacts` lists uploaded artifacts for a job execution
- `POST /api/v1/jobs/{id}/artifacts` uploads artifacts for a job execution (agent use)
- `GET /api/v1/jobs/{id}/tests` returns parsed test report for a job execution
- `POST /api/v1/jobs/{id}/tests` uploads parsed test report for a job execution (agent use)
- `GET/POST /api/v1/vault/connections` lists/upserts Vault AppRole connections
- `DELETE /api/v1/vault/connections/{id}` deletes a Vault connection
- `POST /api/v1/vault/connections/{id}/test` tests Vault connection auth and optional read
- `POST /api/v1/agent/lease` leases a matching queued job to an agent
- `GET /api/v1/projects` returns persisted projects with pipelines
- `GET /api/v1/projects/{projectId}` returns full project structure (pipelines/jobs/matrix)
- `POST /api/v1/pipelines/run` loads `ciwi.yaml` and enqueues pipeline jobs
- `POST /api/v1/pipelines/{pipelineDbId}/run` runs a persisted pipeline from sqlite (optional `{ "dry_run": true }`)
- `POST /api/v1/pipelines/{pipelineDbId}/run-selection` runs a selected job/matrix include (optional `{ "dry_run": true }`)
- `POST /api/v1/update/check` checks latest GitHub release compatibility/version
- `POST /api/v1/update/apply` downloads latest compatible binary, starts helper, and restarts process

Pipeline configs (for example root `ciwi-project.yaml`) require:
- `pipelines[].source.repo`: git URL to clone before running job steps
- `pipelines[].source.ref` (optional): branch/tag/ref to checkout
- `pipelines[].depends_on` (optional): list of pipeline IDs that must have latest successful run before enqueue

Config parsing uses strict YAML field validation (`KnownFields`), so unknown keys are rejected.

`steps` supports two step types:
- `run`: executes a shell command line.
- `test`: executes a dedicated test command and enables parsed test reports in job UI/API.
  - fields: `name` (optional), `command` (required), `format` (optional, currently `go-test-json`).

Step-level env vars are supported:
- `steps[].env` key/value pairs are passed to the job process environment.
- Secret placeholders inside env values are resolved at lease-time:
  - `{{ secret.<name> }}`

## Vault setup (AppRole)

Vault is configured in two layers:
1. Global Vault connection (`/vault` page)
2. Per-project secret mappings (Project page -> "Vault Access")

You can also define project Vault mappings in `ciwi-project.yaml` under `project.vault`; on import/reload/load this is synced into sqlite.

### 1) Add Vault connection

Open `/vault` and create a connection with:
- `name` (e.g. `home-vault`)
- `url` (e.g. `http://bhakti.local:8200`)
- `approle_mount` (usually `approle`)
- `role_id`
- `secret_id_env` (required): name of environment variable that contains the AppRole Secret ID

Then use the **Test** button to validate AppRole login (and optional read checks through project test flow).

### 2) Configure project Vault access

Open a project page and in **Vault Access**:
- select the Vault connection
- define mappings, one per line:
  - `name=mount/path#key`
  - example: `github_secret=kv/gh#token`
- click **Save Vault Settings**
- click **Test**

### 3) Use mapped secrets in pipeline YAML

Example `ciwi-project.yaml` step:

```yaml
steps:
  - run: github-release ... --security-token "$GITHUB_SECRET"
    env:
      GITHUB_SECRET: "{{ secret.github_secret }}"
```

This references the project mapping named `github_secret`.

### Optional: define mappings in `ciwi-project.yaml`

```yaml
project:
  name: ciwi
  vault:
    connection: home-vault
    secrets:
      - name: github-secret
        mount: kv
        path: gh
        key: token
        kv_version: 2
```

## Vault security model

- Secrets are resolved **just-in-time** when an agent leases a job.
- Plaintext secret values are **not persisted** in sqlite.
- Jobs with secrets automatically disable shell trace (`set -x`) for safer logs.
- Job output streaming/final logs redact known secret values as `***`.

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
