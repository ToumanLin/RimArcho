# Builds a release: archo.exe and RimArcho.exe (pure Go, no cgo, no runtime to install; the same
# program, RimArcho.exe linked as a Windows GUI so a double click opens the window, 16 §8.1)
# with the version stamped in through -ldflags, plus README, CHANGELOG and LICENSE, and from
# 1.0.6 on README.zh-CN.md and skills/.
# Archo Monitor, the in-game companion mod (19 §6), is
# built by monitor/build.ps1 (needs the .NET SDK 10; game references come from NuGet) and
# shipped under monitor/ArchoMonitor/.
#
#   pwsh scripts/release.ps1 -Version 0.5.0            # tests, then dist/archo-0.5.0-windows-amd64.zip and .zip.sha256
#   pwsh scripts/release.ps1 -Version 0.5.0 -SkipTests
#
# Outputs go to -OutputDir (default dist/); the script never pushes or publishes.
# -GOOS linux builds only the Linux tarball. The default builds both platforms.
# -LinuxGUI puts rimarcho, the Linux GUI, in the tarball (17 §5): it needs cgo and GTK, so CI's
# linux-gui job builds it. Pass the downloaded artifact's path, or "ci" to download it with gh
# from the workflow_dispatch run of this commit (gh workflow run ci.yml -f version=<version>).
#
# The Windows ZIP can come from CI as well (17 §5): CI's windows-release job runs this script
# with -StageOnly (the folder, no archive), has SignPath sign the executables and Monitor's DLLs
# once signing is set up (docs/research/2026-11-24_signpath-code-signing.md), then runs it
# with -PackageOnly (archive and checksum of that folder). -FromCI downloads that ZIP for HEAD instead of building it here,
# and implies -LinuxGUI ci. The local build stays the fallback.
#
#   gh workflow run ci.yml --ref <branch> -f version=1.2.0     # after pushing the release commit
#   pwsh scripts/release.ps1 -Version 1.2.0 -FromCI
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [ValidateSet("windows", "linux")][string]$GOOS = "windows",
    [ValidateSet("amd64")][string]$GOARCH = "amd64",
    [string]$OutputDir = "dist",
    [string]$LinuxGUI = "",
    [switch]$SkipTests,
    [switch]$StageOnly,
    [switch]$PackageOnly,
    [switch]$FromCI
)
$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)

if ($Version -notmatch '^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$') { throw "Version must look like 1.2.3 or 1.2.3-rc1" }
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $local = Join-Path $env:LOCALAPPDATA "Programs\go\bin"
    if (Test-Path (Join-Path $local "go.exe")) { $env:Path = "$local;$env:Path" } else { throw "go not found on PATH" }
}

if (@($StageOnly, $PackageOnly, $FromCI | Where-Object { $_ }).Count -gt 1) { throw "-StageOnly, -PackageOnly and -FromCI exclude each other" }
if (($StageOnly -or $PackageOnly -or $FromCI) -and $GOOS -ne "windows") { throw "-StageOnly, -PackageOnly and -FromCI are for the Windows ZIP only" }
if ($FromCI -and -not $LinuxGUI) { $LinuxGUI = "ci" }

