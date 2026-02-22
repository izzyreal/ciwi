$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($args.Count -ne 0) {
  Write-Error 'this installer takes no options; run it directly'
  exit 2
}

function Require-Command {
  param([Parameter(Mandatory = $true)][string]$Name)
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "missing required command: $Name"
  }
}

function Ensure-Admin {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'run this installer from an elevated PowerShell session (Run as Administrator)'
  }
}

function Trim-OneLine {
  param([string]$Value)
  if ($null -eq $Value) { return '' }
  return ($Value -replace '[\r\n]', '').Trim()
}

function Is-IPv4 {
  param([string]$Value)
  return ($Value -match '^\d{1,3}(\.\d{1,3}){3}$')
}

function Canonicalize-Url {
  param([Parameter(Mandatory = $true)][string]$Url)
  $raw = Trim-OneLine $Url
  if ([string]::IsNullOrWhiteSpace($raw)) {
    throw 'server URL is required'
  }
  if ($raw -notmatch '^[a-zA-Z][a-zA-Z0-9+\-.]*://') {
    $raw = "http://$raw"
  }
  $uri = $null
  if (-not [Uri]::TryCreate($raw, [UriKind]::Absolute, [ref]$uri)) {
    throw "invalid server URL: $Url"
  }
  if ($uri.Scheme -ne 'http') {
    throw "unsupported server URL scheme '$($uri.Scheme)'; expected http://"
  }
  $serverHost = Trim-OneLine $uri.Host
  if ([string]::IsNullOrWhiteSpace($serverHost)) {
    throw "invalid server URL host in '$Url'"
  }
  $serverHost = $serverHost.TrimEnd('.').ToLowerInvariant()
  $port = $uri.Port
  if ($port -le 0) {
    $port = 8112
  }
  return "http://${serverHost}:$port"
}

function Get-ServerInfoJson {
  param([Parameter(Mandatory = $true)][string]$BaseUrl)
  try {
    return Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/server-info" -TimeoutSec 2
  } catch {
    return $null
  }
}

function Test-CiwiServer {
  param([Parameter(Mandatory = $true)][string]$BaseUrl)
  try {
    $health = Invoke-RestMethod -Method Get -Uri "$BaseUrl/healthz" -TimeoutSec 2
    if ($null -eq $health) { return $false }
    if ((Trim-OneLine ([string]$health.status)) -ne 'ok') { return $false }

    $info = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/server-info" -TimeoutSec 2
    if ($null -eq $info) { return $false }
    if ((Trim-OneLine ([string]$info.name)) -ne 'ciwi') { return $false }
    if ((Trim-OneLine ([string]$info.api_version)) -ne '1') { return $false }
    return $true
  } catch {
    return $false
  }
}

function Resolve-HostnameForIp {
  param([Parameter(Mandatory = $true)][string]$Ip)
  if (-not (Is-IPv4 $Ip)) {
    return ''
  }
  try {
    $entry = [System.Net.Dns]::GetHostEntry($Ip)
    if ($null -ne $entry -and -not [string]::IsNullOrWhiteSpace($entry.HostName)) {
      return (Trim-OneLine $entry.HostName).TrimEnd('.').ToLowerInvariant()
    }
  } catch {
  }
  return ''
}

function Get-DnsSuffixCandidates {
  $seen = @{}
  $values = New-Object System.Collections.Generic.List[string]

  $addSuffix = {
    param([string]$Suffix)
    $v = Trim-OneLine $Suffix
    if ([string]::IsNullOrWhiteSpace($v)) { return }
    $v = $v.TrimEnd('.').ToLowerInvariant()
    while ($v.StartsWith('.')) {
      $v = $v.Substring(1)
    }
    if ([string]::IsNullOrWhiteSpace($v)) { return }
    if (-not $seen.ContainsKey($v)) {
      $seen[$v] = $true
      $values.Add($v) | Out-Null
    }
  }

  & $addSuffix 'local'
  & $addSuffix $env:USERDNSDOMAIN

  try {
    $globalDns = Get-DnsClientGlobalSetting -ErrorAction Stop
    foreach ($suffix in @($globalDns.SuffixSearchList)) {
      & $addSuffix ([string]$suffix)
    }
  } catch {
  }

  try {
    foreach ($client in @(Get-DnsClient -ErrorAction Stop)) {
      & $addSuffix ([string]$client.ConnectionSpecificSuffix)
    }
  } catch {
  }

  return @($values)
}

