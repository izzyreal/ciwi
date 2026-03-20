$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($args.Count -ne 0) {
  Write-Error 'usage: set CIWI_GITHUB_TOKEN and run this script directly'
  exit 2
}

function Ensure-Admin {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'run this token updater from an elevated PowerShell session (Run as Administrator)'
  }
}

function Trim-OneLine {
  param([string]$Value)
  if ($null -eq $Value) {
    return ''
  }
  return ($Value -replace "[`r`n]", '').Trim()
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

Ensure-Admin

$serviceName = 'ciwi-agent'
$dataRoot = Join-Path $env:ProgramData 'ciwi-agent'
$envFile = Join-Path $dataRoot 'agent.env'
$token = Trim-OneLine $env:CIWI_GITHUB_TOKEN

if ([string]::IsNullOrWhiteSpace($token)) {
  throw 'CIWI_GITHUB_TOKEN is required'
}

if (-not (Test-Path -LiteralPath $envFile)) {
  throw "agent env file not found: $envFile`ninstall the Windows agent first"
}

Write-Host '[1/3] Updating agent env file...'
$existing = @()
foreach ($rawLine in (Get-Content -LiteralPath $envFile -ErrorAction Stop)) {
  $line = [string]$rawLine
  if ($line.StartsWith('CIWI_GITHUB_TOKEN=')) {
    continue
  }
  $existing += $line
}
$existing += "CIWI_GITHUB_TOKEN=$token"
Set-Content -LiteralPath $envFile -Value $existing -Encoding Ascii
& icacls $envFile '/inheritance:r' '/grant:r' 'SYSTEM:(R,W)' 'Administrators:(R,W)' | Out-Null

Write-Host '[2/3] Restarting Windows service...'
$svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($null -eq $svc) {
  throw "service '$serviceName' not found"
}
if ($svc.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Stopped) {
  Stop-Service -Name $serviceName -Force
  Wait-ServiceStopped -Name $serviceName -TimeoutSeconds 30
}
Start-Service -Name $serviceName

Write-Host '[3/3] Done.'
Write-Host "ciwi Windows agent GitHub token updated in $envFile"
