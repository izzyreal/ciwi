# Pipelines and ciwi-project.yaml

The root document uses `version: 1`, a required `project.name`, one or more
`pipelines`, and an optional `pipeline_chains` list. YAML decoding is strict:
unknown fields and invalid references are rejected during import or validation.

## Source and execution model

Pipeline-level VCS source (optional):
- `pipelines[].vcs_source.repo`
- `pipelines[].vcs_source.ref`

If `vcs_source` is omitted, ciwi runs the pipeline as artifact/script-only and skips VCS checkout.

Agent checkout behavior:
- clone default branch
- `git fetch origin <ref>`
- `git checkout --force FETCH_HEAD`

UI run controls:

- Browser **Run** / **Dry Run** without modifiers: enqueue immediately with default source resolution.
- Browser `Shift+Run` / `Shift+Dry Run`: opens the run modal first.
- Native **Options**: opens the shared run-options screen before enqueueing.

Run modal fields include:
- source branch selector (from `.../source-refs`)
- eligible agent selector (from `.../eligible-agents`), default is `Any eligible agent` to keep normal lease matching.

Dry-run preview controls:
- `Preview Dry Run` is available on pipeline and chain rows (index and project pages).

Preview modal can:
- render planned pending jobs and required capabilities without enqueueing
- optionally filter to `offline_cached_only`
- enqueue an offline execution directly via `Execute Offline` (uses `execution_mode=offline_cached`)

Git identity behavior for source-backed jobs:
- After checkout, ciwi configures repository-local git identity:
- `user.name=ciwi-agent`
- `user.email=ciwi-agent@local`
- This applies to git commit/tag/push operations in job steps unless overridden later by step scripts.
- Artifact-only jobs (no `vcs_source`) have no checked-out repository, so this repo-local identity setup does not run.

## Dependency chains

- `pipelines[].depends_on`: upstream pipeline IDs
- dependent runs inherit resolved version/source metadata for consistency
- `depends_on` gates execution and version/source inheritance; it does not restore artifacts by itself

Jobs restore dependency artifacts only from explicit `artifact_sources`:

```yaml
jobs:
  - id: codesign
    artifact_sources:
      - pipeline: build
        job: compile
        matrix:
          name: darwin-arm64
```

- `pipeline` must be a direct pipeline-level `depends_on`.
- `job` must identify an artifact-producing job in that pipeline.
- Optional `matrix` entries are literal partial matches against producer matrix variables.
- Omitting `matrix` restores artifacts from every execution of that producer job.
- Every declared source is required; a missing execution or an execution with no published artifacts fails the consumer job.
- During a dry run, a successful source that published no artifacts because its producing steps were skipped is logged and ignored; consumer validation steps still fail normally if they require the absent files.
- When selected sources publish the same path, later sources overwrite earlier ones and ciwi logs the collision.
- The server resolves selectors to concrete producer execution IDs and sends them to the agent as typed job data; these internal IDs are not injected into the job's environment.

`pipeline_chains` execution is DAG-based:
- A chain is defined by its ordered `pipelines` list; ciwi derives its stable runtime ID from that sequence.
- Optional `name` provides a concise UI label. Without it, ciwi displays the pipeline IDs joined with arrows.
- Jobs in a pipeline are enqueued together.
- A chain pipeline stays blocked until all listed in-chain `depends_on` pipelines finish successfully.
- On upstream failure, only blocked downstream pipelines that depend on that failed pipeline are cancelled.
- If no in-chain `depends_on` is declared, ciwi falls back to linear order (depends on previous chain item).

## Job dependency graphs

- `pipelines[].jobs[].needs` lists job IDs in the same pipeline.
- The graph must be acyclic; unknown, duplicate, and self-references are rejected.
- A dependent job remains queued/waiting until every required job execution in
  that pipeline run succeeds.
- Matrix jobs materialize one execution per `matrix.include` entry. A dependent
  job waits for all selected executions of each needed job.
- Failure cancels only downstream work that can no longer run; independent
  branches remain eligible.

## Versioning