function Build-HostUrlCandidates {
  param(
    [Parameter(Mandatory = $true)][string]$HostName,
    [Parameter(Mandatory = $true)][int]$Port
  )

  $normalizedHost = Trim-OneLine $HostName
  $normalizedHost = $normalizedHost.TrimEnd('.').ToLowerInvariant()
  if ([string]::IsNullOrWhiteSpace($normalizedHost)) {
    return @()
  }

  $seen = @{}
  $candidates = New-Object System.Collections.Generic.List[string]
  $addHost = {
    param([string]$CandidateHost)
    $h = Trim-OneLine $CandidateHost
    $h = $h.TrimEnd('.').ToLowerInvariant()
    if ([string]::IsNullOrWhiteSpace($h)) { return }
    if ($seen.ContainsKey($h)) { return }
    $seen[$h] = $true
    $candidates.Add("http://${h}:$Port") | Out-Null
  }

  & $addHost $normalizedHost
  if ($normalizedHost -notmatch '\.') {
    foreach ($suffix in @(Get-DnsSuffixCandidates)) {
      & $addHost "$normalizedHost.$suffix"
    }
  }

  return @($candidates)
}

function Get-DnsCacheHostCandidates {
  $seen = @{}
  $values = New-Object System.Collections.Generic.List[string]
  $suffixes = @(Get-DnsSuffixCandidates)

  if (-not (Get-Command Get-DnsClientCache -ErrorAction SilentlyContinue)) {
    return @()
  }

  try {
    foreach ($entry in @(Get-DnsClientCache -ErrorAction Stop)) {
      $name = Trim-OneLine ([string]$entry.Entry)
      $name = $name.TrimEnd('.').ToLowerInvariant()
      if ([string]::IsNullOrWhiteSpace($name)) { continue }
      if ($name -eq 'localhost') { continue }
      if ($name -like '*.in-addr.arpa' -or $name -like '*.ip6.arpa') { continue }

      if ($name -match '\.') {
        $likelyLan = $false
        foreach ($suffix in $suffixes) {
          if ([string]::IsNullOrWhiteSpace($suffix)) { continue }
          if ($name -eq $suffix -or $name.EndsWith(".$suffix")) {
            $likelyLan = $true
            break
          }
        }
        if (-not $likelyLan -and $name -notmatch '\.(lan|local)$') {
          continue
        }
      }

      if (-not $seen.ContainsKey($name)) {
        $seen[$name] = $true
        $values.Add($name) | Out-Null
      }
    }
  } catch {
  }

  return @($values | Sort-Object)
}

function Invoke-CommandWithTimeout {
  param(
    [Parameter(Mandatory = $true)][string]$FilePath,
    [Parameter(Mandatory = $true)][string[]]$ArgumentList,
    [int]$TimeoutSeconds = 2
  )

  $stdoutFile = Join-Path $env:TEMP ("ciwi-installer-cmd-out-" + [Guid]::NewGuid().ToString('N') + ".log")
  $stderrFile = Join-Path $env:TEMP ("ciwi-installer-cmd-err-" + [Guid]::NewGuid().ToString('N') + ".log")
  $proc = $null
  try {
    $proc = Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -NoNewWindow -PassThru -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile
    $finished = $proc.WaitForExit(($TimeoutSeconds * 1000))
    if (-not $finished) {
      try { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue } catch {}
      try { $proc.WaitForExit(1000) } catch {}
    }
    $lines = @()
    if (Test-Path -LiteralPath $stdoutFile) {
      $lines += @(Get-Content -LiteralPath $stdoutFile -ErrorAction SilentlyContinue)
    }
    if (Test-Path -LiteralPath $stderrFile) {
      $lines += @(Get-Content -LiteralPath $stderrFile -ErrorAction SilentlyContinue)
    }
    return @($lines)
  } catch {
  } finally {
    if ($null -ne $proc) {
      try { $proc.Dispose() } catch {}
    }
    Remove-Item -LiteralPath $stdoutFile -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stderrFile -Force -ErrorAction SilentlyContinue
  }
  return @()
}

