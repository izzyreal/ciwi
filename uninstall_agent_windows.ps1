$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($args.Count -ne 0) {
  Write-Error 'this uninstaller takes no options; run it directly'
  exit 2
}

function Ensure-Admin {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'run this uninstaller from an elevated PowerShell session (Run as Administrator)'
  }
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
$programFilesRoot = $env:ProgramW6432
if ([string]::IsNullOrWhiteSpace($programFilesRoot)) {
  $programFilesRoot = $env:ProgramFiles
}
$installDir = Join-Path $programFilesRoot 'ciwi'
$binaryPath = Join-Path $installDir 'ciwi.exe'
$dataRoot = Join-Path $env:ProgramData 'ciwi-agent'
$workDir = Join-Path $dataRoot 'work'
$logsDir = Join-Path $dataRoot 'logs'
$envFile = Join-Path $dataRoot 'agent.env'

Write-Host '[1/4] Stopping and deleting Windows service...'
$svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($null -ne $svc) {
  & sc.exe stop $serviceName | Out-Null
  Wait-ServiceStopped -Name $serviceName -TimeoutSeconds 30
  & sc.exe delete $serviceName | Out-Null
}

Write-Host '[2/4] Removing ciwi binary...'
if (Test-Path -LiteralPath $binaryPath) {
  Remove-Item -LiteralPath $binaryPath -Force
  Write-Host "Removed $binaryPath"
}
if (Test-Path -LiteralPath $installDir) {
  $remaining = Get-ChildItem -LiteralPath $installDir -Force -ErrorAction SilentlyContinue
  if ($null -eq $remaining -or $remaining.Count -eq 0) {
    Remove-Item -LiteralPath $installDir -Force -ErrorAction SilentlyContinue
  }
}

Write-Host '[3/4] Keeping data by default:'
Write-Host "  $dataRoot"
Write-Host "  $workDir"
Write-Host "  $logsDir"
Write-Host "  $envFile"
$answer = Read-Host 'Remove data/logs/config now too? [y/N]'
if ($answer -match '^(?i:y|yes)$') {
  if (Test-Path -LiteralPath $dataRoot) {
    Remove-Item -LiteralPath $dataRoot -Recurse -Force
    Write-Host "Removed $dataRoot"
  }
} else {
  Write-Host "Kept $dataRoot"
}

Write-Host '[4/4] Done.'
Write-Host 'ciwi Windows agent uninstall complete.'
