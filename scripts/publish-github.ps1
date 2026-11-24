# Publishes a release built by scripts/release.ps1 to GitHub Releases (17 §5 step 7, maintainer
# only): creates tag v<version> on GitHub at the current commit and uploads the Windows ZIP, the
# Linux tarball and their .sha256 files. The notes are CHANGELOG.md's "## <version>" section.
# A prerelease (-rc1 and the like) is marked as one. release.ps1 itself never publishes.
#
#   pwsh scripts/publish-github.ps1 -Version 1.1.0 -DryRun   # every check, nothing created
#   pwsh scripts/publish-github.ps1 -Version 1.1.0
#
# Refuses: a dirty working tree, a commit GitHub does not have yet (push it first), a tag or
# release that already exists, a missing or mismatched asset, binaries built from another
# commit, a stable release without a passing update rehearsal of this ZIP
# (scripts/vm/update-rehearsal.ps1), and a CHANGELOG without the version's section. Needs gh, signed in (gh auth login).
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [string]$OutputDir = "dist",
    [string]$Repo = "ToumanLin/RimArcho",
    [string]$NotesFile,
    [switch]$DryRun
)
$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)

if ($Version -notmatch '^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$') { throw "Version must look like 1.2.3 or 1.2.3-rc1" }
$prerelease = $Version -match '-'
$tag = "v$Version"
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw "gh (GitHub CLI) not found on PATH" }

# The release is the commit the binaries were built from, and nothing uncommitted.
if (git status --porcelain) { throw "the working tree has uncommitted changes; commit them, rebuild with scripts/release.ps1, then publish" }
$commit = (git rev-parse HEAD).Trim()
$short = $commit.Substring(0, 12)
gh api "repos/$Repo/commits/$commit" --silent 2>$null
if ($LASTEXITCODE -ne 0) { throw "GitHub does not have commit $short yet; push it first (the tag is created there at this commit)" }
if (git tag --list $tag) { throw "tag $tag already exists locally" }
if (git ls-remote --tags origin "refs/tags/$tag") { throw "tag $tag already exists on GitHub" }
gh release view $tag --repo $Repo *> $null
if ($LASTEXITCODE -eq 0) { throw "release $tag already exists on GitHub" }

# The four assets, each checked against its .sha256.
$OutputDir = [IO.Path]::GetFullPath($OutputDir)
$assets = @()
foreach ($archive in "archo-$Version-windows-amd64.zip", "archo-$Version-linux-amd64.tar.gz") {
    $path = Join-Path $OutputDir $archive
    $sum = "$path.sha256"
    foreach ($f in $path, $sum) { if (-not (Test-Path -LiteralPath $f -PathType Leaf)) { throw "$f is missing; run scripts/release.ps1 -Version $Version" } }
    $want = "$((Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant())  $archive"
    $got = (Get-Content -LiteralPath $sum -Raw).Trim()
    if ($got -ne $want) { throw "$sum does not match $archive" }
    $assets += $path, $sum
}

# The Windows build must report this version and commit, not a dirty or older one.
$exe = Join-Path $OutputDir "archo-$Version-windows-amd64\archo.exe"
if (-not (Test-Path -LiteralPath $exe)) { throw "$exe is missing; run scripts/release.ps1 -Version $Version" }
# A built program never runs against the player's real data folder (17 §4): an unreleased
# schema would lock released RimArcho out of it.
function Invoke-Isolated([string]$Exe, [string[]]$Arguments) {
    $saved = $env:RIMARCHO_HOME
    $env:RIMARCHO_HOME = Join-Path ([IO.Path]::GetTempPath()) ("rimarcho-release-" + [guid]::NewGuid().ToString("N"))
    try { & $Exe @Arguments } finally {
        Remove-Item -LiteralPath $env:RIMARCHO_HOME -Recurse -Force -ErrorAction SilentlyContinue
        $env:RIMARCHO_HOME = $saved
    }
}
$built = (Invoke-Isolated $exe @("version") | Select-Object -First 1)
if ($built -notmatch "^archo $([regex]::Escape($Version)) \($short, ") { throw "the built archo.exe says '$built', not $Version at $short; rebuild with scripts/release.ps1" }