function Discover-MDNSServers {
  $found = @{}
  $dnsSd = Get-Command dns-sd -ErrorAction SilentlyContinue
  if ($null -eq $dnsSd) {
    return @()
  }
  $dnsSdPath = Trim-OneLine ([string]$dnsSd.Source)
  if ([string]::IsNullOrWhiteSpace($dnsSdPath)) {
    $dnsSdPath = Trim-OneLine ([string]$dnsSd.Path)
  }
  if ([string]::IsNullOrWhiteSpace($dnsSdPath)) {
    return @()
  }

  $browseLines = @(Invoke-CommandWithTimeout -FilePath $dnsSdPath -ArgumentList @('-B', '_ciwi._tcp', 'local') -TimeoutSeconds 3)
  $instanceSeen = @{}
  foreach ($line in $browseLines) {
    $text = Trim-OneLine ([string]$line)
    if ([string]::IsNullOrWhiteSpace($text)) { continue }
    if ($text -notmatch '\bAdd\b') { continue }
    $parts = $text -split '\s+'
    if ($parts.Length -lt 1) { continue }
    $instance = Trim-OneLine ([string]$parts[$parts.Length - 1])
    if ([string]::IsNullOrWhiteSpace($instance)) { continue }
    if ($instanceSeen.ContainsKey($instance)) { continue }
    $instanceSeen[$instance] = $true

    $resolveLines = @(Invoke-CommandWithTimeout -FilePath $dnsSdPath -ArgumentList @('-L', $instance, '_ciwi._tcp', 'local') -TimeoutSeconds 3)
    foreach ($resolveLine in $resolveLines) {
      $resolveText = Trim-OneLine ([string]$resolveLine)
      if ([string]::IsNullOrWhiteSpace($resolveText)) { continue }
      $m = [regex]::Match($resolveText, 'can be reached at\s+([^:.\s]+(?:\.[^:.\s]+)*)\s*:\s*([0-9]+)\.?', 'IgnoreCase')
      if (-not $m.Success) { continue }
      $mdnsHost = Trim-OneLine $m.Groups[1].Value
      $mdnsPort = 0
      if (-not [int]::TryParse((Trim-OneLine $m.Groups[2].Value), [ref]$mdnsPort)) { continue }
      foreach ($candidate in @(Build-HostUrlCandidates -HostName $mdnsHost -Port $mdnsPort)) {
        if (Test-CiwiServer -BaseUrl $candidate) {
          Add-UniqueServer -Map $found -Url $candidate
          break
        }
      }
    }
  }

  return @($found.Values | Sort-Object)
}

function Prefer-HostnameUrl {
  param([Parameter(Mandatory = $true)][string]$Url)
  $canonical = Canonicalize-Url $Url
  $uri = [Uri]$canonical
  if (-not (Is-IPv4 $uri.Host)) {
    return $canonical
  }
  $info = Get-ServerInfoJson -BaseUrl $canonical
  if ($null -eq $info) {
    return $canonical
  }
  $serverHost = Trim-OneLine ([string]$info.hostname)
  $serverHost = $serverHost.TrimEnd('.').ToLowerInvariant()
  if ([string]::IsNullOrWhiteSpace($serverHost)) {
    return $canonical
  }
  if ($serverHost -eq 'localhost' -or $serverHost -eq '127.0.0.1') {
    return $canonical
  }
  foreach ($candidate in @(Build-HostUrlCandidates -HostName $serverHost -Port $uri.Port)) {
    if (Test-CiwiServer -BaseUrl $candidate) {
      return $candidate
    }
  }
  return $canonical
}

function Add-UniqueServer {
  param(
    [Parameter(Mandatory = $true)][hashtable]$Map,
    [Parameter(Mandatory = $true)][string]$Url
  )
  try {
    $preferred = Prefer-HostnameUrl -Url $Url
    $key = Canonicalize-Url $preferred
    if (-not $Map.ContainsKey($key)) {
      $Map[$key] = $preferred
    }
  } catch {
  }
}

