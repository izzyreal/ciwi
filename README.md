# ciwi
Simple CI/CD and build automation system

## Getting started

```bash
go run ./cmd/ciwi --help
go run ./cmd/ciwi server
go run ./cmd/ciwi agent
go run ./cmd/ciwi all-in-one
```

## Environment variables

- `CIWI_SERVER_ADDR`: server bind address (default `:8080`)
- `CIWI_SERVER_URL`: agent target base URL (default `http://127.0.0.1:8080`)
- `CIWI_AGENT_ID`: override agent ID (default `agent-<hostname>`)

## First functional API slice

- `GET /healthz` returns `{"status":"ok"}`
- `POST /api/v1/heartbeat` accepts agent heartbeats in JSON
