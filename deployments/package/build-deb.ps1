<#
.SYNOPSIS
    Builds the .deb package via Docker. PowerShell mirror of build-deb.sh.

.DESCRIPTION
    No host Go/Node toolchain required — the build runs entirely inside
    the iptv-deb-builder image.

.PARAMETER Version
    Explicit version override. Defaults to the latest git tag (with the
    leading 'v' stripped); falls back to 0.1.0 when there are no tags.

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

$commit = & git -C $RepoRoot rev-parse --short HEAD 2>$null
if ($LASTEXITCODE -ne 0 -or -not $commit) {
    $commit = 'unknown'
}

$dockerfile = Join-Path $RepoRoot 'deployments\package\Dockerfile.builder'
& docker build `
    -f $dockerfile `
    --build-arg "DEB_VERSION=$resolvedVersion" `
    --build-arg "VERSION=v$resolvedVersion" `
    --build-arg "COMMIT=$commit" `
    -t $image `
    $RepoRoot
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host ">> Extracting .deb to $outDir"

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