function Discover-Servers {
  $found = @{}
  foreach ($candidate in @('http://127.0.0.1:8112', 'http://localhost:8112')) {
    if (Test-CiwiServer -BaseUrl $candidate) {
      Add-UniqueServer -Map $found -Url $candidate
    }
  }

  foreach ($mdnsUrl in @(Discover-MDNSServers)) {
    Add-UniqueServer -Map $found -Url $mdnsUrl
  }

  # Probe likely LAN names already resolved by this host (for example from ping/nslookup).
  foreach ($dnsHost in @(Get-DnsCacheHostCandidates)) {
    foreach ($candidateHost in @(Build-HostUrlCandidates -HostName $dnsHost -Port 8112)) {
      if (Test-CiwiServer -BaseUrl $candidateHost) {
        Add-UniqueServer -Map $found -Url $candidateHost
        break
      }
    }
  }

  $ips = @{}
  if (Get-Command arp -ErrorAction SilentlyContinue) {
    foreach ($line in (& arp -a 2>$null)) {
      if ([string]::IsNullOrWhiteSpace($line)) { continue }
      foreach ($m in [regex]::Matches($line, '\b\d{1,3}(?:\.\d{1,3}){3}\b')) {
        $ip = Trim-OneLine $m.Value
        if (-not [string]::IsNullOrWhiteSpace($ip)) {
          $ips[$ip] = $true
        }
      }
    }
  }

  foreach ($ip in ($ips.Keys | Sort-Object)) {
    $candidateIp = "http://${ip}:8112"
    if (Test-CiwiServer -BaseUrl $candidateIp) {
      Add-UniqueServer -Map $found -Url $candidateIp
      continue
    }
    $resolvedHost = Resolve-HostnameForIp -Ip $ip
    if (-not [string]::IsNullOrWhiteSpace($resolvedHost)) {
      foreach ($candidateHost in @(Build-HostUrlCandidates -HostName $resolvedHost -Port 8112)) {
        if (Test-CiwiServer -BaseUrl $candidateHost) {
          Add-UniqueServer -Map $found -Url $candidateHost
          break
        }
      }
    }
  }

  return @($found.Values | Sort-Object)
}

function Choose-ServerUrl {
  $discovered = @(Discover-Servers)
  if ($discovered.Count -eq 1) {
    return $discovered[0]
  }
  if ($discovered.Count -gt 1) {
    Write-Host 'Multiple ciwi servers discovered:'
    for ($i = 0; $i -lt $discovered.Count; $i++) {
      Write-Host ("  [{0}] {1}" -f ($i + 1), $discovered[$i])
    }
    $choice = Trim-OneLine (Read-Host 'Choose server number [1]')
    if ([string]::IsNullOrWhiteSpace($choice)) {
      $choice = '1'
    }
    $idx = 0
    if (-not [int]::TryParse($choice, [ref]$idx)) {
      throw "invalid selection: $choice"
    }
    if ($idx -lt 1 -or $idx -gt $discovered.Count) {
      throw "invalid selection: $choice"
    }
    return $discovered[$idx - 1]
  }

  $entered = Trim-OneLine (Read-Host 'No ciwi server auto-discovered. Enter server URL (example http://bhakti.local:8112)')
  return Canonicalize-Url $entered
}

function New-GitHubHeaders {
  param(
    [string]$Token,
    [switch]$Api
  )
  $headers = @{
    'User-Agent' = 'ciwi-installer'
  }
  if ($Api.IsPresent) {
    $headers['Accept'] = 'application/vnd.github+json'
  }
  if (-not [string]::IsNullOrWhiteSpace($Token)) {
    $headers['Authorization'] = "Bearer $Token"
  }
  return $headers
}

function Get-LatestTag {
  param(
    [Parameter(Mandatory = $true)][string]$Repo,
    [string]$Token
  )
  $headers = New-GitHubHeaders -Token $Token -Api
  try {
    $rel = Invoke-RestMethod -Method Get -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
    return Trim-OneLine ([string]$rel.tag_name)
  } catch {
    return ''
  }
}

function Get-WindowsArch {
  $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
  switch ($arch) {
    'x64' { return 'amd64' }
    'arm64' { return 'arm64' }
    default { throw "unsupported architecture: $arch" }
  }
}

function Parse-Checksums {
  param([Parameter(Mandatory = $true)][string]$Content)
  $out = @{}
  foreach ($rawLine in ($Content -split "`n")) {
    $line = $rawLine.Trim()
    if ([string]::IsNullOrWhiteSpace($line)) { continue }
    if ($line.StartsWith('#')) { continue }
    $parts = $line -split '\s+'
    if ($parts.Length -lt 2) { continue }
    $name = $parts[$parts.Length - 1].TrimStart('*')
    $out[$name] = $parts[0].ToLowerInvariant()
  }
  return $out
}

function Read-EnvFileValue {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Key
  )
  if (-not (Test-Path -LiteralPath $Path)) {
    return ''
  }
  foreach ($rawLine in (Get-Content -LiteralPath $Path -ErrorAction SilentlyContinue)) {
    $line = [string]$rawLine
    if ($line.Trim().StartsWith('#')) { continue }
    $prefix = "$Key="
    if ($line.StartsWith($prefix)) {
      return $line.Substring($prefix.Length).Trim()
    }
  }
  return ''
}

