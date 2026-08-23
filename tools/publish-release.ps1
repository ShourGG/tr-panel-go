[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Version,
    [switch]$Stable
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $Version.StartsWith("v")) {
    $Version = "v$Version"
}
$releaseDir = Join-Path $repoRoot "dist\release\$Version"
$binaryPath = Join-Path $releaseDir "terraria-panel"
$checksumsPath = Join-Path $releaseDir "SHA256SUMS"
$notesPath = Join-Path $releaseDir "release-notes.md"

foreach ($path in @($binaryPath, $checksumsPath, $notesPath)) {
    if (-not (Test-Path $path)) {
        throw "Release file is missing: $path. Run tools/build-release.ps1 first."
    }
}

$flags = @("release", "create", $Version, $binaryPath, $checksumsPath, "--repo", "ShourGG/tr-panel-go", "--title", "TR Panel $Version", "--notes-file", $notesPath, "--target", "main")
if (-not $Stable) {
    $flags += "--prerelease"
}

Write-Host "Publishing $Version to GitHub..." -ForegroundColor Cyan
& gh @flags
if ($LASTEXITCODE -ne 0) {
    throw "GitHub release creation failed with exit code $LASTEXITCODE"
}
