# ciwi

![ciwi logo](internal/server/assets/ciwi-logo.png)

Simple, portable, single binary CI/CD server and agent.</br>
WIP.</br>
NOT SUITABLE FOR PUBLIC SERVERS.</br>
ONLY FOR PRIVATE NETWORKS AND HOMELAB STYLE PROJECTS.</br>

## Background

When OpenAI released Codex I decided to give it a try. In about 3 days I had a Jenkins and TeamCity alternative that worked well enough to suit some of my basic needs. In the next few weeks I'll see if becomes a project I will keep maintaining.

The nice thing about ciwi is that it's open source, free, has portable binaries of about 13MB, uses the same binary just in a different mode for agent, server or both. Memory usages is looking pretty good, with my home server instance reporting 72MB and my agents 10-21MB.

## Getting started

Check out the [automated installation scripts](#automated-installation-scripts). They set up a systemd persistent server or agent on Linux, agent service for Windows, or LaunchAgent for macOS. All of them are capable of self-updating. Go to the Global Settings ⚙️ (top-right on main page) to check for updates and roll back.

Check out the example `ciwi-project.yaml` files:
* [`ciwi-project.yaml`](https://github.com/izzyreal/cupuacu/blob/main/ciwi-project.yaml) for building and publishing cupuacu (a C++ SDL3 audio editor).
* [`ciwi-project.yaml`](ciwi-project.yaml) for building ciwi itself.

If do prefer not having the persistent server or agent that the installer scripts give you, you can run the binaries manually, giving you these options:

```bash
go run ./cmd/ciwi --help
go run ./cmd/ciwi server
go run ./cmd/ciwi agent
go run ./cmd/ciwi all-in-one
```
This comes with notably less zero-config-ness, so you'll have to read up on [environment variables](#environment-variables) below.

## Terminology

Canonical domain terminology lives in [`terminology.md`](terminology.md).

## Design philosophy

ciwi intentionally avoids fragile behavior that depends on parsing human-readable logs.

- ciwi is designed around explicit API contracts and structured payloads between server, agent, and UI.
- Features should use dedicated fields/endpoints (for example status/report payloads) instead of scraping job output text.
- Job output remains for humans; machine behavior should rely on typed data.

## Environment variables

Prefer the [automated installation scripts](#automated-installation-scripts), but if you wish to tinker, here are a few options:

- `CIWI_SERVER_ADDR`: server bind address (default `:8112`)
- `CIWI_DB_PATH`: sqlite database path (default `ciwi.db`)
- `CIWI_ARTIFACTS_DIR`: server artifact storage directory (default `ciwi-artifacts`)
- `CIWI_SERVER_URL`: agent target base URL (default `http://127.0.0.1:8112`)
- `CIWI_AGENT_ID`: override agent ID (default `agent-<hostname>`)
- `CIWI_AGENT_WORKDIR`: local working directory for job execution (default `.ciwi-agent/work`)
  - Workspace layout:
    - `workspaces/<project_id>_<project_name>_<pipeline_job_id>[_<matrix_name_or_idx-N>]_env-<fingerprint>`
    - non-matrix jobs omit the matrix segment
    - environment fingerprint is derived from required capabilities (`os`, `arch`, `shell`, `executor`)
  - Agent cache path remains `cache/` under this workdir.
- `CIWI_AGENT_ENV_FILE`: Windows-only env file auto-loaded by agent startup (default `%ProgramData%\\ciwi-agent\\agent.env`)
- `CIWI_AGENT_TRACE_SHELL`: enable shell command tracing (`set -x` for `posix`, `@echo on` for `cmd`, `Set-PSDebug -Trace 1` for `powershell`) (default `true`)
- `CIWI_AGENT_GO_BUILD_VERBOSE`: sets `GOFLAGS=-v` when unset (default `true`)
- `CIWI_WINDOWS_SERVICE_NAME`: Windows service name for service mode/self-update (default `ciwi-agent`)
- `CIWI_UPDATE_REPO`: GitHub repo for update checks (default `izzyreal/ciwi`)
- `CIWI_UPDATE_API_BASE`: GitHub API base URL (default `https://api.github.com`)
- `CIWI_LOG_LEVEL`: log verbosity (`debug`, `info`, `warn`, `error`; default `info`)

Build-time version embedding:
- Version is embedded via linker flag `-X github.com/izzyreal/ciwi/internal/version.Version=<value>`.
- If not set at build time, ciwi reports `dev`.

## Server prerequisites

- `git` must be installed on the server host to import projects from git repositories.
- Project import fetches only git metadata + the root config file (no full repo checkout).
- `git` is also used by pipeline versioning (`pipelines[].versioning`) to resolve a single run version + pinned source commit.

## Agent prerequisites

- `git` must be installed on the agent host for pipeline jobs that define `source.repo`.
- `gh` must be installed on the agent host for GitHub release steps that use GitHub CLI.

## Tool capabilities and requirements

Agents automatically detect common shell tools and report versions in heartbeats:
- `git`, `go`, `gh`, `cmake`, `sccache`, `ccache`, `ninja`, `docker`, `gcc`, `clang`, `xcodebuild`, `msvc`, `iscc`, `signtool`, `codesign`, `productsign`, `packagesbuild`, `packagesutil`, `notarytool`, `stapler`, `plistbuddy`, `xorg-dev` (when present)

Use `requires.tools` in pipeline jobs to constrain tool presence/version:

```yaml
jobs:
  - id: build
    runs_on:
      os: linux
      arch: amd64
      executor: script
      shell: posix
    requires:
      tools:
        go: ">=1.24"
        git: ">=2.30"
        gh: "*"
```

Constraint syntax supports:
- presence only: `*` (or empty)
- version compare: `>=`, `>`, `<=`, `<`, `=`, `==`
- synthetic tool flags are also supported (for example `xorg-dev: "*"` when Linux X11 development headers/libs are available on the agent host)

From `/agents`, use **Refresh Tools** to request an on-demand re-scan on an agent.

Container runtime probe (opt-in):
- For containerized jobs, set:
  - `runs_on.container_image`: container image used by ciwi to start/manage the runtime container before requirement validation
  - `runs_on.container_devices` (optional): comma-separated host device paths to pass through to the runtime container (for example `/dev/snd`)
  - `runs_on.container_groups` (optional): comma-separated supplemental groups passed as `docker run --group-add` (for example `audio`)
- When `runs_on.container_image` is set, ciwi executes job steps inside that managed container (`docker exec`) instead of directly on the host.
- Source checkout and cache directories are bind-mounted into the runtime container by the agent.
- ciwi will probe tool versions inside that container using dedicated structured API fields, and expose them on the Job Details page under runtime capability data.
- This probe data is stored as `runtime_capabilities` on the job execution and does not rely on output log scraping.

## Job History actions

UI actions map to these server behaviors:
- **Run Again** clones an existing job execution into a new queued job with the same script, env, capabilities, source repo/ref metadata and step plan.
- **Run Again** keeps whatever source ref string is stored on that job execution.
- If that stored source ref is a pinned commit SHA, rerun checks out the same commit.
- If that stored source ref is a moving branch/tag ref, rerun fetches it again at execution time and may build a newer commit.
- **Run Again** creates a new job execution ID with fresh logs and artifact records; prior artifacts are kept and not replaced.
- **Run Again** is useful for fast retries after flaky failures or after fixing agent/tooling issues, without re-enqueueing the full pipeline.
- Job details include a **Cache statistics** panel populated from structured agent status payloads (`cache_stats`), not log parsing.
- **Flush History** deletes job executions whose status is not `queued`, `leased`, or `running`.
- **Flush History** removes execution logs/status payloads in sqlite for flushed jobs.
- **Flush History** does not remove artifact files from disk (`CIWI_ARTIFACTS_DIR`); it is history cleanup, not artifact GC.
- Artifact upload limits (agent defaults): at most `2500` files per job, max `50MB` per file.

## FetchContent source caching

ciwi cache entries are generic directory caches. For CMake FetchContent, the supported use case is:
- cache dependency **sources** across jobs/runs
- keep build/subbuild output job-local

Recommended pattern:
- choose a ciwi-owned cache env var name, for example `CIWI_FETCHCONTENT_SOURCES_DIR`
- configure `caches[].env` to that variable
- pass one project-level CMake switch, for example `-DFETCHCONTENT_CACHE_ROOT="$CIWI_FETCHCONTENT_SOURCES_DIR"`
- in `CMakeLists.txt`, resolve all FetchContent source dirs from that one switch

Example:

```yaml
jobs:
  - id: build
    caches:
      - id: fetchcontent
        env: CIWI_FETCHCONTENT_SOURCES_DIR
    steps:
      - run: mkdir -p "$CIWI_FETCHCONTENT_SOURCES_DIR"
      - run: >
          cmake -S . -B build
          -DFETCHCONTENT_CACHE_ROOT="$CIWI_FETCHCONTENT_SOURCES_DIR"
```

Notes:
- `CIWI_FETCHCONTENT_SOURCES_DIR` is just an env var name you define in ciwi config; CMake does not know it directly.
- `FETCHCONTENT_CACHE_ROOT` is a project-defined CMake variable (example name), not a ciwi built-in.
- Your `CMakeLists.txt` should map that variable to per-dependency FetchContent source dirs internally.

Cache behavior:
- ciwi cache directory path is stable and based on `caches[].id`.
- for FetchContent use, put hash-specific dependency subdirectories under `FETCHCONTENT_CACHE_ROOT` in CMake helper logic.

Moving-target caveat:
- if dependencies track moving refs (for example `main`), use per-dependency hash subdirs in your CMake helper under `FETCHCONTENT_CACHE_ROOT`.
- best fix is still pinning dependencies to immutable tags/SHAs where possible.
- fallback invalidation option:
- rotate `caches[].id` (for example `fetchcontent` -> `fetchcontent-v2`) for explicit cache cutovers.

## Project icon auto-discovery

When importing/reloading a git project, ciwi tries to auto-discover and store one project icon:

- scans files from `FETCH_HEAD` and keeps candidates whose basename contains `icon` or `logo`
- supports `.png`, `.jpg`, `.jpeg`, `.bmp`
- size guard: `>0` and `<=500KB`
- ranking: larger file first, then shallower path depth, then path lexicographically
- validates bytes via MIME detection and stores `image/png`, `image/jpeg`, or `image/bmp`

Stored icon is served at `/api/v1/projects/{projectId}/icon` and shown in project/job UI when present.

# Automated installation scripts

## macOS agent installer (LaunchAgent)

For macOS build/signing workflows, run ciwi agent as a **LaunchAgent** (user session), not a LaunchDaemon.

Install with GitHub API token (recommended to avoid rate limits):

```bash
export CIWI_GITHUB_TOKEN="<your-token>"
curl -fsSL -o /tmp/install_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_macos.sh && \
sh /tmp/install_ciwi_agent_macos.sh
```

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
- Installs agent binary into `~/.local/bin/ciwi` (user-writable) so self-update can swap binaries in place.
- Applies ad-hoc code signing to the installed binary (`codesign --force --sign -`) so launchd can execute updated binaries without a full signing/notarization setup.
- Stores `CIWI_GITHUB_TOKEN` in the agent LaunchAgent env when provided, and preserves existing token on reinstall if not provided.
- Installs two LaunchAgents:
  - `nl.izmar.ciwi.agent` (main agent)
  - `nl.izmar.ciwi.agent-updater` (oneshot staged updater used for self-update)
- Installs `/etc/newsyslog.d/ciwi-<user>.conf` to cap ciwi log files at 100MB (agent logs and optional server logs in `~/Library/Logs/ciwi`).

Server identity validation during install checks:
- `GET /healthz` returns `{"status":"ok"}`
- `GET /api/v1/server-info` returns `{"name":"ciwi","api_version":1,"hostname":"<host>",...}`

After install:

```bash
launchctl print gui/$(id -u)/nl.izmar.ciwi.agent
launchctl print gui/$(id -u)/nl.izmar.ciwi.agent-updater
tail -f "$HOME/Library/Logs/ciwi/agent.out.log" "$HOME/Library/Logs/ciwi/agent.err.log"
```

Manage agent lifecycle (run as the logged-in user, no sudo):

```bash
# Disable auto-start at login
launchctl disable gui/$(id -u)/nl.izmar.ciwi.agent

# Re-enable auto-start at login
launchctl enable gui/$(id -u)/nl.izmar.ciwi.agent

# Restart the running agent process
launchctl kickstart -k gui/$(id -u)/nl.izmar.ciwi.agent

# Stop now (boot out from current GUI session)
launchctl bootout gui/$(id -u)/nl.izmar.ciwi.agent

# Start now (bootstrap from plist)
launchctl bootstrap gui/$(id -u) $HOME/Library/LaunchAgents/nl.izmar.ciwi.agent.plist
```

### macOS agent uninstall

One-line uninstall (no options):

```bash
curl -fsSL -o /tmp/uninstall_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_macos.sh && \
sh /tmp/uninstall_ciwi_agent_macos.sh
```

Uninstaller behavior:
- Stops/unloads LaunchAgents `nl.izmar.ciwi.agent` and `nl.izmar.ciwi.agent-updater`
- Removes `~/Library/LaunchAgents/nl.izmar.ciwi.agent.plist` and `~/Library/LaunchAgents/nl.izmar.ciwi.agent-updater.plist`
- Removes ciwi binary from `~/.local/bin/ciwi` and `/usr/local/bin/ciwi` (with sudo if needed)
- Removes `/etc/newsyslog.d/ciwi-<user>.conf` (with sudo when available)
- Leaves logs/workdir by default (`~/Library/Logs/ciwi`, `~/.ciwi-agent/work`) and prints cleanup command

## Windows agent installer (Service)

Run from an elevated PowerShell session (Run as Administrator).

Install with GitHub API token (recommended to avoid rate limits):

```powershell
$env:CIWI_GITHUB_TOKEN = "<your-token>"
$script = Join-Path $env:TEMP ("install_ciwi_agent_windows_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_windows.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

One-line install (no options):

```powershell
$script = Join-Path $env:TEMP ("install_ciwi_agent_windows_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_windows.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

Installer behavior:
- If `CIWI_SERVER_URL` is unset, tries auto-discovery first:
- Probe step: checks `http://127.0.0.1:8112` and `http://localhost:8112`.
- Probe step: checks LAN neighbors from `arp -a` as `http://<ip>:8112`.
- Validation step: requires `GET /healthz` and `GET /api/v1/server-info` to match ciwi API identity.
- URL preference step: prefers hostname URLs when resolvable (for example `http://bhakti.local:8112`).
- If multiple servers are found, prompts you to choose one.
- If none are found, prompts for server URL.
- Downloads latest `ciwi-windows-<arch>.exe` release asset.
- Verifies SHA256 using `ciwi-checksums.txt`.
- Installs `C:\\Program Files\\ciwi\\ciwi.exe`.
- Creates/updates service `ciwi-agent` with command `"C:\\Program Files\\ciwi\\ciwi.exe agent"`.
- Writes `%ProgramData%\\ciwi-agent\\agent.env` with:
  - `CIWI_SERVER_URL`
  - `CIWI_AGENT_ID`
  - `CIWI_AGENT_WORKDIR`
  - `CIWI_LOG_LEVEL`
  - `CIWI_AGENT_TRACE_SHELL`
  - `CIWI_WINDOWS_SERVICE_NAME`
  - optional `CIWI_GITHUB_TOKEN` (preserved on reinstall if not passed)
- Starts `ciwi-agent`.

After install:

```powershell
Get-Service ciwi-agent
sc.exe qc ciwi-agent
sc.exe query ciwi-agent
```

### Windows agent uninstall

One-line uninstall (elevated PowerShell):

```powershell
$script = Join-Path $env:TEMP ("uninstall_ciwi_agent_windows_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_windows.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

Uninstaller behavior:
- Stops/deletes service `ciwi-agent`.
- Removes binary `C:\\Program Files\\ciwi\\ciwi.exe`.
- Optionally removes `%ProgramData%\\ciwi-agent` (workdir/logs/config) after confirmation.

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
- Installs `/etc/logrotate.d/ciwi` (rotates server logs at 100MB).

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

## Linux agent installer (systemd)

Install with GitHub API token (recommended to avoid rate limits):

```bash
export CIWI_GITHUB_TOKEN="<your-token>"
curl -fsSL -o /tmp/install_ciwi_agent_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_linux.sh && \
sh /tmp/install_ciwi_agent_linux.sh
```

One-line install (no options):

```bash
curl -fsSL -o /tmp/install_ciwi_agent_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_linux.sh && \
sh /tmp/install_ciwi_agent_linux.sh
```

One-line uninstall (no options):

```bash
curl -fsSL -o /tmp/uninstall_ciwi_agent_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_linux.sh && \
sh /tmp/uninstall_ciwi_agent_linux.sh
```

Installer behavior:
- Downloads latest `ciwi-linux-<arch>` release asset.
- Verifies SHA256 using `ciwi-checksums.txt`.
- Installs `/usr/local/bin/ciwi`.
- Creates system user `ciwi-agent`.
- Installs and starts `ciwi-agent.service` via systemd.
- Installs `/etc/logrotate.d/ciwi-agent` (rotates agent logs at 100MB).
- Writes `/etc/default/ciwi-agent` with:
  - `CIWI_SERVER_URL` (default `http://127.0.0.1:8112`)
  - `CIWI_AGENT_ID`
  - `CIWI_AGENT_WORKDIR`
  - `CIWI_GITHUB_TOKEN` (if provided during install)

If your jobs need Docker and/or host audio device access, add `ciwi-agent` to the relevant groups:

```bash
sudo usermod -aG docker ciwi-agent
sudo usermod -aG audio ciwi-agent
sudo systemctl restart ciwi-agent
```

Verify:

```bash
id ciwi-agent; getent group docker; getent group audio
```

Default paths:
- Binary: `/usr/local/bin/ciwi`
- Env file: `/etc/default/ciwi-agent`
- Service: `ciwi-agent.service`
- Work/data: `/var/lib/ciwi-agent`
- Logs: `/var/log/ciwi-agent/agent.out.log`, `/var/log/ciwi-agent/agent.err.log`

After install:

```bash
sudo systemctl status ciwi-agent
sudo journalctl -u ciwi-agent -f
```

## Backend API

### Shared across consumers

- Agent + Frontend:
  - `GET /api/v1/jobs/{id}`
  - `GET /api/v1/jobs/{id}/artifacts`
- Frontend + Installer:
  - `GET /healthz`

### Consumed by agent runtime

- `POST /api/v1/heartbeat`
- `POST /api/v1/agent/lease`
- `POST /api/v1/jobs/{id}/status`
- `POST /api/v1/jobs/{id}/artifacts`
- `POST /api/v1/jobs/{id}/tests`

### Consumed by frontend UI

- `GET /healthz`
- `GET /api/v1/agents`
- `GET /api/v1/agents/{agentId}`
- `POST /api/v1/agents/{agentId}/actions` (`action`: `update`, `refresh-tools`, `restart`, `run-script`)
- `GET /api/v1/projects`
- `POST /api/v1/projects/import`
- `GET /api/v1/projects/{projectId}`
- `GET /api/v1/projects/{projectId}/icon`
- `POST /api/v1/projects/{projectId}/reload`
- `GET /api/v1/projects/{projectId}/vault`
- `PUT /api/v1/projects/{projectId}/vault`
- `POST /api/v1/projects/{projectId}/vault-test`
- `POST /api/v1/pipelines/{pipelineDbId}/run-selection`
- `GET /api/v1/pipelines/{pipelineDbId}/version-resolve` (SSE)
- `POST /api/v1/pipeline-chains/{chainDbId}/run`
- `GET /api/v1/vault/connections`
- `POST /api/v1/vault/connections`
- `DELETE /api/v1/vault/connections/{id}`
- `POST /api/v1/vault/connections/{id}/test`
- `GET /api/v1/jobs`
- `DELETE /api/v1/jobs/{id}`
- `POST /api/v1/jobs/clear-queue`
- `POST /api/v1/jobs/flush-history`
- `POST /api/v1/jobs/{id}/cancel`
- `POST /api/v1/jobs/{id}/rerun`
- `GET /api/v1/jobs/{id}/tests`
- `POST /api/v1/update/check`
- `POST /api/v1/update/apply`
- `POST /api/v1/update/rollback`
- `POST /api/v1/server/restart`
- `GET /api/v1/update/tags`
- `GET /api/v1/update/status`

Agent update policy:
- Server-requested agent updates use **drain queue** behavior.
- Each agent applies its pending update only after it finishes its current and already-queued eligible jobs; no immediate preemption mode is exposed in UI/API.
- Server self-update/rollback controls are enabled only when ciwi is running under a supported service manager.
- In dev mode (`go run` / `version=dev`) and standalone server mode, update/rollback controls stay visible but disabled, with guidance in Global Settings.
- Agents not running as a service report self-update disabled failures, and Global Settings shows a snackbar with a link to the automated installer docs.

### Consumed by installer/provisioning scripts

- `GET /healthz`
- `GET /api/v1/server-info`

Pipeline configs (for example root `ciwi-project.yaml`) require:
- `pipelines[].source.repo`: git URL to clone before running job steps
- `pipelines[].source.ref` (optional): branch/tag/ref/commit SHA to checkout
  - Agent behavior: clone default branch, then `git fetch origin <ref>` and `git checkout --force FETCH_HEAD`
- `pipelines[].depends_on` (optional): list of pipeline IDs that must have latest successful run before enqueue

Optional pipeline versioning:
- `pipelines[].versioning.file` (default `VERSION`): file read once per pipeline run from source checkout at a pinned commit.
- `pipelines[].versioning.tag_prefix` (default `v`): prepended to `x.y.z` from version file.
- `pipelines[].versioning.auto_bump` (`patch|minor|major`): ciwi-managed bump/push after successful run (currently requires that the run resolves to exactly one job execution).

When versioning is active, ciwi injects env vars into every job in that pipeline run:
- `CIWI_PIPELINE_VERSION_RAW` (for example `1.2.3`)
- `CIWI_PIPELINE_VERSION` / `CIWI_PIPELINE_TAG` (for example `v1.2.3`)
- `CIWI_PIPELINE_TAG_PREFIX` (for example `v`)
- `CIWI_PIPELINE_SOURCE_REF` (resolved commit SHA)
- `CIWI_PIPELINE_SOURCE_REPO`
- `CIWI_PIPELINE_VERSION_FILE`

`depends_on` pipelines inherit dependency run version/source metadata, so chained runs (for example `build -> release`) stay version-consistent end-to-end.

Config parsing uses strict YAML field validation (`KnownFields`), so unknown keys are rejected.

`steps` supports two step types:
- `run`: executes a script line in the shell defined by `runs_on.shell`.
- `test`: executes a dedicated test command and enables parsed test reports in job UI/API.
  - fields: `name` (optional), `command` (required), `format` (optional, supports `go-test-json` and `junit-xml`), `report` (required), `coverage_format` (optional, supports `go-coverprofile` and `lcov`), `coverage_report` (optional).

Executor model:
- `runs_on.executor` must be `script`.
- `runs_on.shell` is required when `runs_on.executor=script` and must be `posix`, `cmd`, or `powershell`.

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
curl -s -X POST http://127.0.0.1:8112/api/v1/pipelines/1/run-selection -d '{}'

# 6) Check jobs:
curl -s http://127.0.0.1:8112/api/v1/jobs
```

## Screenshots

<img width="1091" height="702" alt="image" src="https://github.com/user-attachments/assets/f6ae903c-40a0-47f5-b961-8ec0611f5e3c" />
<img width="1101" height="712" alt="image" src="https://github.com/user-attachments/assets/35a88fd3-b612-4781-b306-ff476df75f31" />
<img width="1089" height="715" alt="image" src="https://github.com/user-attachments/assets/f0c9f1ab-5a5b-44b9-9207-c7fb295b09a0" />
<img width="1095" height="699" alt="image" src="https://github.com/user-attachments/assets/1515fc10-466f-478e-bc3a-b2669e612c90" />
<img width="1096" height="710" alt="image" src="https://github.com/user-attachments/assets/c4d6ccda-bc09-4645-8372-e80fd02290f4" />