function Write-AgentEnvFile {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$ServerUrl,
    [Parameter(Mandatory = $true)][string]$AgentId,
    [Parameter(Mandatory = $true)][string]$WorkDir,
    [string]$GitHubToken
  )

  $lines = @(
    "CIWI_SERVER_URL=$ServerUrl"
    "CIWI_AGENT_ID=$AgentId"
    "CIWI_AGENT_WORKDIR=$WorkDir"
    'CIWI_LOG_LEVEL=info'
    'CIWI_AGENT_TRACE_SHELL=true'
    'CIWI_WINDOWS_SERVICE_NAME=ciwi-agent'
  )
  if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
    $lines += "CIWI_GITHUB_TOKEN=$GitHubToken"
  }

  $parent = Split-Path -Parent $Path
  if (-not [string]::IsNullOrWhiteSpace($parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
  }
  Set-Content -LiteralPath $Path -Value $lines -Encoding Ascii

  & icacls $Path '/inheritance:r' '/grant:r' 'SYSTEM:(R,W)' 'Administrators:(R,W)' | Out-Null
}

function Wait-ServiceStopped {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [int]$TimeoutSeconds = 20
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $svc) { return }
    if ($svc.Status -eq [System.ServiceProcess.ServiceControllerStatus]::Stopped) { return }
    Start-Sleep -Milliseconds 500
  }
  throw "timed out waiting for service '$Name' to stop"
}

function Invoke-Sc {
  param([Parameter(Mandatory = $true)][string[]]$Args)
  $output = & sc.exe @Args 2>&1
  $code = $LASTEXITCODE
  if ($code -ne 0) {
    $text = ($output | ForEach-Object { $_.ToString().Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }) -join "`n"
    if ([string]::IsNullOrWhiteSpace($text)) {
      $text = '(no output)'
    }
    throw "sc.exe $($Args -join ' ') failed with exit code $code.`n$text"
  }
  return $output
}

function Ensure-Service {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][string]$BinaryPath
  )

  $bin = "`"$BinaryPath`" agent"
  $svc = Get-Service -Name $Name -ErrorAction SilentlyContinue
  $serviceRegPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$Name"
  if ($null -eq $svc) {
    New-Service `
      -Name $Name `
      -BinaryPathName $bin `
      -DisplayName 'ciwi agent' `
      -StartupType Automatic `
      -Description 'ciwi execution agent service' | Out-Null
  } else {
    Set-ItemProperty -Path $serviceRegPath -Name 'ImagePath' -Value $bin
    Set-ItemProperty -Path $serviceRegPath -Name 'DisplayName' -Value 'ciwi agent'
    Set-ItemProperty -Path $serviceRegPath -Name 'Description' -Value 'ciwi execution agent service'
    Set-Service -Name $Name -StartupType Automatic
  }

  $created = Get-Service -Name $Name -ErrorAction SilentlyContinue
  if ($null -eq $created) {
    throw "service '$Name' was not found after create/config"
  }
}

Require-Command curl.exe
Require-Command sc.exe
Require-Command icacls.exe
Ensure-Admin

$repo = 'izzyreal/ciwi'
$serviceName = 'ciwi-agent'
$programFilesRoot = Trim-OneLine $env:ProgramW6432
if ([string]::IsNullOrWhiteSpace($programFilesRoot)) {
  $programFilesRoot = Trim-OneLine $env:ProgramFiles
}
if ([string]::IsNullOrWhiteSpace($programFilesRoot)) {
  throw 'unable to resolve Program Files path'
}

$installDir = Join-Path $programFilesRoot 'ciwi'
$binaryPath = Join-Path $installDir 'ciwi.exe'
$dataRoot = Join-Path $env:ProgramData 'ciwi-agent'
$workDir = Join-Path $dataRoot 'work'
$logsDir = Join-Path $dataRoot 'logs'
$envFile = Join-Path $dataRoot 'agent.env'

$serverUrl = Trim-OneLine $env:CIWI_SERVER_URL
$serverUrlSource = 'CIWI_SERVER_URL environment variable'
if ([string]::IsNullOrWhiteSpace($serverUrl)) {
  $serverUrl = Trim-OneLine (Read-EnvFileValue -Path $envFile -Key 'CIWI_SERVER_URL')
  $serverUrlSource = 'existing agent env file'
}
if ([string]::IsNullOrWhiteSpace($serverUrl)) {
  Write-Host '[info] CIWI_SERVER_URL not set; auto-discovering ciwi server(s)...'
  $serverUrl = Choose-ServerUrl
  $serverUrlSource = 'auto-discovery / interactive selection'
} else {
  $serverUrl = Canonicalize-Url $serverUrl
}
Write-Host ("[info] Configuring CIWI_SERVER_URL={0} (source: {1})" -f $serverUrl, $serverUrlSource)
$agentId = Trim-OneLine $env:CIWI_AGENT_ID
if ([string]::IsNullOrWhiteSpace($agentId)) {
  $agentId = "agent-$($env:COMPUTERNAME)"
}

