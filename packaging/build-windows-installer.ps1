param(
    [Parameter(Mandatory = $true)]
    [string]$Version,

    [Parameter(Mandatory = $true)]
    [string]$OutputExe
)

$ErrorActionPreference = "Stop"
$Version = $Version.Trim() -replace '^[vV]', ''
$repoRoot = Split-Path -Parent $PSScriptRoot
$gogioVersion = if ($env:GOGIO_VERSION) { $env:GOGIO_VERSION } else { "v0.10.0" }
$appExe = Join-Path $repoRoot "dist\Ciwi.exe"
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

& go run "gioui.org/cmd/gogio@$gogioVersion" `
    -target windows `
    -arch amd64 `
    -minsdk 10 `
    -appid nl.izmar.ciwi.desktop `
    -name Ciwi `
    -version "$Version.1" `
    -icon (Join-Path $repoRoot "packaging\icons\ciwi.png") `
    -ldflags "-s -w -X github.com/izzyreal/ciwi/internal/version.Version=v$Version" `
    -o $appExe `
    ./cmd/ciwi-desktop
if ($LASTEXITCODE -ne 0) { throw "gogio failed to build Ciwi.exe" }
Remove-Item (Join-Path $repoRoot "cmd\ciwi-desktop\Ciwi_windows_amd64.syso") -ErrorAction SilentlyContinue

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
