<#
.SYNOPSIS
    Builds the .deb package via Docker (Windows equivalent of build-deb.sh).

.DESCRIPTION
    Mirrors deployments/package/build-deb.sh so the same workflow runs on
    Windows / PowerShell Core. Resolves the .deb version from the latest
    git tag (stripping a leading 'v'), builds the iptv-deb-builder Docker
    image, and extracts iptv.deb into dist/iptv_<version>_amd64.deb.

.PARAMETER Version
    Explicit version override. When omitted, uses the latest git tag.

.EXAMPLE
    .\deployments\package\build-deb.ps1
    .\deployments\package\build-deb.ps1 -Version 1.2.3
#>
[CmdletBinding()]
param(
    [string]$Version
)

$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = (Resolve-Path (Join-Path $ScriptDir '..\..')).Path

function Resolve-DebVersion {
    param([string]$Explicit)

    if ($Explicit) {
        return $Explicit
    }

    $tag = & git -C $RepoRoot describe --tags --abbrev=0 2>$null
    if ($LASTEXITCODE -eq 0 -and $tag) {
        return ($tag -replace '^v', '')
    }

    return '0.1.0'
}

function Test-DebianVersion {
    param([string]$Ver)
    return $Ver -match '^[0-9][0-9a-zA-Z\.+~-]*$'
}

$resolvedVersion = Resolve-DebVersion -Explicit $Version
if (-not (Test-DebianVersion $resolvedVersion)) {
    Write-Error "Invalid version '$resolvedVersion': must start with a digit and contain only [0-9a-zA-Z.+~-]"
    exit 1
}

$image = "iptv-deb-builder:${resolvedVersion}"
$outDir = Join-Path $RepoRoot 'dist'
if (-not (Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}

Write-Host ">> Building $image with version $resolvedVersion"

$dockerfile = Join-Path $RepoRoot 'deployments\package\Dockerfile.builder'
& docker build `
    -f $dockerfile `
    --build-arg "DEB_VERSION=$resolvedVersion" `
    -t $image `
    $RepoRoot
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host ">> Extracting .deb to $outDir"

# Use docker create + docker cp to avoid PowerShell path-mangling on the
# container-side paths (mirrors the rationale in build-deb.sh).
$container = & docker create $image
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
try {
    $debDest = Join-Path $outDir "iptv_${resolvedVersion}_amd64.deb"
    & docker cp "${container}:/iptv.deb" $debDest
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
finally {
    & docker rm $container 2>$null | Out-Null
}

Write-Host ">> Done: $debDest"
