# Host-only packaging and optional execution. -BundleOnly never contacts VMware.
# Test VM only; no power/snapshot/revert operations and no credential access here.
[CmdletBinding()]
param(
 [ValidateSet('crawl','walk')][string]$Mode = 'crawl',
 [int]$Seed = 271027,
 [int]$Steps = 200,
 [double]$Minutes = 60,
 [ValidateSet('safe','local','external')][string]$Allow = 'safe',
 [string]$GuestBaseline,
 [string]$PowerShellZip,
 [string]$NodeExe = (Get-Command node).Source,
 [string]$SQLiteExe = (Get-Command sqlite3).Source,
 [string]$TransferRoot = 'D:\VMs\Transfer',
 [string]$GuestRoot = 'C:\Users\Public\Documents\VM-Test',
 [switch]$StandardUser,
 [switch]$BundleOnly
)
$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'Run this host script with PowerShell 7' }
if ($Steps -lt 1 -or $Minutes -le 0) { throw 'Steps and Minutes must be positive' }
if ($GuestBaseline -and -not $PowerShellZip) { throw 'Guest real-copy preparation needs a portable PowerShell 7 zip (-PowerShellZip)' }
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$build = (& git -C $repo rev-parse HEAD).Trim()
if ($LASTEXITCODE) { throw 'Cannot identify build' }
$runName = 'vm-player-' + $Mode + '-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + ([guid]::NewGuid().ToString('N').Substring(0,6))
$transfer = Join-Path $TransferRoot $runName
$bundle = Join-Path $transfer 'bundle'
$guestRun = Join-Path $GuestRoot $runName
$guestBundle = Join-Path $guestRun 'bundle'
New-Item -ItemType Directory -Force (Join-Path $bundle 'tools'),(Join-Path $bundle 'scripts/vm') | Out-Null
Push-Location $repo
try {
 foreach ($tool in @(@('archo','./cmd/archo'),@('vmstate','./spikes/vmstate'),@('RimArcho','./cmd/archo'))) {
  $buildArgs = @('build','-trimpath','-o',(Join-Path $bundle ('tools/' + $tool[0] + '.exe')))
  if ($tool[0] -eq 'RimArcho') { $buildArgs += @('-ldflags','-H windowsgui -X github.com/ToumanLin/RimArcho/internal/buildinfo.GUI=1') }
  & go @buildArgs $tool[1]
  if ($LASTEXITCODE) { throw ('Build failed: ' + $tool[0]) }
 }
} finally { Pop-Location }
Copy-Item -LiteralPath $NodeExe -Destination (Join-Path $bundle 'tools/node.exe')
Copy-Item -LiteralPath $SQLiteExe -Destination (Join-Path $bundle 'tools/sqlite3.exe')
# SQLite distributions may have a companion DLL; keep it next to the executable.
$sqliteDll = Join-Path (Split-Path $SQLiteExe) 'sqlite3.dll'
if (Test-Path -LiteralPath $sqliteDll) { Copy-Item -LiteralPath $sqliteDll -Destination (Join-Path $bundle 'tools/sqlite3.dll') }
Get-ChildItem -LiteralPath $PSScriptRoot -File | Where-Object Extension -in '.mjs','.ps1','.md' | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $bundle 'scripts/vm') }
if ($PowerShellZip) { Expand-Archive -LiteralPath $PowerShellZip -DestinationPath (Join-Path $bundle 'pwsh') }
$manifest = @(Get-ChildItem -LiteralPath $bundle -Recurse -File | ForEach-Object { @{path=[IO.Path]::GetRelativePath($bundle,$_.FullName);sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash} })
@{build=$build;files=$manifest} | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 (Join-Path $bundle 'manifest.json')
$zip = Join-Path $transfer 'player-bundle.zip'
Compress-Archive -Path (Join-Path $bundle '*') -DestinationPath $zip
$zipHash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash
$runnerArgs = @('scripts/vm/run.mjs','--mode',$Mode,'--driver','W','--seed',[string]$Seed,'--steps',[string]$Steps,'--minutes',[string]$Minutes,'--tools',(Join-Path $guestBundle 'tools'),'--out',(Join-Path $guestRun 'results'))
if ($Allow -ne 'safe') { $runnerArgs += @('--allow',$Allow) }
if ($GuestBaseline) { $runnerArgs += @('--baseline',$GuestBaseline) }
function Literal([string]$s) { "'" + $s.Replace("'","''") + "'" }
$argsPS = ($runnerArgs | ForEach-Object { Literal $_ }) -join ','
# Runs under inbox Windows PowerShell 5.1; portable pwsh is only needed for prepare-real.
$guestScript = @'
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
try {
 if ((Get-FileHash -LiteralPath '__ZIP__' -Algorithm SHA256).Hash -ne '__HASH__') { throw 'bundle hash mismatch' }
 Expand-Archive -LiteralPath '__ZIP__' -DestinationPath '__BUNDLE__' -Force
 $manifest = Get-Content -Raw -LiteralPath '__BUNDLE__\manifest.json' | ConvertFrom-Json
 foreach ($file in $manifest.files) { if ((Get-FileHash -LiteralPath (Join-Path '__BUNDLE__' $file.path) -Algorithm SHA256).Hash -ne $file.sha256) { throw ('file hash mismatch: ' + $file.path) } }
 $env:RIMARCHO_VM_BUILD = $manifest.build
 $env:PATH = '__BUNDLE__\tools;__BUNDLE__\pwsh;' + $env:PATH
 Set-Location -LiteralPath '__BUNDLE__'
 & '__BUNDLE__\tools\node.exe' --version
 if ($LASTEXITCODE) { throw 'bundled Node cannot start' }
 & '__BUNDLE__\tools\sqlite3.exe' -version
 if ($LASTEXITCODE) { throw 'bundled SQLite cannot start; supply a standalone SQLite distribution' }
 $runnerArgs = @(__ARGS__)
 & '__BUNDLE__\tools\node.exe' @runnerArgs *> '__RUN__\runner.log'
 $runnerExit = $LASTEXITCODE
 $latest = Get-ChildItem -LiteralPath '__RUN__\results' -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
 if (-not $latest -or -not (Test-Path -LiteralPath (Join-Path $latest.FullName 'result.json'))) { throw 'runner produced no result.json' }
 $evidence = '__RUN__\evidence'
 New-Item -ItemType Directory -Force $evidence | Out-Null
 foreach ($name in @('result.json','ledger.json','ledger.md','trace.json','minimal-trace.json','replay.json','shrink.json')) {
  $source = Join-Path $latest.FullName $name
  if (Test-Path -LiteralPath $source) { Copy-Item -LiteralPath $source -Destination $evidence }
 }
 Compress-Archive -Path "$evidence\*" -DestinationPath '__RUN__\evidence.zip'
 @{status='completed';runnerExit=$runnerExit;result=$latest.FullName;user=[Environment]::UserName;build=$manifest.build} | ConvertTo-Json | Set-Content -Encoding UTF8 '__RUN__\completion.json'
} catch {
 @{status='failed';error=$_.Exception.Message} | ConvertTo-Json | Set-Content -Encoding UTF8 '__RUN__\completion.json'
 throw
}
'@
foreach ($pair in @(@('__ZIP__',"$GuestRoot\$runName.zip"),@('__HASH__',$zipHash),@('__BUNDLE__',$guestBundle),@('__RUN__',$guestRun),@('__ARGS__',$argsPS))) { $guestScript = $guestScript.Replace($pair[0],$pair[1]) }
$scriptFile = Join-Path $transfer 'run-player.ps1'
# Create the run directory before guest execution; the script itself is BOM encoded.
$guestScript = "New-Item -ItemType Directory -Force $(Literal $guestRun) | Out-Null`r`n" + $guestScript
[IO.File]::WriteAllText($scriptFile,$guestScript,[Text.UTF8Encoding]::new($true))
@{run=$runName;build=$build;zipHash=$zipHash;mode=$Mode;seed=$Seed;steps=$Steps;minutes=$Minutes;allow=$Allow;guestBaseline=$GuestBaseline;bundleOnly=[bool]$BundleOnly} | ConvertTo-Json | Set-Content -Encoding utf8 (Join-Path $transfer 'host.json')
if ($BundleOnly) { Write-Output "Bundle prepared without contacting the VM: $transfer"; return }
# Deliberately no snapshot revert, VM start, app-wide process kill or credential read.
$vm = Join-Path $PSScriptRoot 'vm.ps1'
$vmArgs = @{Transfer=$transfer;StandardUser=$StandardUser}
& $vm put $zip "$GuestRoot\$runName.zip" @vmArgs
& $vm exec $scriptFile -Interactive @vmArgs
& $vm get "$guestRun\completion.json" (Join-Path $transfer 'completion.json') @vmArgs
$completion = Get-Content -Raw -LiteralPath (Join-Path $transfer 'completion.json') | ConvertFrom-Json
if ($completion.status -ne 'completed') { throw ('Guest run failed: ' + $completion.error) }
& $vm get "$guestRun\evidence.zip" (Join-Path $transfer 'evidence.zip') @vmArgs
Expand-Archive -LiteralPath (Join-Path $transfer 'evidence.zip') -DestinationPath (Join-Path $transfer 'evidence')
Write-Output "Guest exit $($completion.runnerExit); evidence: $transfer\evidence"
if ($completion.runnerExit) { exit $completion.runnerExit }
