[CmdletBinding()]
param(
    [string]$Repo = "ShourGG/tr-panel-go",
    [string]$Branch = "main",
    [string]$ExpectedParent = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

function Invoke-GitHubJson {
    param(
        [Parameter(Mandatory = $true)][string]$Endpoint,
        [ValidateSet("GET", "POST", "PATCH", "PUT")][string]$Method = "GET",
        [object]$Body = $null
    )

    $tempPath = $null
    try {
        $arguments = @("api", $Endpoint, "--method", $Method)
        if ($null -ne $Body) {
            $tempPath = Join-Path ([IO.Path]::GetTempPath()) ("tr-panel-gh-" + [guid]::NewGuid().ToString("N") + ".json")
            $json = $Body | ConvertTo-Json -Depth 20 -Compress
            [IO.File]::WriteAllText($tempPath, $json, [Text.UTF8Encoding]::new($false))
            $arguments += @("--input", $tempPath)
        }
        $output = & gh @arguments 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "GitHub API request failed: $Endpoint`n$($output -join "`n")"
        }
        $text = ($output -join "`n").Trim()
        if ([string]::IsNullOrWhiteSpace($text)) {
            return $null
        }
        return $text | ConvertFrom-Json
    } finally {
        if ($tempPath -and (Test-Path -LiteralPath $tempPath)) {
            Remove-Item -LiteralPath $tempPath -Force
        }
    }
}

function Get-LocalTree {
    $entries = @{}
    $lines = & git -C $repoRoot ls-tree -r --full-tree HEAD
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to read local HEAD tree"
    }
    foreach ($line in $lines) {
        $parts = $line -split "`t", 2
        if ($parts.Count -ne 2) {
            continue
        }
        $meta = $parts[0] -split " ", 4
        $entries[$parts[1]] = [ordered]@{
            path = $parts[1]
            mode = $meta[0]
            type = $meta[1]
            sha  = $meta[2]
        }
    }
    return $entries
}

function Get-LocalCommitField([string]$Format) {
    $value = & git -C $repoRoot show -s --format=$Format HEAD
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to read local commit metadata"
    }
    return ($value -join "`n").TrimEnd()
}

$ref = Invoke-GitHubJson "repos/$Repo/git/ref/heads/$Branch"
$remoteHead = $ref.object.sha
if ($ExpectedParent -and $remoteHead -ne $ExpectedParent) {
    throw "Remote $Branch moved. Expected $ExpectedParent but found $remoteHead; refusing to overwrite it."
}

$remoteCommit = Invoke-GitHubJson "repos/$Repo/git/commits/$remoteHead"
$remoteTree = Invoke-GitHubJson "repos/$Repo/git/trees/$($remoteCommit.tree.sha)?recursive=1"
$remoteEntries = @{}
foreach ($entry in $remoteTree.tree) {
    if ($entry.type -ne "tree") {
        $remoteEntries[$entry.path] = $entry
    }
}

$localEntries = Get-LocalTree
$treeChanges = [System.Collections.Generic.List[object]]::new()
$changedCount = 0

foreach ($path in $localEntries.Keys) {
    $local = $localEntries[$path]
    $remote = $remoteEntries[$path]
    if ($null -ne $remote -and $remote.sha -eq $local.sha -and $remote.mode -eq $local.mode) {
        continue
    }

    $relativePath = $path.Replace('/', [IO.Path]::DirectorySeparatorChar)
    $fullPath = Join-Path $repoRoot $relativePath
    if ($local.type -eq "blob") {
        $content = [Convert]::ToBase64String([IO.File]::ReadAllBytes($fullPath))
        $blob = Invoke-GitHubJson "repos/$Repo/git/blobs" "POST" ([ordered]@{
            content  = $content
            encoding = "base64"
        })
        $sha = $blob.sha
    } else {
        $sha = $local.sha
    }
    $treeChanges.Add([ordered]@{
        path = $path
        mode = $local.mode
        type = $local.type
        sha  = $sha
    })
    $changedCount++
}

foreach ($path in $remoteEntries.Keys) {
    if (-not $localEntries.ContainsKey($path)) {
        $treeChanges.Add([ordered]@{
            path = $path
            mode = $remoteEntries[$path].mode
            type = $remoteEntries[$path].type
            sha  = $null
        })
        $changedCount++
    }
}

if ($changedCount -eq 0) {
    Write-Host "GitHub $Repo/$Branch already matches local HEAD ($remoteHead)." -ForegroundColor Green
    exit 0
}

Write-Host "Uploading $changedCount changed tree entries to $Repo/$Branch..." -ForegroundColor Cyan
$tree = Invoke-GitHubJson "repos/$Repo/git/trees" "POST" ([ordered]@{
    base_tree = $remoteCommit.tree.sha
    tree      = @($treeChanges)
})

$message = Get-LocalCommitField "%B"
$authorName = Get-LocalCommitField "%an"
$authorEmail = Get-LocalCommitField "%ae"
$authorDate = Get-LocalCommitField "%aI"
$committerName = Get-LocalCommitField "%cn"
$committerEmail = Get-LocalCommitField "%ce"
$committerDate = Get-LocalCommitField "%cI"
$newCommit = Invoke-GitHubJson "repos/$Repo/git/commits" "POST" ([ordered]@{
    message = $message
    tree    = $tree.sha
    parents = @($remoteHead)
    author  = [ordered]@{ name = $authorName; email = $authorEmail; date = $authorDate }
    committer = [ordered]@{ name = $committerName; email = $committerEmail; date = $committerDate }
})

$updatedRef = Invoke-GitHubJson "repos/$Repo/git/refs/heads/$Branch" "PATCH" ([ordered]@{
    sha   = $newCommit.sha
    force = $false
})

Write-Host "GitHub branch updated: $($updatedRef.object.sha)" -ForegroundColor Green
