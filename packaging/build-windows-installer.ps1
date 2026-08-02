param(
    [Parameter(Mandatory = $true)]
    [string]$InputExe,

    [Parameter(Mandatory = $true)]
    [string]$Version,

    [Parameter(Mandatory = $true)]
    [string]$OutputExe
)

$ErrorActionPreference = "Stop"
$Version = $Version.Trim() -replace '^[vV]', ''
$repoRoot = Split-Path -Parent $PSScriptRoot
$appExe = [System.IO.Path]::GetFullPath($InputExe)
$iconPath = Join-Path $repoRoot "packaging\icons\ciwi.ico"
$installerWxs = Join-Path $PSScriptRoot "windows-installer.wxs"
$bundleWxs = Join-Path $PSScriptRoot "windows-bundle.wxs"
$intermediateDir = Join-Path $repoRoot "dist\windows-wix"
$msiPath = Join-Path $intermediateDir "Ciwi-Client-$Version-x64.msi"
$resolvedOutput = [System.IO.Path]::GetFullPath($OutputExe)

function Resolve-Wix {
    $command = Get-Command wix.exe -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    $command = Get-Command wix -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    throw "wix was not found. Install WiX Toolset v6 on the Windows runner."
}

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $resolvedOutput), $intermediateDir | Out-Null
if (-not (Test-Path -LiteralPath $appExe -PathType Leaf)) {
    throw "Windows application executable not found: $appExe"
}

$wix = Resolve-Wix
$wixVersion = (& $wix --version).Trim()
$wixVersionCore = ($wixVersion -split '[\+\-]', 2)[0]
if ([int](($wixVersionCore -split '\.')[0]) -lt 6) {
    throw "WiX Toolset v6 or newer is required. Found: $wixVersion"
}
$extension = "WixToolset.BootstrapperApplications.wixext/$wixVersionCore"
& $wix extension add --global $extension
if ($LASTEXITCODE -ne 0) { throw "WiX failed to add $extension" }

$common = @("build", "-arch", "x64", "-d", "Version=$Version", "-d", "AppIcon=$iconPath")
& $wix @common "-o" $msiPath "-d" "AppExe=$appExe" $installerWxs
if ($LASTEXITCODE -ne 0) { throw "WiX failed to build the Ciwi MSI" }
& $wix @common "-ext" $extension "-o" $resolvedOutput "-d" "MsiPath=$msiPath" $bundleWxs
if ($LASTEXITCODE -ne 0) { throw "WiX failed to build the Ciwi installer" }
