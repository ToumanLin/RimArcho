# Builds Archo Monitor (docs/specs 17 §5, 09 §6): the same source for 1.5 and 1.6 (net472,
# Mono, Harmony 2.x) and 1.7 (net10.0, .NET 10, Harmony 3.x), then assembles the mod folder:
#
#   monitor/out/ArchoMonitor/
#     About/About.xml        modVersion = monitor/VERSION (or explicit -Version)
#     About/ModIcon.png      generated from RimArcho's icon (go run ./scripts/monitoricon)
#     About/Preview.png
#     LoadFolders.xml
#     LICENSE
#     1.5/Assemblies/ArchoMonitor.dll
#     1.6/Assemblies/ArchoMonitor.dll
#     1.7/Assemblies/ArchoMonitor.dll
#
# Game assemblies come from NuGet (Krafs.Rimworld.Ref); -RimWorldDir builds the version that
# game folder holds (read from its Version.txt) against the local game instead. Nothing is
# committed: the output is ignored by git.
#
#   pwsh monitor/build.ps1
#   pwsh monitor/build.ps1 -Version 0.9.0 -RimWorldDir D:\SteamLibrary\steamapps\common\RimWorld
param(
    [string]$Version = "",
    [string]$RimWorldDir = "",
    [string]$Out = ""
)
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
$repo = Split-Path -Parent $here
if (-not $Out) { $Out = Join-Path $here "out\ArchoMonitor" }
if (!$Version) { $Version = (Get-Content -LiteralPath (Join-Path $here 'VERSION') -Raw).Trim() }
if ($Version -notmatch '^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$') { throw "Version must look like 1.2.3 or 1.2.3-dev" }
if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) { throw "dotnet not found; install the .NET SDK 10 (17 §5)" }

$localMM = ""
if ($RimWorldDir) {
    $vt = Join-Path $RimWorldDir "Version.txt"
    if (-not (Test-Path $vt)) { throw "$vt not found: -RimWorldDir must be the game folder" }
    if ((Get-Content $vt -Raw) -match '^(\d+\.\d+)') { $localMM = $Matches[1] }
}

$proj = Join-Path $here "ArchoMonitor.csproj"
foreach ($gv in "1.5", "1.6", "1.7") {
    $a = @("build", $proj, "-c", "Release", "-nologo", "-v", "q", "-p:GameVersion=$gv", "-p:MonitorVersion=$Version")
    $from = "Krafs.Rimworld.Ref"
    if ($localMM -eq $gv) { $a += "-p:RimWorldDir=$RimWorldDir"; $from = $RimWorldDir }
    Write-Host "dotnet build ArchoMonitor for $gv ($from)"
    & dotnet @a
    if ($LASTEXITCODE -ne 0) { throw "dotnet build for $gv failed" }
}

if (Test-Path $Out) { Remove-Item -Recurse -Force $Out }
$about = Join-Path $Out "About"
New-Item -ItemType Directory -Force $about | Out-Null
$utf8 = New-Object System.Text.UTF8Encoding($false)
$xml = [System.IO.File]::ReadAllText((Join-Path $here "About\About.xml"), $utf8).Replace('$VERSION$', $Version)
[System.IO.File]::WriteAllText((Join-Path $about "About.xml"), $xml, $utf8)
Copy-Item (Join-Path $here "LoadFolders.xml") $Out
Copy-Item -LiteralPath (Join-Path $here "Languages") -Destination $Out -Recurse
Copy-Item (Join-Path $repo "LICENSE") $Out
foreach ($gv in "1.5", "1.6", "1.7") {
    $dir = Join-Path $Out "$gv\Assemblies"
    New-Item -ItemType Directory -Force $dir | Out-Null
    Copy-Item (Join-Path $here "bin\$gv\ArchoMonitor.dll") $dir
}
Push-Location $repo
try {
    go run ./scripts/monitoricon -o $about
    if ($LASTEXITCODE -ne 0) { throw "monitoricon failed" }
} finally { Pop-Location }
Write-Host "Archo Monitor $Version -> $Out"