Optional `pipelines[].versioning`:
- `file` (default `VERSION`)
- `tag_prefix` (default `v`)
- `auto_bump`: `patch|minor|major`
- `auto_bump_vcs_token`: required when `auto_bump` is set; accepts a literal
  token or a `{{ secret.<name> }}` placeholder declared by `auto_bump_vault`
- `auto_bump_vault`: Vault connection and secret mappings for the auto-bump token

Auto-bump appends a final step to a non-dry pipeline run, commits the next
semantic version, and pushes it to the source branch. It requires the selection
to materialize exactly one job execution and fails safely if the branch or
version file advanced after resolution. Dry runs do not append the auto-bump
step.

Injected env vars:
- `CIWI_PIPELINE_VERSION_RAW`
- `CIWI_PIPELINE_VERSION` / `CIWI_PIPELINE_TAG`
- `CIWI_PIPELINE_TAG_PREFIX`
- `CIWI_PIPELINE_SOURCE_REF`
- `CIWI_PIPELINE_SOURCE_REF_RAW`
- `CIWI_PIPELINE_SOURCE_REPO`
- `CIWI_PIPELINE_VERSION_FILE`
- `CIWI_DRY_RUN` (`1` for dry-run executions)

Matrix and version values can also be used in YAML templates, including
`{{ciwi.version_raw}}`, `{{ciwi.version}}`, and `{{ciwi.tag_prefix}}`.

## Job requirements and runtime

`runs_on` fields:
- `os`, `arch`, `executor`, `shell`
- optional `container_image` for managed container execution
- optional `container_workdir`, `container_user`, `container_devices`, and
  `container_groups`

`executor`:
- currently `script`

`shell`:
- `posix`, `cmd`, `powershell`

## Steps

Supported step kinds:
- `run`
- `test` with parsed test reports and optional coverage reports

`test` supports:
- `format`: `go-test-json`, `junit`, or `junit-xml`
- `coverage_format`: `go-coverprofile`, `lcov`

Each test step requires a relative `report` path. A `coverage_report` path is
required when `coverage_format` is set. Every step can set `name`, `env`, and
`skip_dry_run`; a skipped dry-run step remains visible in the structured
timeline but its command is not executed.

Step-level env is supported via `steps[].env`.

## Secrets in YAML

Secret placeholder form:
- `{{ secret.<name> }}`

Secrets are declared per step:

```yaml
steps:
  - run: echo release
    vault:
      connection: home-vault
      secrets:
        - name: github-secret
          mount: kv
          path: gh
          key: token
    env:
      GITHUB_TOKEN: "{{ secret.github-secret }}"
```

Resolved just-in-time when agent leases a job.

The same mapping shape can be used at
`pipelines[].versioning.auto_bump_vault`; in that location placeholders are
allowed only in `auto_bump_vcs_token`. See [`vault.md`](vault.md).

## Job history actions behavior

- **Run Again** creates a new job execution from existing definition.
- Rerun uses the same pinned commit as the original queued job.
- Existing artifacts/logs remain tied to old execution ID.

## Project import identity and naming

Project identity for import/reload is:
- `repo_url`
- `repo_ref`
- `config_file`

Behavior:
- Import with the same identity updates/reloads the existing project.
- Import with different identity does not replace an existing project, even if `project.name` inside YAML matches.
- Project name is kept as declared in YAML; branch/ref disambiguation is shown in UI via the `branch:<ref>` badge.

Definitions entered through Global Settings use the separate **Managed YAML** source type. Ciwi assigns these projects an internal identity, stores the YAML in SQLite, and uses `project.name` as the editable display name. Managed-project names must be unique among managed projects, ignoring case.

## Cache notes

- Caches are directory caches keyed by `caches[].id`.
- Each ordinary cache also requires `caches[].env`, the environment variable
  that receives its managed directory.
- Recommended FetchContent approach is source-only caching; keep build output job-local.
- Go projects can enable managed Go caches per job:

```yaml
jobs:
  - id: unit-tests
    go_cache: {}
    steps:
      - run: go test ./...
```

- `go_cache: {}` adds two managed caches under ciwi's cache root:
  - `go-build` → `GOCACHE`
  - `go-mod` → `GOMODCACHE`
- You can disable it explicitly with `go_cache: { enabled: false }`.
