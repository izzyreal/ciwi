# Pipelines and ciwi-project.yaml

## Source and execution model

Pipeline-level source:
- `pipelines[].source.repo` (required)
- `pipelines[].source.ref` (optional)

Agent checkout behavior:
- clone default branch
- `git fetch origin <ref>`
- `git checkout --force FETCH_HEAD`

## Dependency chains

- `pipelines[].depends_on`: upstream pipeline IDs
- dependent runs inherit resolved version/source metadata for consistency

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

Resolved just-in-time when agent leases a job.

## Job history actions behavior

- **Run Again** creates a new job execution from existing definition.
- If stored source ref is commit SHA, rerun uses same commit.
- If stored source ref is branch/tag, rerun may resolve newer commit.
- Existing artifacts/logs remain tied to old execution ID.

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
