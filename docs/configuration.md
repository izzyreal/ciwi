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
- `CIWI_AGENT_LOG_FILE`: macOS agent log override (default
  `$HOME/Library/Logs/ciwi/agent.log`)
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
```

Constraint syntax:
- presence: `*` or empty
- comparison: `>=`, `>`, `<=`, `<`, `=`, `==`

## Container runtime probe

When `runs_on.container_image` is set:
- agent starts/manages runtime container
- steps execute through `docker exec`
- source and cache paths are bind-mounted
- tool probes run in container and are persisted as structured runtime capabilities

Optional:
- `runs_on.container_devices`
- `runs_on.container_groups`
- `runs_on.container_workdir`
- `runs_on.container_user`

Container-only requirements can be declared separately from host requirements:

```yaml
requires:
  tools:
    docker: "*"
  container:
    tools:
      cmake: ">=3.20"
```

## Work directory layout

`CIWI_AGENT_WORKDIR` contains:
- `workspaces/<project_id>_<project_name>_<pipeline_job_id>[_<matrix_name_or_idx-N>]_env-<fingerprint>`
- `cache/`

Environment fingerprint is derived from execution requirements (`os`, `arch`, `shell`, `executor`).
