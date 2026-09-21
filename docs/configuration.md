# Configuration

## Environment variables

Common variables:
- `CIWI_SERVER_ADDR`: server bind address (default `:8112`)
- `CIWI_NATIVE_ADDR`: common CNP v1 QUIC/UDP and TCP bind address (default `:8113`); set to `off` to disable both
- `CIWI_NATIVE_QUIC_ADDR`: optional QUIC-specific bind override; set to `off` to disable QUIC only
- `CIWI_NATIVE_TCP_ADDR`: optional TCP-specific bind override; set to `off` to disable TCP only
- `CIWI_MDNS_ENABLE`: advertise HTTP and enabled native endpoints over mDNS (default `true`)
- `CIWI_MDNS_INSTANCE`: override the advertised mDNS instance name
- `CIWI_DB_PATH`: sqlite path (default `ciwi.db`)
- `CIWI_ARTIFACTS_DIR`: artifact root (default `ciwi-artifacts`)
- `CIWI_SERVER_URL`: agent target URL (default `http://127.0.0.1:8112`)
- `CIWI_AGENT_ID`: override agent ID
- `CIWI_AGENT_WORKDIR`: agent work dir (default `.ciwi-agent/work`)
- `CIWI_AGENT_ENV_FILE`: service env file override (macOS default
  `$HOME/Library/Application Support/ciwi/agent.env`; Windows default
  `%ProgramData%\\ciwi-agent\\agent.env`)
- `CIWI_AGENT_LOG_FILE`: agent log override (macOS default
  `$HOME/Library/Logs/ciwi/agent.log`; Windows default
  `%ProgramData%\ciwi-agent\logs\agent.log`). Windows rotates at 10 MiB
  with three backups and honors `CIWI_LOG_LEVEL` from `agent.env`.
- `CIWI_AGENT_TRACE_SHELL`: shell tracing (default `true`)
- `CIWI_AGENT_GO_BUILD_VERBOSE`: sets `GOFLAGS=-v` when unset (default `true`)
- `CIWI_ARTIFACT_LOG_LEVEL`: artifact collection log verbosity: `none|summary|verbose` (default `summary`)
- `CIWI_ARTIFACT_LOG_MAX_INCLUDE_LINES`: max per-file `[artifacts] include=...` lines when level is `verbose` (default `25`)
- `CIWI_DEP_ARTIFACT_LOG_LEVEL`: dependency artifact restore log verbosity: `none|summary|verbose` (default `summary`)
- `CIWI_DEP_ARTIFACT_LOG_MAX_RESTORED_LINES`: max per-file `[dep-artifacts] restored=...` lines when level is `verbose` (default `25`)
- `CIWI_WINDOWS_SERVICE_NAME`: Windows service name (default `ciwi-agent`)
- `CIWI_UPDATE_REPO`: update repo (default `izzyreal/ciwi`)
- `CIWI_UPDATE_API_BASE`: update API base (default `https://api.github.com`)
- `CIWI_UPDATE_CHECKSUM_ASSET`: release checksum asset (default `ciwi-checksums.txt`)
- `CIWI_UPDATE_REQUIRE_CHECKSUM`: require a matching release checksum (default `true`)
- `CIWI_GITHUB_TOKEN`: optional GitHub token used by installers and server/agent release updates
- `CIWI_LOG_LEVEL`: `debug|info|warn|error` (default `info`)

Native client variables:

- `CIWI_NATIVE_SERVER`: explicit CNP endpoint, equivalent to `ciwi-desktop -addr`; accepts `quic://host:port`, `tcp://host:port`, or scheme-less QUIC `host:port`
- `CIWI_NATIVE_THEME`: shared theme name, equivalent to `ciwi-desktop -theme`
- `CIWI_NATIVE_ROUTE`: initial native route, equivalent to `ciwi-desktop -route`
- `CIWI_IOS_BUILD_NUMBER`: optional positive integer used as the iOS
  `CFBundleVersion` during local or CI archive builds (default `1`)

