# ciwi

![ciwi logo](internal/server/assets/ciwi-logo.png)

Simple, portable, single-binary CI/CD server + agent.

WIP.
NOT SUITABLE FOR PUBLIC SERVERS.
ONLY FOR PRIVATE NETWORKS AND HOMELAB STYLE PROJECTS.

## Background

ciwi started as a practical replacement for Jenkins/TeamCity for private projects.

## Quick Start

1. Install server/agent with scripts in [`docs/installation.md`](docs/installation.md).
2. Open UI at `http://127.0.0.1:8112/`.
3. Import a project that contains `ciwi-project.yaml`.
4. Run pipeline and inspect jobs.

Detailed guide: [`docs/getting-started.md`](docs/getting-started.md).

Note on agent workdir:
- `CIWI_AGENT_WORKDIR` is optional. If unset, the agent defaults to `.ciwi-agent/work` relative to its working directory.
- Installer-based deployments set an absolute workdir (Linux: `/var/lib/ciwi-agent/work`, macOS: `$HOME/.ciwi-agent/work`, Windows: `%ProgramData%\\ciwi-agent\\work`).
- The agent normalizes the workdir to an absolute path at runtime.

## Documentation Map

- Getting started: [`docs/getting-started.md`](docs/getting-started.md)
- Installation scripts: [`docs/installation.md`](docs/installation.md)
- Env vars, prerequisites, tool requirements: [`docs/configuration.md`](docs/configuration.md)
- Pipeline config and runtime model: [`docs/pipelines.md`](docs/pipelines.md)
- Backend API reference (grouped by consumer): [`docs/api.md`](docs/api.md)
- Vault/AppRole integration: [`docs/vault.md`](docs/vault.md)
- Operations (update policy, maintenance, troubleshooting): [`docs/operations.md`](docs/operations.md)
- Architecture and flows: [`docs/architecture.md`](docs/architecture.md)
- Domain terminology: [`terminology.md`](terminology.md)

## Design Philosophy

ciwi intentionally avoids fragile behavior that depends on parsing human-readable logs.

- ciwi is designed around explicit API contracts and structured payloads between server, agent, and UI.
- Features should use dedicated fields/endpoints instead of scraping job output text.
- Job output remains for humans; machine behavior should rely on typed data.

## Examples

Example `ciwi-project.yaml` files:
- [`ciwi-project.yaml`](https://github.com/izzyreal/cupuacu/blob/main/ciwi-project.yaml) for building/publishing cupuacu.
- [`ciwi-project.yaml`](ciwi-project.yaml) for building ciwi itself.

## Screenshots

<img width="1091" height="702" alt="image" src="https://github.com/user-attachments/assets/f6ae903c-40a0-47f5-b961-8ec0611f5e3c" />
<img width="1101" height="712" alt="image" src="https://github.com/user-attachments/assets/35a88fd3-b612-4781-b306-ff476df75f31" />
<img width="1089" height="715" alt="image" src="https://github.com/user-attachments/assets/f0c9f1ab-5a5b-44b9-9207-c7fb295b09a0" />
<img width="1095" height="699" alt="image" src="https://github.com/user-attachments/assets/1515fc10-466f-478e-bc3a-b2669e612c90" />
<img width="1096" height="710" alt="image" src="https://github.com/user-attachments/assets/c4d6ccda-bc09-4645-8372-e80fd02290f4" />