$token = Trim-OneLine $env:CIWI_GITHUB_TOKEN
if ([string]::IsNullOrWhiteSpace($token)) {
  $token = Trim-OneLine $env:INSTALL_GITHUB_TOKEN
}
if ([string]::IsNullOrWhiteSpace($token)) {
  $token = Trim-OneLine (Read-EnvFileValue -Path $envFile -Key 'CIWI_GITHUB_TOKEN')
}

$arch = Get-WindowsArch
$assetName = "ciwi-windows-$arch.exe"
$checksumAssetName = 'ciwi-checksums.txt'
$releaseBase = "https://github.com/$repo/releases/latest/download"

Write-Host "[1/7] Resolving latest release version..."
$targetVersion = Get-LatestTag -Repo $repo -Token $token
if (-not [string]::IsNullOrWhiteSpace($targetVersion)) {
  Write-Host "[info] Preparing to install ciwi agent version: $targetVersion"
} else {
  Write-Host '[info] Preparing to install ciwi agent version: unknown (GitHub API query failed; continuing with latest/download)'
}

$headers = New-GitHubHeaders -Token $token

$tempRoot = Join-Path $env:TEMP ("ciwi-agent-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
$assetPath = Join-Path $tempRoot $assetName
$checksumPath = Join-Path $tempRoot $checksumAssetName

try {
  Write-Host "[2/7] Downloading $assetName..."
  Invoke-WebRequest -Uri "$releaseBase/$assetName" -Headers $headers -OutFile $assetPath
  Invoke-WebRequest -Uri "$releaseBase/$checksumAssetName" -Headers $headers -OutFile $checksumPath

  Write-Host '[3/7] Verifying checksum...'
  $checksums = Parse-Checksums -Content (Get-Content -LiteralPath $checksumPath -Raw)
  if (-not $checksums.ContainsKey($assetName)) {
    throw "checksum entry not found for '$assetName'"
  }
  $expectedSha = $checksums[$assetName]
  $actualSha = (Get-FileHash -Algorithm SHA256 -LiteralPath $assetPath).Hash.ToLowerInvariant()
  if ($actualSha -ne $expectedSha) {
    throw "checksum mismatch for '$assetName' (expected $expectedSha, got $actualSha)"
  }

  Write-Host '[4/7] Preparing directories and environment file...'
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  New-Item -ItemType Directory -Force -Path $dataRoot | Out-Null
  New-Item -ItemType Directory -Force -Path $workDir | Out-Null
  New-Item -ItemType Directory -Force -Path $logsDir | Out-Null
  Write-AgentEnvFile -Path $envFile -ServerUrl $serverUrl -AgentId $agentId -WorkDir $workDir -GitHubToken $token

  Write-Host '[5/7] Stopping existing service if present...'
  $existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
  if ($null -ne $existing) {
    Invoke-Sc -Args @('stop', $serviceName) | Out-Null
    Wait-ServiceStopped -Name $serviceName -TimeoutSeconds 30
  }

  Write-Host '[6/7] Installing binary and configuring service...'
  Copy-Item -LiteralPath $assetPath -Destination $binaryPath -Force
  Ensure-Service -Name $serviceName -BinaryPath $binaryPath

  Write-Host '[7/7] Starting service...'
  Invoke-Sc -Args @('start', $serviceName) | Out-Null

  Write-Host ''
  Write-Host 'ciwi Windows agent installed and started.'
  Write-Host "Service:      $serviceName"
  Write-Host "Binary:       $binaryPath"
  Write-Host "Config:       $envFile"
  Write-Host "Server URL:   $serverUrl ($serverUrlSource)"
  Write-Host "Workdir:      $workDir"
  Write-Host "Logs dir:     $logsDir"
  Write-Host ''
  Write-Host 'Useful commands:'
  Write-Host "  Get-Service $serviceName"
  Write-Host "  sc.exe qc $serviceName"
  Write-Host "  sc.exe query $serviceName"
  Write-Host ''
  Write-Host 'To change target server/token:'
  Write-Host "  notepad $envFile"
  Write-Host "  sc.exe stop $serviceName"
  Write-Host "  sc.exe start $serviceName"
} finally {
  Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
