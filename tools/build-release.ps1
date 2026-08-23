[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$FrontendDir = "",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

function Invoke-Native {
    param(
        [string]$File,
        [string[]]$Arguments,
        [string]$WorkingDirectory = $repoRoot
    )

    Push-Location $WorkingDirectory
    try {
        & $File @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "$File failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
}

if ([string]::IsNullOrWhiteSpace($FrontendDir)) {
    $FrontendDir = $env:TR_PANEL_FRONTEND_DIR
}
if ([string]::IsNullOrWhiteSpace($FrontendDir)) {
    $localCandidate = Join-Path $repoRoot "frontend"
    if (Test-Path (Join-Path $localCandidate "package.json")) {
        $FrontendDir = $localCandidate
    }
}
if ([string]::IsNullOrWhiteSpace($FrontendDir) -or -not (Test-Path (Join-Path $FrontendDir "package.json"))) {
    throw "Frontend source was not found. Pass -FrontendDir <path> or set TR_PANEL_FRONTEND_DIR."
}
$FrontendDir = (Resolve-Path $FrontendDir).Path

if ([string]::IsNullOrWhiteSpace($Version)) {
    $package = Get-Content (Join-Path $FrontendDir "package.json") -Raw | ConvertFrom-Json
    $Version = "v$($package.version)"
}
if (-not $Version.StartsWith("v")) {
    $Version = "v$Version"
}
$versionNumber = $Version.Substring(1)

$frontendDist = Join-Path $FrontendDir "dist"
$embedDist = Join-Path $repoRoot "web\dist"
$legacyFrontendDist = Join-Path $repoRoot "frontend\dist"
$releaseDir = Join-Path $repoRoot "dist\release\$Version"
$binaryPath = Join-Path $releaseDir "terraria-panel"
$notesPath = Join-Path $releaseDir "release-notes.md"

Write-Host "== TR Panel release build $Version ==" -ForegroundColor Cyan
Write-Host "Repository: $repoRoot"
Write-Host "Frontend:   $FrontendDir"

if (-not $SkipTests) {
    Write-Host "[1/6] Go tests" -ForegroundColor Yellow
    $oldTestGoos = $env:GOOS
    $oldTestGoarch = $env:GOARCH
    $oldTestJwtSecret = $env:JWT_SECRET
    try {
        # The workstation may have GOOS=linux in go env; tests must match the host.
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        $env:JWT_SECRET = "test-only-jwt-secret-for-local-build-validation-32"
        Invoke-Native "go" @("test", "./...") $repoRoot
    } finally {
        $env:GOOS = $oldTestGoos
        $env:GOARCH = $oldTestGoarch
        $env:JWT_SECRET = $oldTestJwtSecret
    }
} else {
    Write-Host "[1/6] Tests skipped" -ForegroundColor DarkYellow
}

Write-Host "[2/6] Frontend build" -ForegroundColor Yellow
if (-not (Test-Path (Join-Path $FrontendDir "node_modules"))) {
    Invoke-Native "npm.cmd" @("ci") $FrontendDir
}
Invoke-Native "npm.cmd" @("run", "build") $FrontendDir
if (-not (Test-Path (Join-Path $frontendDist "index.html"))) {
    throw "Frontend build did not produce dist/index.html"
}

function Sync-Dist {
    param([string]$Target)
    if (Test-Path $Target) {
        Remove-Item -LiteralPath $Target -Recurse -Force
    }
    New-Item -ItemType Directory -Path $Target -Force | Out-Null
    Copy-Item -Path (Join-Path $frontendDist "*") -Destination $Target -Recurse -Force
}

Write-Host "[3/6] Sync embedded frontend" -ForegroundColor Yellow
Sync-Dist $embedDist
if (Test-Path (Join-Path $repoRoot "frontend")) {
    Sync-Dist $legacyFrontendDist
}

Write-Host "[4/6] Linux amd64 build" -ForegroundColor Yellow
New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
$oldCgo = $env:CGO_ENABLED
$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    Invoke-Native "go" @("build", "-trimpath", "-ldflags=-s -w -X terraria-panel/buildinfo.Version=$versionNumber", "-o", $binaryPath, ".") $repoRoot
} finally {
    $env:CGO_ENABLED = $oldCgo
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
}

Write-Host "[5/6] Verify ELF and checksum" -ForegroundColor Yellow
$bytes = [System.IO.File]::ReadAllBytes($binaryPath)
if ($bytes.Length -lt 4 -or $bytes[0] -ne 0x7f -or $bytes[1] -ne 0x45 -or $bytes[2] -ne 0x4c -or $bytes[3] -ne 0x46) {
    throw "Build output is not an ELF binary: $binaryPath"
}
$hash = (Get-FileHash -LiteralPath $binaryPath -Algorithm SHA256).Hash.ToLowerInvariant()
Set-Content -LiteralPath (Join-Path $releaseDir "SHA256SUMS") -Value "$hash  terraria-panel" -Encoding ascii

$notes = @(
    "## $Version"
    ""
    "- Add a unified room console command API for vanilla, tModLoader, and room-scoped TShock."
    "- Add command input and live output to the room console page."
    "- Keep the TShock PTY command path and normalize stdin commands with a newline."
    "- Improve one-command Linux installation and preserve existing panel data during updates."
    ""
    "SHA-256: $hash"
) -join "`n"
Set-Content -LiteralPath $notesPath -Value $notes -Encoding utf8

Write-Host "[6/6] Build complete" -ForegroundColor Green
Write-Host "Binary:  $binaryPath"
Write-Host "SHA256:  $hash"
Write-Host "Notes:   $notesPath"