$commit = (git rev-parse --short=12 HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
if (git status --porcelain 2>$null) { $commit += "-dirty" }
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd")

if (-not $SkipTests -and -not $PackageOnly -and -not $FromCI) {
    # Test product code and build helpers, plus the maintained corpus-tool suites.
    # tmp/ is disposable diagnostics; other spikes are development/measurement programs.
    # Keep tested spikes explicit so local experiments cannot enter the release gate.
    # Scale checks run only with RIMARCHO_SCALE_TESTS=1; set it for scan or TUI speed changes.
    # -count=1 reruns tests without recording the costly Windows file-access test cache.
    Write-Host "go test -count=1 ./internal/... ./cmd/... ./scripts/... ./spikes/hugslogs ./spikes/corpuslabel"
    go test -count=1 ./internal/... ./cmd/... ./scripts/... ./spikes/hugslogs ./spikes/corpuslabel
    if ($LASTEXITCODE -ne 0) { throw "tests failed" }
}

$name = "archo-$Version-$GOOS-$GOARCH"
$OutputDir = [IO.Path]::GetFullPath($OutputDir)
$out = Join-Path $OutputDir $name
# -PackageOnly archives the folder -StageOnly built (and SignPath signed), so it keeps it.
if ($PackageOnly -and -not (Test-Path -LiteralPath (Join-Path $out "archo.exe"))) { throw "-PackageOnly needs the folder -StageOnly builds: $out" }
if (-not $PackageOnly -and (Test-Path $out)) { Remove-Item -Recurse -Force $out }
# A rebuild of the same version (after committing) replaces the archive and its checksum too.
foreach ($old in "$out.zip", "$out.zip.sha256", "$out.tar.gz", "$out.tar.gz.sha256") {
    if (Test-Path -LiteralPath $old) { Remove-Item -LiteralPath $old }
}
New-Item -ItemType Directory -Force $out | Out-Null

# -FromCI: the ZIP and checksum of CI's windows-release job for HEAD (artifact
# archo-windows-amd64 of the successful workflow_dispatch run), checked against each other and
# extracted into the staging folder, which publish-github.ps1 reads.
function Get-WindowsFromCI {
    $sha = (git rev-parse HEAD)
    $runs = gh run list --workflow ci.yml --commit $sha --event workflow_dispatch --status success --limit 1 --json databaseId | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or -not $runs) { throw "no successful workflow_dispatch CI run for $sha; run: gh workflow run ci.yml --ref <branch> -f version=$Version" }
    $id = $runs[0].databaseId
    $dir = Join-Path ([IO.Path]::GetTempPath()) "archo-windows-$Version-$id"
    if (Test-Path $dir) { Remove-Item -Recurse -Force $dir }
    gh run download $id -n archo-windows-amd64 -D $dir
    if ($LASTEXITCODE -ne 0) { throw "gh run download of archo-windows-amd64 from run $id failed (dispatched without -f version?)" }
    $zipName = "$name.zip"
    foreach ($f in $zipName, "$zipName.sha256") {
        if (-not (Test-Path -LiteralPath (Join-Path $dir $f))) { throw "CI run $id has no $f (dispatched with another version?)" }
        Copy-Item -LiteralPath (Join-Path $dir $f) (Join-Path $OutputDir $f)
    }
    Remove-Item -Recurse -Force $dir
    $zip = Join-Path $OutputDir $zipName
    $want = "$((Get-FileHash -Algorithm SHA256 -LiteralPath $zip).Hash.ToLowerInvariant())  $zipName"
    if ((Get-Content -LiteralPath "$zip.sha256" -Raw).Trim() -ne $want) { throw "$zipName from CI run $id does not match its .sha256" }
    Expand-Archive -LiteralPath $zip -DestinationPath $out
    Write-Host "Windows ZIP from CI run $id"
    foreach ($f in "archo.exe", "RimArcho.exe") {
        $sig = Get-AuthenticodeSignature -LiteralPath (Join-Path $out $f)
        Write-Host ("  {0}: {1}{2}" -f $f, $sig.Status, $(if ($sig.SignerCertificate) { ", " + $sig.SignerCertificate.Subject } else { "" }))
    }
}

