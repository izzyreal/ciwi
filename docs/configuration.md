# Configuration

## Environment variables

Common variables:
- `CIWI_SERVER_ADDR`: server bind address (default `:8112`)
- `CIWI_DB_PATH`: sqlite path (default `ciwi.db`)
- `CIWI_ARTIFACTS_DIR`: artifact root (default `ciwi-artifacts`)
- `CIWI_SERVER_URL`: agent target URL (default `http://127.0.0.1:8112`)
- `CIWI_AGENT_ID`: override agent ID
- `CIWI_AGENT_WORKDIR`: agent work dir (default `.ciwi-agent/work`)
- `CIWI_AGENT_ENV_FILE`: Windows env file (default `%ProgramData%\\ciwi-agent\\agent.env`)
- `CIWI_AGENT_TRACE_SHELL`: shell tracing (default `true`)
- `CIWI_AGENT_GO_BUILD_VERBOSE`: sets `GOFLAGS=-v` when unset (default `true`)
- `CIWI_WINDOWS_SERVICE_NAME`: Windows service name (default `ciwi-agent`)
- `CIWI_UPDATE_REPO`: update repo (default `izzyreal/ciwi`)
- `CIWI_UPDATE_API_BASE`: update API base (default `https://api.github.com`)
- `CIWI_LOG_LEVEL`: `debug|info|warn|error` (default `info`)

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
- `git`, `go`, `gh`, `cmake`, `ninja`, `docker`, `gcc`, `clang`
- `ccache`, `sccache`
- macOS signing/packaging tools
- Windows `msvc`, `iscc`, `signtool`
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

## Work directory layout

`CIWI_AGENT_WORKDIR` contains:
- `workspaces/<project_id>_<project_name>_<pipeline_job_id>[_<matrix_name_or_idx-N>]_env-<fingerprint>`
- `cache/`

Environment fingerprint is derived from execution requirements (`os`, `arch`, `shell`, `executor`).
