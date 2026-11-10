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
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [ValidateSet("windows", "linux")][string]$GOOS = "windows",
    [ValidateSet("amd64")][string]$GOARCH = "amd64",
    [string]$OutputDir = "dist",
    [switch]$SkipTests
)
$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)

if ($Version -notmatch '^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$') { throw "Version must look like 1.2.3 or 1.2.3-rc1" }
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $local = Join-Path $env:LOCALAPPDATA "Programs\go\bin"
    if (Test-Path (Join-Path $local "go.exe")) { $env:Path = "$local;$env:Path" } else { throw "go not found on PATH" }
}

$commit = (git rev-parse --short=12 HEAD 2>$null)
if (-not $commit) { $commit = "unknown" }
if (git status --porcelain 2>$null) { $commit += "-dirty" }
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd")

if (-not $SkipTests) {
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
if (Test-Path $out) { Remove-Item -Recurse -Force $out }
New-Item -ItemType Directory -Force $out | Out-Null

$pkg = "github.com/ToumanLin/RimArcho/internal/buildinfo"
$ldflags = "-s -w -X $pkg.Version=$Version -X $pkg.Commit=$commit -X $pkg.Date=$date"
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
$env:CGO_ENABLED = "0"; $env:GOOS = $GOOS; $env:GOARCH = $GOARCH
try {
    Write-Res $exe "RimArcho command line"
    Write-Host "go build $exe ($Version, $commit, $date)"
    go build -trimpath -ldflags $ldflags -o (Join-Path $out $exe) ./cmd/archo
    if ($LASTEXITCODE -ne 0) { throw "build failed" }
    if ($GOOS -eq "windows") {
        Write-Res "RimArcho.exe" "RimArcho"
        Write-Host "go build RimArcho.exe (Windows GUI)"
        go build -trimpath -ldflags "$ldflags -H windowsgui -X $pkg.GUI=1" -o (Join-Path $out "RimArcho.exe") ./cmd/archo
        if ($LASTEXITCODE -ne 0) { throw "build failed" }
    }
} finally {
    if (Test-Path $syso) { Remove-Item $syso }
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Copy-Item README.md, CHANGELOG.md, LICENSE $out

# README.zh-CN.md and skills/ (agent skills, such as rimworld-probe) from 1.0.6 on: 1.0.0's
# own update rejects a zip with top-level entries it does not know, and 1.0.5 knows these
# (selfupdate.Files). Only files tracked by git go in, never local build output.
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

if ($GOOS -eq "linux") {
    $archive = "$out.tar.gz"
    go run ./scripts/releasetar -dir $out -out $archive
    if ($LASTEXITCODE -ne 0) { throw "tar.gz packaging failed" }
} else {
    $archive = "$out.zip"
    Compress-Archive -Path (Join-Path $out "*") -DestinationPath $archive
}

# sha256sum format is shared by both platforms (17 §5.1).
$archiveName = Split-Path -Leaf $archive
$hash = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
[IO.File]::WriteAllText("$archive.sha256", "$hash  $archiveName`n", [Text.Encoding]::ASCII)
Write-Host ("dist: {0} ({1:N0} bytes)" -f $archive, (Get-Item $archive).Length)
Get-ChildItem -LiteralPath $out -Recurse -File | ForEach-Object { "  {0}  {1:N0}" -f $_.FullName, $_.Length }
if ($GOOS -eq "windows" -and $env:OS -eq "Windows_NT") { & (Join-Path $out $exe) version }
if ($GOOS -eq "windows") {
    & $PSCommandPath -Version $Version -GOOS linux -GOARCH $GOARCH -OutputDir $OutputDir -SkipTests
}