$pkg = "github.com/ToumanLin/RimArcho/internal/buildinfo"
$ldflags = "-s -w -X $pkg.Version=$Version -X $pkg.Commit=$commit -X $pkg.Date=$date"
# -buildvcs=false: Go would otherwise stamp a module pseudo-version derived from the nearest git
# tag, which differs between CI's shallow clone and a local clone, so the same commit would not
# build the same bytes. The commit is stamped above; only dev builds read the VCS stamp.
$exe = if ($GOOS -eq "windows") { "archo.exe" } else { "archo" }
# Windows resources (16 §8.6): the keystone icon and the version information, written as a
# .syso next to main.go before each build (the two exes differ in their description) and
# removed afterwards; the .syso is never committed.
$syso = "cmd/archo/rsrc_windows_$GOARCH.syso"
function Write-Res([string]$file, [string]$desc) {
    if ($GOOS -ne "windows" -or $GOARCH -ne "amd64") { return }
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    go run ./scripts/winres -version $Version -exe $file -desc $desc -o $syso
    if ($LASTEXITCODE -ne 0) { throw "winres failed" }
    $env:GOOS = $GOOS; $env:GOARCH = $GOARCH
}
if ($FromCI) {
    Get-WindowsFromCI
} elseif (-not $PackageOnly) {
$env:CGO_ENABLED = "0"; $env:GOOS = $GOOS; $env:GOARCH = $GOARCH
try {
    Write-Res $exe "RimArcho command line"
    Write-Host "go build $exe ($Version, $commit, $date)"
    go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $out $exe) ./cmd/archo
    if ($LASTEXITCODE -ne 0) { throw "build failed" }
    if ($GOOS -eq "windows") {
        Write-Res "RimArcho.exe" "RimArcho"
        Write-Host "go build RimArcho.exe (Windows GUI)"
        go build -trimpath -buildvcs=false -ldflags "$ldflags -H windowsgui -X $pkg.GUI=1" -o (Join-Path $out "RimArcho.exe") ./cmd/archo
        if ($LASTEXITCODE -ne 0) { throw "build failed" }
    }
} finally {
    if (Test-Path $syso) { Remove-Item $syso }
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

# rimarcho, the Linux GUI (16 §8.1), comes from CI: it links GTK and WebKitGTK through cgo,
# which this script cannot build. It must be this version, built from this commit.
function Get-LinuxGUI([string]$from) {
    if ($from -eq "ci") {
        $sha = (git rev-parse HEAD)
        $runs = gh run list --workflow ci.yml --commit $sha --event workflow_dispatch --status success --limit 1 --json databaseId | ConvertFrom-Json
        if ($LASTEXITCODE -ne 0 -or -not $runs) { throw "no successful workflow_dispatch CI run for $sha; run: gh workflow run ci.yml --ref <branch> -f version=$Version" }
        $dir = Join-Path ([IO.Path]::GetTempPath()) "rimarcho-$Version-$($runs[0].databaseId)"
        if (Test-Path $dir) { Remove-Item -Recurse -Force $dir }
        gh run download $runs[0].databaseId -n rimarcho-linux-amd64 -D $dir
        if ($LASTEXITCODE -ne 0) { throw "gh run download failed" }
        $from = Join-Path $dir "rimarcho"
    }
    if (-not (Test-Path -LiteralPath $from -PathType Leaf)) { throw "rimarcho not found: $from" }
    $text = [Text.Encoding]::Latin1.GetString([IO.File]::ReadAllBytes($from))
    if (-not $text.StartsWith("$([char]0x7F)ELF")) { throw "$from is not a Linux program" }
    if (-not $text.Contains($Version)) { throw "$from does not carry version $Version (dispatch CI with -f version=$Version)" }
    $clean = $commit -replace '-dirty$', ''
    if ($clean -ne "unknown" -and -not $text.Contains($clean)) { throw "$from was not built from commit $clean" }
    return $from
}
if ($GOOS -eq "linux" -and $LinuxGUI) {
    $gui = Get-LinuxGUI $LinuxGUI
    Write-Host "rimarcho (Linux GUI, from CI): $gui"
    Copy-Item -LiteralPath $gui (Join-Path $out "rimarcho")
} elseif ($GOOS -eq "linux") {
    Write-Host "no -LinuxGUI: the Linux tarball has archo only, without the window (rimarcho)"
}

Copy-Item README.md, CHANGELOG.md, LICENSE $out

# README.zh-CN.md and skills/ (agent skills, such as rimworld-probe) from 1.0.6 on: 1.0.0's
# own update rejects a zip with top-level entries it does not know, and 1.0.5 knows these
# (selfupdate.Files). Updaters up to 1.0.x accept only selfupdate.Files, so the first release
# after 1.0.x packs nothing else; from the next one a new top-level entry may be added here
# directly (17 §5.1 step 3). Only files tracked by git go in, never local build output.
if ([version]($Version -replace '[-+].*$', '') -ge [version]"1.0.6") {
    Copy-Item README.zh-CN.md $out
    $skillFiles = @(git ls-files skills)
    if ($LASTEXITCODE -ne 0 -or $skillFiles.Count -eq 0) { throw "git ls-files skills found nothing" }
    foreach ($f in $skillFiles) {
        $dest = Join-Path $out $f
        New-Item -ItemType Directory -Force (Split-Path -Parent $dest) | Out-Null
        Copy-Item $f $dest
    }
} else {
    Write-Host "README.zh-CN.md and skills/ are packed from 1.0.6 on (older updaters reject them)"
}

# Archo Monitor (19 §6): built for 1.5, 1.6 and 1.7 with monitor/VERSION and shipped as
# monitor/ArchoMonitor/, where RimArcho finds it next to the exe.
Write-Host "building Archo Monitor"
& (Join-Path $PSScriptRoot "..\monitor\build.ps1") -Out (Join-Path $out "monitor\ArchoMonitor")
if (-not (Test-Path (Join-Path $out "monitor\ArchoMonitor\About\About.xml"))) { throw "Archo Monitor build produced no About.xml" }
}

