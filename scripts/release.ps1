# Builds a release: one self-contained archo.exe (pure Go, no cgo, no runtime to install)
# with the version stamped in through -ldflags, plus README and CHANGELOG, zipped.
#
#   pwsh scripts/release.ps1 -Version 0.5.0            # tests, then dist/archo-0.5.0-windows-amd64.zip
#   pwsh scripts/release.ps1 -Version 0.5.0 -SkipTests
#
# Nothing outside dist/ is written; the script never pushes or publishes.
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [string]$GOOS = "windows",
    [string]$GOARCH = "amd64",
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
    Write-Host "go test ./..."
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "tests failed" }
}

$name = "archo-$Version-$GOOS-$GOARCH"
$out = Join-Path "dist" $name
if (Test-Path $out) { Remove-Item -Recurse -Force $out }
New-Item -ItemType Directory -Force $out | Out-Null

$pkg = "github.com/rimarcho/rimarcho/internal/buildinfo"
$ldflags = "-s -w -X $pkg.Version=$Version -X $pkg.Commit=$commit -X $pkg.Date=$date"
$exe = if ($GOOS -eq "windows") { "archo.exe" } else { "archo" }
$env:CGO_ENABLED = "0"; $env:GOOS = $GOOS; $env:GOARCH = $GOARCH
Write-Host "go build $exe ($Version, $commit, $date)"
go build -trimpath -ldflags $ldflags -o (Join-Path $out $exe) ./cmd/archo
if ($LASTEXITCODE -ne 0) { throw "build failed" }
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED

Copy-Item README.md, CHANGELOG.md $out
$zip = "$out.zip"
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $out "*") -DestinationPath $zip

$size = [math]::Round((Get-Item (Join-Path $out $exe)).Length / 1MB, 1)
if ($GOOS -eq "windows" -and $env:OS -eq "Windows_NT") { & (Join-Path $out $exe) version }
Write-Host "dist: $zip ($exe $size MB)"
