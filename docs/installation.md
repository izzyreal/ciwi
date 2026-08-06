# Installation

ciwi provides automated installation/uninstall scripts for Linux, macOS, and Windows.

The scripts install the server and execution agents. Desktop-client packages
are published with each GitHub release:

- `Ciwi-Client-macos-v<version>.dmg`: signed, notarized universal macOS app.
- `Ciwi-Client-windows-Setup-x86_64-v<version>.exe`: Windows x64 installer.
- `Ciwi-Client-linux-amd64-v<version>.zip`: Linux amd64 app and desktop files.

The iPhone/iPad client is distributed through TestFlight rather than the
GitHub release assets. See [`native-client.md`](native-client.md) for native
connection and platform details.

## Linux server (systemd)

Install:

```bash
curl -fsSL -o /tmp/install_ciwi_server_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_server_linux.sh && \
sh /tmp/install_ciwi_server_linux.sh
```

Uninstall:

```bash
curl -fsSL -o /tmp/uninstall_ciwi_server_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_server_linux.sh && \
sh /tmp/uninstall_ciwi_server_linux.sh
```

Default paths:
- Binary: `/usr/local/bin/ciwi`
- Env file: `/etc/default/ciwi`
- DB: `/var/lib/ciwi/ciwi.db`
- Artifacts: `/var/lib/ciwi/artifacts`
- Update staging: `/var/lib/ciwi/updates`
- Logs: `/var/log/ciwi/server.log`

## Linux agent (systemd)

Install with token (recommended):

```bash
export CIWI_GITHUB_TOKEN="<your-token>"
curl -fsSL -o /tmp/install_ciwi_agent_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_linux.sh && \
sh /tmp/install_ciwi_agent_linux.sh
```

Install without token:

```bash
curl -fsSL -o /tmp/install_ciwi_agent_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_linux.sh && \
sh /tmp/install_ciwi_agent_linux.sh
```

Uninstall:

```bash
curl -fsSL -o /tmp/uninstall_ciwi_agent_linux.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_linux.sh && \
sh /tmp/uninstall_ciwi_agent_linux.sh
```

If jobs need Docker/audio:

```bash
sudo usermod -aG docker ciwi-agent
sudo usermod -aG audio ciwi-agent
sudo systemctl restart ciwi-agent
id ciwi-agent; getent group docker; getent group audio
```

Default paths:
- Binary: `/usr/local/bin/ciwi`
- Env file: `/etc/default/ciwi-agent`
- Data dir: `/var/lib/ciwi-agent`
- Workdir: `/var/lib/ciwi-agent/work`
- Logs: `/var/log/ciwi-agent/agent.log`

## macOS agent (LaunchAgent)

Install with token (recommended):

```bash
export CIWI_GITHUB_TOKEN="<your-token>"
curl -fsSL -o /tmp/install_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_macos.sh && \
sh /tmp/install_ciwi_agent_macos.sh
```

Install without token:

```bash
curl -fsSL -o /tmp/install_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_macos.sh && \
sh /tmp/install_ciwi_agent_macos.sh
```

Update token:

```bash
export CIWI_GITHUB_TOKEN="<new-token>"
curl -fsSL -o /tmp/update_ciwi_agent_macos_token.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/update_agent_macos_token.sh && \
sh /tmp/update_ciwi_agent_macos_token.sh
```

Uninstall:

```bash
curl -fsSL -o /tmp/uninstall_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_macos.sh && \
sh /tmp/uninstall_ciwi_agent_macos.sh
```

Manage lifecycle with the bundled service helper:

```bash
CIWI_SERVICE="$HOME/Library/Application Support/ciwi/CiwiAgent.app/Contents/MacOS/ciwi-service"
"$CIWI_SERVICE" status-agent
"$CIWI_SERVICE" unregister-agent
"$CIWI_SERVICE" register-agent
```

Default paths:
- App bundle: `$HOME/Library/Application Support/ciwi/CiwiAgent.app`
- Env file: `$HOME/Library/Application Support/ciwi/agent.env`
- Workdir: `$HOME/.ciwi-agent/work`
- Log file: `$HOME/Library/Logs/ciwi/agent.log`

## Windows agent (Service)

Run in elevated PowerShell.

Install with token (recommended):

```powershell
$env:CIWI_GITHUB_TOKEN = "<your-token>"
$script = Join-Path $env:TEMP ("install_ciwi_agent_windows_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_windows.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

Install without token:

```powershell
$script = Join-Path $env:TEMP ("install_ciwi_agent_windows_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/install_agent_windows.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

Update token:

```powershell
$env:CIWI_GITHUB_TOKEN = "<new-token>"
$script = Join-Path $env:TEMP ("update_ciwi_agent_windows_token_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/update_agent_windows_token.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

Uninstall:

```powershell
$script = Join-Path $env:TEMP ("uninstall_ciwi_agent_windows_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_windows.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

Default paths:
- Binary: `%ProgramFiles%\ciwi\ciwi.exe`
- Env file: `%ProgramData%\ciwi-agent\agent.env`
- Data dir: `%ProgramData%\ciwi-agent`
- Workdir: `%ProgramData%\ciwi-agent\work`
- Logs dir: `%ProgramData%\ciwi-agent\logs`

## Notes

Installer scripts perform server identity checks via:

- `GET /healthz`
- `GET /api/v1/server-info`

For update behavior and self-update capability rules, see [`docs/operations.md`](operations.md).
For a fuller path inventory across server and agent platforms, see [`docs/files.md`](files.md).