if ($StageOnly) {
    Get-ChildItem -LiteralPath $out -Recurse -File | ForEach-Object { "  {0}  {1:N0}" -f $_.FullName, $_.Length }
    Write-Host "staged $out (no archive: -StageOnly; sign, then run with -PackageOnly)"
    return
}

if ($GOOS -eq "linux") {
    $archive = "$out.tar.gz"
    go run ./scripts/releasetar -dir $out -out $archive
    if ($LASTEXITCODE -ne 0) { throw "tar.gz packaging failed" }
} elseif ($FromCI) {
    $archive = "$out.zip"
} else {
    $archive = "$out.zip"
    Compress-Archive -Path (Join-Path $out "*") -DestinationPath $archive
}

# sha256sum format is shared by both platforms (17 §5.1); -FromCI keeps CI's, checked above.
$archiveName = Split-Path -Leaf $archive
if (-not $FromCI) {
    $hash = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText("$archive.sha256", "$hash  $archiveName`n", [Text.Encoding]::ASCII)
}
Write-Host ("dist: {0} ({1:N0} bytes)" -f $archive, (Get-Item $archive).Length)
Get-ChildItem -LiteralPath $out -Recurse -File | ForEach-Object { "  {0}  {1:N0}" -f $_.FullName, $_.Length }
if ($GOOS -eq "windows" -and $env:OS -eq "Windows_NT") {
    $built = (& (Join-Path $out $exe) version | Select-Object -First 1)
    Write-Host $built
    $clean = $commit -replace '-dirty$', ''
    if ($FromCI -and $built -notmatch "^archo $([regex]::Escape($Version)) \($([regex]::Escape($clean)), ") { throw "the CI build says '$built', not $Version at $clean" }
}
if ($GOOS -eq "windows" -and -not $PackageOnly) {
    & $PSCommandPath -Version $Version -GOOS linux -GOARCH $GOARCH -OutputDir $OutputDir -LinuxGUI $LinuxGUI -SkipTests
    # The publishing steps (17 §5 steps 7-10), in order; this script runs none of them.
    $root = (Get-Location).Path
    $zip = Join-Path $OutputDir "archo-$Version-windows-amd64.zip"
    $built = Join-Path $out "archo.exe"
    $lanzou = "file:///" + ((Join-Path $PSScriptRoot "publish-lanzou.mjs") -replace '\\', '/')
    Write-Host ""
    Write-Host "built $Version; to publish (spec 17, section 5), from $root, after committing and pushing:"
    Write-Host "  7. GitHub:   pwsh scripts/publish-github.ps1 -Version $Version -DryRun   (then without -DryRun)"
    if ($Version -match '-') {
        Write-Host "  prerelease: GitHub only; no Lanzou, no Workshop"
    } else {
        Write-Host "  8. Lanzou:   ego-browser nodejs -e `"await import('$lanzou`?zip=$($zip -replace '\\', '/')')`""
        Write-Host "  9. Workshop: $built workshop publish --version $Version --dry-run   (then without --dry-run)"
        Write-Host " 10. Verify:   an older build's 'archo update --check' lists github, monitor and lanzou with $Version"
    }
    if ($commit -match '-dirty') { Write-Host "warning: built from a dirty tree; publish-github.ps1 refuses it. Commit and rebuild." }
    if (-not $FromCI) { Write-Host "note: built here, not code-signed; a signed ZIP comes from CI (-FromCI, 17 section 5)" }
}