# A stable release has passed the update rehearsal (17 §5): this very ZIP updated itself in
# the VM. scripts/vm/update-rehearsal.ps1 writes the record beside the ZIP on a pass.
if (-not $prerelease) {
    $zip = Join-Path $OutputDir "archo-$Version-windows-amd64.zip"
    $rec = Join-Path $OutputDir "update-rehearsal-$Version.json"
    $again = "run: pwsh scripts/vm/update-rehearsal.ps1 -Candidate $zip"
    if (-not (Test-Path -LiteralPath $rec)) { throw "no update rehearsal for $Version ($rec); $again" }
    $r = Get-Content -Raw -LiteralPath $rec | ConvertFrom-Json
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $zip).Hash.ToLowerInvariant()
    if (-not $r.passed -or $r.version -ne $Version -or $r.commit -ne $short -or $r.sha256 -ne $hash) {
        throw "the update rehearsal record does not match this ZIP (passed $($r.passed), $($r.version) at $($r.commit)); $again"
    }
    Write-Host "rehearsal: passed $($r.date) ($Version -> $($r.rehearsal))"
}

# Notes: CHANGELOG.md's section for this version, or -NotesFile.
if ($NotesFile) {
    $notes = Get-Content -LiteralPath $NotesFile -Raw -Encoding utf8
} else {
    $lines = Get-Content -LiteralPath CHANGELOG.md -Encoding utf8
    $start = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match "^## $([regex]::Escape($Version))(\s|$)") { $start = $i + 1; break }
    }
    if ($start -lt 0) { throw "CHANGELOG.md has no '## $Version' section; rename 'Unreleased' to the version first" }
    $end = $lines.Count
    for ($i = $start; $i -lt $lines.Count; $i++) { if ($lines[$i] -match '^## ') { $end = $i; break } }
    $notes = ($lines[$start..($end - 1)] -join "`n").Trim()
}
if (-not $notes.Trim()) { throw "the release notes are empty" }
$notesPath = Join-Path ([IO.Path]::GetTempPath()) "rimarcho-notes-$Version.md"
[IO.File]::WriteAllText($notesPath, $notes + "`n", [Text.UTF8Encoding]::new($false))

$ghArgs = @("release", "create", $tag) + $assets + @("--repo", $Repo, "--target", $commit, "--title", "RimArcho $Version", "--notes-file", $notesPath)
if ($prerelease) { $ghArgs += "--prerelease" } else { $ghArgs += "--latest" }
Write-Host "release:  $tag on $Repo at $short$(if ($prerelease) { ' (prerelease)' })"
$assets | ForEach-Object { Write-Host ("asset:    {0} ({1:N0} bytes)" -f (Split-Path -Leaf $_), (Get-Item -LiteralPath $_).Length) }
Write-Host ("notes:    {0} lines from {1}" -f ($notes -split "`n").Count, $(if ($NotesFile) { $NotesFile } else { "CHANGELOG.md" }))
if ($DryRun) {
    Write-Host "dry run: would run gh $($ghArgs -join ' ')"
    Remove-Item -LiteralPath $notesPath
    return
}
try {
    gh @ghArgs
    if ($LASTEXITCODE -ne 0) { throw "gh release create failed; check https://github.com/$Repo/releases before retrying" }
} finally {
    Remove-Item -LiteralPath $notesPath -ErrorAction SilentlyContinue
}
Write-Host "published: https://github.com/$Repo/releases/tag/$tag"
if (-not $prerelease) {
    $zip = Join-Path $OutputDir "archo-$Version-windows-amd64.zip"
    Write-Host "next (spec 17, section 5, steps 8-10):"
    Write-Host "  ego-browser nodejs -e `"await import('file:///$(($PSScriptRoot -replace '\\', '/'))/publish-lanzou.mjs?zip=$($zip -replace '\\', '/')')`""
    Write-Host "  $exe workshop publish --version $Version   (from $(Get-Location))"
}