Build-time version embedding:
- `-X github.com/izzyreal/ciwi/internal/version.Version=<value>`

## Server prerequisites

- `git` on server host for project import/reload and versioning resolution.

## Agent prerequisites

- `git` for jobs with `vcs_source.repo`
- `gh` for release steps using GitHub CLI

## Tool capability detection and requirements

Agent reports tool versions in heartbeat.

Supported tool keys include:
- `git`, `go`, `gh`, `lftp`, `lcov`, `cmake`, `ninja`, `docker`, `gcc`, `clang`, `zip`
- `sphinx-build`, `rinoh`
- `python`, `python3` (detected independently on hosts and in managed containers)
- `ccache`, `sccache`
- macOS signing/packaging tools such as `xcodebuild`, `dmgbuild`, `codesign`, `productsign`, `notarytool`, `stapler`, `packagesbuild`, `packagesutil`, `plistbuddy`
- Windows `msvc`, `iscc`, `wix`, `signtool`
- synthetic host capability `xorg-dev`

Use `requires.tools` in job config:

```yaml
requires:
  tools:
    go: ">=1.24"
    git: ">=2.30"
    gh: "*"
    python3: ">=3.10"
```

Python tool keys match the command name: `python3` does not satisfy a `python`
requirement, or vice versa. Each reports its own detected version, including
Python 2 if that is what `python` runs. Use a version constraint to require Python 3.
Ciwi detects installed interpreters; it does not install Python or create aliases.
For Python inside a managed container, use:

```yaml
requires:
  container:
    tools:
      python3: ">=3.10"
```

Constraint syntax:
- presence: `*` or empty
- comparison: `>=`, `>`, `<=`, `<`, `=`, `==`

## Managed containers

Jobs with `runs_on.container_image` or `runs_on.container_build_context` execute
inside a managed Linux container. Ciwi selects an available Docker or Apple
Container runtime, prepares the image, mounts source and caches, probes tools,
executes steps, and removes the job container when execution ends. Container
commands use a non-login POSIX shell so the image's PATH and the step's HOME are
preserved.

```yaml
runs_on:
  executor: script
  shell: posix
  container_runtime: auto
  container_image: mcr.microsoft.com/playwright:v1.62.1-noble
  container_cpus: "4"
  container_memory: 4G
  container_shm_size: 1G
requires:
  tools:
    git: "*"
  container:
    tools:
      node: "*"
      npm: "*"
```

`container_runtime` accepts `auto` (the default), `docker`, or `apple`. Automatic
selection prefers Apple Container on Apple silicon Macs and Docker elsewhere.
The scheduler requires one available backend to satisfy all container settings.
It rechecks readiness before execution and does not switch backends after image
preparation starts. Runtime readiness refreshes every 30 seconds.

`runs_on.os` and `runs_on.arch` always constrain the **host**. Omit them for a job
that can run on either a Linux Docker agent or an Apple silicon Mac. Use
`container_platform: linux/amd64` or `linux/arm64` to select the **container**
platform; omission uses the runtime's native Linux platform. Docker currently
advertises its native Linux platform. Apple advertises arm64, plus amd64 when
Rosetta is installed. Image availability is checked during environment preparation.

Use `requires.tools` for host tools and `requires.container.tools` for tools inside
the image. A Docker tool requirement still restricts scheduling to hosts with
Docker installed; remove it when making a job portable.

### Images built from the repository

```yaml
runs_on:
  executor: script
  shell: posix
  container_build_context: packaging/linux
  container_build_file: Dockerfile
  container_platform: linux/amd64
  container_memory: 4G
```

The context is relative to the checkout; the Dockerfile is relative to that
context and defaults to `Dockerfile`. Paths must remain inside their respective
roots, including after resolving symlinks. `container_build_context` and
`container_image` are mutually exclusive. Ciwi builds a private execution image
using the selected runtime, then runs all job steps inside it. Engine build caches
are retained; the execution image tag is removed after use. Builder resource
limits come from engine configuration; job resource settings apply to the job
container itself.

