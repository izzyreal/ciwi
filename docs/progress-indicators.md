# Progress Indicators

Ciwi displays time-based progress on active job executions, pipeline sections, pipeline chains, and individual execution-step headers. The indicators are estimates based on previous successful executions, not completion percentages reported by build tools.

## Where progress appears

- The header on a job execution detail page.
- The header strip of each structured execution step, including ciwi-managed phases and YAML steps.
- Chain, pipeline, and job headers in **Queued and In Progress Job Executions**.

Individual rows inside an expanded execution group do not have a separate progress background. Their status and prerequisite reason provide the detailed state instead.

## Historical estimates

Ciwi gathers up to ten recent matching successful executions and uses their median duration. A median prevents one unusually fast or slow run from dominating the estimate.

Only samples with valid start and finish timestamps are used. Failed and cancelled executions are excluded because they often stop early and would make later estimates misleading.

### Job matching

Once a job has an agent, ciwi prefers history from that exact agent. If no same-agent history exists, ciwi falls back to compatible cross-agent history.

Before a job is leased, ciwi uses a provisional cross-agent estimate. Provisional samples must match the same:

- project, pipeline, and pipeline job
- matrix name and index
- required capabilities, including operating system and architecture
- ordinary or dry-run mode
- executable job and step plan

The provisional estimate may be replaced after leasing when exact history exists for the selected agent. A progress bar can therefore adjust when an agent is assigned.

### Step matching

Step estimates use successful `step.finished` events from matching job executions. The executable step definition must match; changing a command, environment, test configuration, or report configuration starts a new history set. A display-name-only change does not invalidate otherwise identical executable history.

Ciwi-managed phases use successful `phase.finished` events from the same matching executions. Their stable phase IDs keep history attached to the operation rather than its current display position. Depending on the job definition, the unified timeline can include:

- Prepare workspace
- Check out source
- Restore dependency artifacts
- Prepare execution environment
- the configured YAML steps
- Publish artifacts
- Publish test results

The visible `Step N/M` numbering covers this complete timeline. A phase without successful matching history remains indeterminate independently of the YAML steps.

## Visual states

### Determinate

When a duration estimate is available, the filled background represents elapsed time divided by expected duration.

- A queued but unstarted estimated job begins at zero.
- A running job advances according to server time and its recorded start time.
- A completed job contributes its full expected duration, or its actual duration when no estimate is available.

### Indeterminate

When active work has no usable estimate, a softly animated rectangle moves from side to side. A complete left-right-left cycle lasts four seconds.

Recreated UI elements share a wall-clock animation phase, so polling does not restart the movement. Unchanged card headers and expanded sections are retained between polls.

### Overrun

When elapsed time exceeds the estimate, the indicator remains full and pulses. It does not grow beyond the available width.

### Waiting

A dependency-blocked job displays `waiting` and identifies its prerequisite, for example `Waiting for job unit-tests` or `Waiting for pipeline build`.

- A waiting job does not animate by itself.
- An all-waiting group remains static.
- Estimated waiting jobs contribute their expected duration at zero completion to a group containing active work.
- If any remaining waiting or active job lacks an estimate, a mixed active group is indeterminate.

## Aggregate progress

Pipeline and chain progress is duration-weighted. Short jobs do not count as much as long jobs merely because each is one row.

Conceptually:

```text
aggregate progress = completed expected milliseconds / total expected milliseconds
```

Completed jobs use their full weight. Running jobs use their elapsed fraction. Estimated queued or waiting jobs contribute weight but no completed milliseconds yet.

## Component responsibilities

- The agent records actual timestamps and emits structured lifecycle events for ciwi-managed phases and YAML steps.
- The server stores those measurements and calculates historical job, phase, and YAML-step estimates.
- The browser converts estimates and current execution state into determinate, indeterminate, waiting, complete, or overrun visuals.

Progress calculation never parses human-readable log output.

## Why an estimate may be unavailable

An indicator remains indeterminate when ciwi has no successful matching history. Common reasons include:

- this is the first execution of the job or step
- previous executions failed or were cancelled
- the command or executable plan changed
- required capabilities or matrix values changed
- previous records do not contain valid duration timestamps or the relevant structured lifecycle events

As matching successful executions accumulate, later runs automatically gain estimates.
