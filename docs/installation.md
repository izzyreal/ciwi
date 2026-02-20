# Installation

ciwi provides automated installation/uninstall scripts for Linux, macOS, and Windows.

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
- Logs: `/var/log/ciwi/server.out.log`, `/var/log/ciwi/server.err.log`

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

Uninstall:

```bash
curl -fsSL -o /tmp/uninstall_ciwi_agent_macos.sh \
  https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_macos.sh && \
sh /tmp/uninstall_ciwi_agent_macos.sh
```

Manage lifecycle (user session):

```bash
launchctl disable gui/$(id -u)/nl.izmar.ciwi.agent
launchctl enable gui/$(id -u)/nl.izmar.ciwi.agent
launchctl kickstart -k gui/$(id -u)/nl.izmar.ciwi.agent
launchctl bootout gui/$(id -u)/nl.izmar.ciwi.agent
launchctl bootstrap gui/$(id -u) $HOME/Library/LaunchAgents/nl.izmar.ciwi.agent.plist
```

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

Uninstall:

```powershell
$script = Join-Path $env:TEMP ("uninstall_ciwi_agent_windows_" + [Guid]::NewGuid().ToString("N") + ".ps1")
$uri = "https://raw.githubusercontent.com/izzyreal/ciwi/main/uninstall_agent_windows.ps1?ts=$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
Invoke-WebRequest -Uri $uri -OutFile $script
powershell -NoProfile -ExecutionPolicy Bypass -File $script
```

## Notes

Installer scripts perform server identity checks via:
- `GET /healthz`
- `GET /api/v1/server-info`

For update behavior and self-update capability rules, see [`docs/operations.md`](operations.md).