Local images are reused only when the requested platform is available. Missing
images are pulled with at most three attempts and 10/20-second backoff. Pulls and
builds stream logs and consume the job timeout. Container startup has a separate
60-second timeout after image preparation.

Additional settings:

- `container_workdir`: workspace mount and working directory, default `/workspace`.
- `container_user`: user or `uid:gid`; default is the agent's UID/GID on POSIX hosts.
- `container_cpus`: positive integer.
- `container_memory`, `container_shm_size`: positive bytes or integer with K/M/G/T suffix.
- `container_devices`, `container_groups`: Docker-only device and supplementary-group settings. Automatic jobs using them select Docker; Apple-pinned jobs reject them.

Give tools a writable home, for example with step environment `HOME: /tmp`.
Browser jobs use `container_shm_size` instead of Docker's `--ipc=host`.
Job details report the selected runtime/version, platform, immutable image
identity, and Rosetta translation when used. Reports and artifacts remain paths
relative to the host checkout, while commands and their environment use container
paths.

### Installation and upgrade

Apple Container support targets CLI 1.2.2 or newer on macOS 26 with Apple silicon.
Install the CLI on the agent's PATH and start `container system start` as the same
user that runs the Ciwi agent. Install Rosetta for `linux/amd64` execution. Ciwi
checks service readiness but does not start services or install system components.
A missing CLI, stopped service, or inaccessible service is shown as an unavailable
runtime in scheduling diagnostics. Configure sufficient engine builder resources
for Dockerfile builds.

Upgrade agents before activating these jobs. Newly enqueued managed jobs require
the managed-execution capability, so older agents cannot lease them. Omitted
runtime settings now mean `auto`; use `container_runtime: docker` to pin existing
jobs. Jobs already queued by an older server retain their legacy Docker execution.
No database migration is needed.

Integration checks are excluded from ordinary Go unit runs by the `integration`
build tag. Run the targeted check for the environment you are testing:

```sh
# Apple Container on macOS arm64 (runtime service must already be running):
CIWI_TEST_CONTAINER_RUNTIME=apple go test -tags integration ./internal/agent -run '^TestContainerLiveCancellation$' -count=1 -timeout=5m -v
# Docker on Linux (runtime service must already be running):
CIWI_TEST_CONTAINER_RUNTIME=docker go test -tags integration ./internal/agent -run '^TestContainerLiveCancellation$' -count=1 -timeout=5m -v
# Debug-info verifier on macOS with Go and Xcode command-line tools:
go test -tags integration ./packaging -run '^TestAppleDebugInfoVerifierAcceptsRelWithDebInfoAndRejectsStrippedBinary$' -count=1 -timeout=5m -v
```

Explicit integration runs fail when the selected runtime or required tools are
unavailable. Container cancellation requires `CIWI_TEST_CONTAINER_RUNTIME` to be
`apple` or `docker`; automatic selection is not supported by this check.

The `build` pipeline runs separate host jobs, `container-cancellation-apple` and
`container-cancellation-docker`, and requires both to pass before cross-platform
compilation. A complete run needs a macOS arm64 agent with Apple Container 1.2.2+
and a Linux agent with Docker. Operators must start the runtime services; the jobs
do not install or start them. The `build-desktop/macos-unsigned` job verifies the
debug-info guard before building the application. Each check has its own Go test
report; coverage remains with the unit job.

## Work directory layout

`CIWI_AGENT_WORKDIR` contains:
- `workspaces/<project_id>_<project_name>_<pipeline_job_id>[_<matrix_name_or_idx-N>]_env-<fingerprint>`
- `cache/`

Environment fingerprint is derived from execution requirements (`os`, `arch`, `shell`, `executor`).
