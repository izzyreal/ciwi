# Architecture

ciwi is a single codebase that runs in three modes:
- server
- agent
- all-in-one

It is designed for private-network CI/CD with explicit, structured contracts between components.

## High-level architecture

```mermaid
flowchart LR
  subgraph UI[Browser UI]
    FE[Frontend]
  end

  subgraph ServerHost[ciwi Server]
    API[HTTP API + UI handlers]
    SCHED[Queue + lease coordination]
    UPDATE[Server update controller]
    DB[(SQLite)]
    ART[(Artifacts dir)]
  end

  subgraph AgentHostA[Agent host A]
    AGENTA[Agent runtime]
    TOOLS_A[Tool/runtime probe]
    EXEC_A[Shell or managed container execution]
  end

  subgraph AgentHostB[Agent host B]
    AGENTB[Agent runtime]
  end

  GIT[(Git remotes)]
  GH[(GitHub releases API/assets)]
  VAULT[(Vault AppRole)]

  FE -->|REST/SSE| API
  API <--> DB
  API <--> ART
  API <--> SCHED
  API <--> UPDATE

  AGENTA <--> |heartbeat/lease/status/artifacts/tests| API
  AGENTB <--> |heartbeat/lease/status/artifacts/tests| API
  AGENTA --> TOOLS_A
  AGENTA --> EXEC_A

  API --> GIT
  AGENTA --> GIT
  UPDATE --> GH
  AGENTA --> GH
  API --> VAULT
```

## Job lifecycle

```mermaid
sequenceDiagram
  autonumber
  participant U as User/UI
  participant S as Server
  participant DB as SQLite
  participant A as Agent
  participant FS as Artifact Storage

  U->>S: enqueue pipeline/job
  S->>DB: persist queued job execution

  loop heartbeat + lease cycle
    A->>S: POST /heartbeat
    A->>S: POST /agent/lease
    S->>DB: select compatible queued job
    S-->>A: leased job payload
  end

  A->>A: checkout source + run steps
  A->>S: POST /jobs/{id}/status (running updates)
  S->>DB: persist status/output/runtime data

  A->>FS: write artifacts locally
  A->>S: POST /jobs/{id}/artifacts
  S->>DB: persist artifact metadata

  A->>S: POST /jobs/{id}/tests
  S->>DB: persist test/coverage report

  A->>S: POST /jobs/{id}/status (terminal)
  S->>DB: persist final state
  U->>S: GET /jobs and job details
```

## Update architecture

```mermaid
sequenceDiagram
  autonumber
  participant UI as Global Settings UI
  participant S as Server
  participant GH as GitHub Releases
  participant SYS as Service Manager
  participant A as Agent

  UI->>S: POST /api/v1/update/check
  S->>GH: query latest/tag info
  S-->>UI: current/latest/update_available

  UI->>S: POST /api/v1/update/apply
  S->>GH: download asset (+ checksum when required)

  alt Linux staged updater path
    S->>S: stage binary + manifest
    S->>SYS: trigger updater unit
  else helper path
    S->>S: start update helper process
  end

  S->>A: set pending agent target version
  loop drain-queue policy
    A->>S: heartbeat/lease
    Note over A: finish running + queued jobs first
  end
  A->>A: self-update and restart
```

## Data model (conceptual)

Primary persisted entities:
- projects
- pipelines and pipeline jobs
- pipeline chains
- job executions
- job artifacts
- test/coverage reports
- app state key-values (including update status)
- vault connections + per-project vault settings

## Design principles

- Structured APIs over log scraping.
- Deterministic server-side state transitions for jobs.
- Agent capability and runtime requirement matching before execution.
- Explicit update orchestration with persisted status.

## Trust boundaries and assumptions

- Intended for private networks/homelab-style deployments.
- No claim of hard multi-tenant isolation/security hardening.
- Credentials/secrets expected to be managed through Vault mappings or host environment discipline.
