# Builds a release: one self-contained archo.exe (pure Go, no cgo, no runtime to install)
# with the version stamped in through -ldflags, plus README, CHANGELOG and LICENSE, and the font of
# the WezTerm host (06 §1.2): Maple Mono NF CN Regular and Bold with its SIL OFL license under
# fonts/. The font is fetched at build time from the official Maple Mono GitHub release
# (pinned by -MapleTag, checked against the release's .sha256) and cached in dist/cache/;
# it is never committed to the repository.
#
#   pwsh scripts/release.ps1 -Version 0.5.0            # tests, then dist/archo-0.5.0-windows-amd64.zip
#   pwsh scripts/release.ps1 -Version 0.5.0 -SkipTests
#
# Nothing outside dist/ is written; the script never pushes or publishes.
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [string]$GOOS = "windows",
    [string]$GOARCH = "amd64",
    [string]$MapleTag = "v7.9",
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

$pkg = "github.com/ToumanLin/RimArcho/internal/buildinfo"
$ldflags = "-s -w -X $pkg.Version=$Version -X $pkg.Commit=$commit -X $pkg.Date=$date"
$exe = if ($GOOS -eq "windows") { "archo.exe" } else { "archo" }
$env:CGO_ENABLED = "0"; $env:GOOS = $GOOS; $env:GOARCH = $GOARCH
Write-Host "go build $exe ($Version, $commit, $date)"
go build -trimpath -ldflags $ldflags -o (Join-Path $out $exe) ./cmd/archo
if ($LASTEXITCODE -ne 0) { throw "build failed" }
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED

Copy-Item README.md, CHANGELOG.md, LICENSE $out

# The bundled font (06 §1.2): Maple Mono NF CN, Regular and Bold, and the OFL.
$maple = "https://github.com/subframe7536/maple-font/releases/download/$MapleTag"
$cache = Join-Path "dist" "cache\maple-$MapleTag"
New-Item -ItemType Directory -Force $cache | Out-Null
$fontZip = Join-Path $cache "MapleMono-NF-CN.zip"
$shaFile = Join-Path $cache "MapleMono-NF-CN.sha256"
$ProgressPreference = "SilentlyContinue"
if (-not (Test-Path $shaFile)) { Invoke-WebRequest "$maple/MapleMono-NF-CN.sha256" -OutFile $shaFile -UseBasicParsing }
$want = ((Get-Content $shaFile -Raw) -split '\s+')[0].Trim().ToLower()
if ($want -notmatch '^[0-9a-f]{64}$') { throw "no sha256 in $shaFile" }
if (-not (Test-Path $fontZip) -or (Get-FileHash $fontZip -Algorithm SHA256).Hash.ToLower() -ne $want) {
    Write-Host "downloading MapleMono-NF-CN.zip ($MapleTag) from the official release"
    Invoke-WebRequest "$maple/MapleMono-NF-CN.zip" -OutFile $fontZip -UseBasicParsing
}
$got = (Get-FileHash $fontZip -Algorithm SHA256).Hash.ToLower()
if ($got -ne $want) { Remove-Item $fontZip; throw "MapleMono-NF-CN.zip sha256 $got, the release says $want" }
Write-Host "font zip sha256 verified ($want)"
$fonts = Join-Path $out "fonts"
New-Item -ItemType Directory -Force $fonts | Out-Null
Add-Type -AssemblyName System.IO.Compression.FileSystem
$z = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path $fontZip))
try {
    foreach ($style in "Regular", "Bold") {
        $e = $z.Entries | Where-Object { $_.Name -eq "MapleMono-NF-CN-$style.ttf" } | Select-Object -First 1
        if (-not $e) { throw "MapleMono-NF-CN-$style.ttf not in the zip" }
        [System.IO.Compression.ZipFileExtensions]::ExtractToFile($e, (Join-Path $fonts $e.Name), $true)
    }
} finally { $z.Dispose() }
$ofl = Join-Path $cache "OFL.txt"
if (-not (Test-Path $ofl)) { Invoke-WebRequest "https://raw.githubusercontent.com/subframe7536/maple-font/$MapleTag/OFL.txt" -OutFile $ofl -UseBasicParsing }
if (-not (Select-String -Path $ofl -Pattern "SIL OPEN FONT LICENSE" -Quiet)) { throw "OFL.txt does not look like the SIL Open Font License" }
Copy-Item $ofl (Join-Path $fonts "OFL.txt")
$zip = "$out.zip"
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $out "*") -DestinationPath $zip

$size = [math]::Round((Get-Item (Join-Path $out $exe)).Length / 1MB, 1)
if ($GOOS -eq "windows" -and $env:OS -eq "Windows_NT") { & (Join-Path $out $exe) version }
Write-Host ("dist: {0} ({1:N1} MB; {2} {3} MB)" -f $zip, ((Get-Item $zip).Length / 1MB), $exe, $size)
Add-Type -AssemblyName System.IO.Compression.FileSystem
$z = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path $zip))
try { $z.Entries | ForEach-Object { "  {0,-40} {1,12:N0}" -f $_.FullName, $_.Length } } finally { $z.Dispose() }
