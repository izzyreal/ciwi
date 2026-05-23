# Files and Directories

This page summarizes the main files and directories used by ciwi installers and default runtime configuration.

## Linux server (systemd)

Installer: `install_server_linux.sh`

Default paths:
- Binary: `/usr/local/bin/ciwi`
- Service unit: `/etc/systemd/system/ciwi.service`
- Staged updater unit: `/etc/systemd/system/ciwi-updater.service`
- Env file: `/etc/default/ciwi`
- Data dir: `/var/lib/ciwi`
- SQLite DB: `/var/lib/ciwi/ciwi.db`
- Artifacts dir: `/var/lib/ciwi/artifacts`
- Update staging dir: `/var/lib/ciwi/updates`
- Log dir: `/var/log/ciwi`
- Log file: `/var/log/ciwi/server.log`
- Logrotate policy: `/etc/logrotate.d/ciwi`
- Polkit rule for updater: `/etc/polkit-1/rules.d/90-ciwi-updater.rules`

Notes:
- The server service runs with working directory `/var/lib/ciwi`.
- The env file sets `CIWI_DB_PATH`, `CIWI_ARTIFACTS_DIR`, and update staging variables explicitly, so those installer paths are the effective defaults for installed Linux servers.

## Linux agent (systemd)

Installer: `install_agent_linux.sh`

Default paths:
- Binary: `/usr/local/bin/ciwi`
- Service unit: `/etc/systemd/system/ciwi-agent.service`
- Env file: `/etc/default/ciwi-agent`
- Data dir: `/var/lib/ciwi-agent`
- Workdir: `/var/lib/ciwi-agent/work`
- Log dir: `/var/log/ciwi-agent`
- Log file: `/var/log/ciwi-agent/agent.log`
- Logrotate policy: `/etc/logrotate.d/ciwi-agent`

Notes:
- The service runs as user `ciwi-agent`.
- The service working directory is `/var/lib/ciwi-agent`.
- `CIWI_AGENT_WORKDIR` is set by the installer to `/var/lib/ciwi-agent/work`.

## macOS agent (LaunchAgent)

Installer: `install_agent_macos.sh`

Default paths:
- App bundle: `$HOME/Library/Application Support/ciwi/CiwiAgent.app`
- LaunchAgent plist label: `nl.izmar.ciwi.agent`
- Env file: `$HOME/Library/Application Support/ciwi/agent.env`
- App support dir: `$HOME/Library/Application Support/ciwi`
- Workdir: `$HOME/.ciwi-agent/work`
- Update staging dir: `$HOME/.ciwi-agent/work/updates`
- Update manifest: `$HOME/.ciwi-agent/work/updates/pending.json`
- Log dir: `$HOME/Library/Logs/ciwi`
- Log file: `$HOME/Library/Logs/ciwi/agent.log`
- `newsyslog` policy: `/etc/newsyslog.d/ciwi-<username>.conf`

Notes:
- The installer stores `CIWI_AGENT_LOG_FILE` in `agent.env`, pointing at `agent.log`.
- The bundled LaunchAgent plist lives inside the app bundle at `CiwiAgent.app/Contents/Library/LaunchAgents/nl.izmar.ciwi.agent.plist`.

## Windows agent (Service)

Installer: `install_agent_windows.ps1`

Default paths:
- Binary: `%ProgramFiles%\ciwi\ciwi.exe`
- Data dir: `%ProgramData%\ciwi-agent`
- Env file: `%ProgramData%\ciwi-agent\agent.env`
- Workdir: `%ProgramData%\ciwi-agent\work`
- Logs dir: `%ProgramData%\ciwi-agent\logs`
- Service name: `ciwi-agent`

Notes:
- The Windows installer creates a logs directory, but this repo does not currently document a single installer-defined agent log filename there.
- If `CIWI_AGENT_ENV_FILE` is unset at runtime, the agent defaults to `%ProgramData%\ciwi-agent\agent.env`.

## Non-installer defaults

If you run ciwi manually instead of through an installer/service manager, some paths become relative to the current working directory unless overridden by environment variables.

Examples:
- Server DB default: `ciwi.db`
- Server artifacts default: `ciwi-artifacts`
- Agent workdir default: `.ciwi-agent/work`

For environment-variable overrides, see [`configuration.md`](configuration.md).
