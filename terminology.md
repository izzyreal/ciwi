# ciwi terminology

This document defines the canonical terms used in ciwi code, API payloads, and UI labels.

## Core model

- Project: top-level imported config scope that owns pipelines.
- Pipeline (definition): static configured workflow (`pipelines[]` in config).
- Pipeline run: one invocation of a pipeline, identified by `pipeline_run_id` metadata.
- Pipeline job (definition): a configured job inside a pipeline (`pipelines[].jobs[]`).
- Matrix entry (definition): one item from `pipeline job.matrix.include`.
- Step (definition): one configured command unit inside a pipeline job (`run` or `test`).
- Job execution (runtime): one materialized executable unit in the queue/history.

## Runtime mapping

- Running a pipeline creates one `JobExecution` per selected `(pipeline job, matrix entry)`.
- Running an ad-hoc script also creates a `JobExecution`, but without a stored pipeline job definition.
- Steps are currently not first-class runtime entities; they execute inside a single `JobExecution` script.
- `current_step` tracks step progress text for the active execution.

## API and payload terms

- `/api/v1/jobs` endpoints operate on job executions.
- Response keys use explicit runtime names:
  - `job_execution`
  - `job_executions`
  - `job_execution_id`
- Pipeline definition identity in execution metadata uses:
  - `pipeline_id`
  - `pipeline_job_id`
  - `pipeline_run_id`
  - `matrix_name` (when set)

## Status terms

- Job execution statuses are:
  - `queued`
  - `leased`
  - `running`
  - `succeeded`
  - `failed`

## Naming guidelines for code

- Use `JobExecution` in type names, function names, and store/server boundary methods for runtime behavior.
- Use `PipelineJob` for definition-layer behavior.
- Use plain `job` only for small local variables when file scope is already execution-only and unambiguous.
