# Pipelines and ciwi-project.yaml

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
- `Run` / `Dry Run` without modifiers: enqueue immediately with default source resolution.
- `Shift+Run` / `Shift+Dry Run`: opens the run modal first.

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

`pipeline_chains` execution is DAG-based:
- Jobs in a pipeline are enqueued together.
- A chain pipeline stays blocked until all listed in-chain `depends_on` pipelines finish successfully.
- On upstream failure, only blocked downstream pipelines that depend on that failed pipeline are cancelled.
- If no in-chain `depends_on` is declared, ciwi falls back to linear order (depends on previous chain item).

## Versioning

Optional `pipelines[].versioning`:
- `file` (default `VERSION`)
- `tag_prefix` (default `v`)
- `auto_bump`: `patch|minor|major`

Injected env vars:
- `CIWI_PIPELINE_VERSION_RAW`
- `CIWI_PIPELINE_VERSION` / `CIWI_PIPELINE_TAG`
- `CIWI_PIPELINE_TAG_PREFIX`
- `CIWI_PIPELINE_SOURCE_REF`
- `CIWI_PIPELINE_SOURCE_REPO`
- `CIWI_PIPELINE_VERSION_FILE`

## Job requirements and runtime

`runs_on` fields:
- `os`, `arch`, `executor`, `shell`
- optional `container_image` for managed container execution

`executor`:
- currently `script`

`shell`:
- `posix`, `cmd`, `powershell`

## Steps

Supported step kinds:
- `run`
- `test` with parsed test reports and optional coverage reports

`test` supports:
- `format`: `go-test-json`, `junit-xml`
- `coverage_format`: `go-coverprofile`, `lcov`

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

## Job history actions behavior

- **Run Again** creates a new job execution from existing definition.
- If stored source ref is commit SHA, rerun uses same commit.
- If stored source ref is branch/tag, rerun may resolve newer commit.
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

## Cache notes

- Caches are directory caches keyed by `caches[].id`.
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
- `go-build` -> `GOCACHE`
- `go-mod` -> `GOMODCACHE`
- You can disable it explicitly with `go_cache: { enabled: false }`.
