# Host-only packaging and optional execution. -BundleOnly never contacts VMware.
# Test VM only; no power/snapshot/revert operations and no credential access here.
[CmdletBinding()]
param(
 [ValidateSet('crawl','walk')][string]$Mode = 'crawl',
 [int]$Seed = 271027,
 [int]$Steps = 200,
 [ValidateRange(1,20)][int]$SeedCount = 1,
 [double]$Minutes = 60,
 [ValidateSet('safe','local','external')][string]$Allow = 'safe',
 [string]$GuestBaseline,
 [string]$ReplayTrace,
 [string]$PreviousLedger,
 [string]$PowerShellZip,
 [string]$NodeExe = (Get-Command node).Source,
 [string]$SQLiteExe = (Get-Command sqlite3).Source,
 [string]$TransferRoot = 'D:\RimArcho-test\transfer',
 [string]$GuestRoot = 'C:\Users\Public\Documents\VM-Test',
 [ValidateSet('true','false')][string]$Shrink = 'false',
 [switch]$Probe,
 [switch]$StandardUser,
 [switch]$BundleOnly
)
$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'Run this host script with PowerShell 7' }
if ($Steps -lt 1 -or $Minutes -le 0) { throw 'Steps and Minutes must be positive' }
if (-not [IO.Path]::GetFullPath($TransferRoot).StartsWith('D:\RimArcho-test\',[StringComparison]::OrdinalIgnoreCase)) { throw 'TransferRoot must be under D:\RimArcho-test' }
$existingBytes=(Get-ChildItem -LiteralPath 'D:\RimArcho-test' -Recurse -File -ErrorAction SilentlyContinue | Measure-Object Length -Sum).Sum
if ($existingBytes -gt 1400MB) { throw 'Host output is above 1400 MiB; no room for a new bounded bundle' }
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
if ($ReplayTrace) { Copy-Item -LiteralPath $ReplayTrace -Destination (Join-Path $bundle 'replay-input.json') }
if ($PreviousLedger) { Copy-Item -LiteralPath $PreviousLedger -Destination (Join-Path $bundle 'crawler-ledger.json') }
$manifest = @(Get-ChildItem -LiteralPath $bundle -Recurse -File | ForEach-Object { @{path=[IO.Path]::GetRelativePath($bundle,$_.FullName);sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash} })
@{build=$build;files=$manifest} | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 (Join-Path $bundle 'manifest.json')
$zip = Join-Path $transfer 'player-bundle.zip'
Compress-Archive -Path (Join-Path $bundle '*') -DestinationPath $zip
$zipHash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash
$runnerArgs = @('scripts/vm/run.mjs','--mode',$Mode,'--shrink',$Shrink,'--driver','W','--seed',[string]$Seed,'--steps',[string]$Steps,'--minutes',[string]$Minutes,'--tools',(Join-Path $guestBundle 'tools'),'--out',(Join-Path $guestRun 'results'))
if ($ReplayTrace) { $runnerArgs += @('--replay',(Join-Path $guestBundle 'replay-input.json')) }
if ($PreviousLedger) { $runnerArgs += @('--ledger',(Join-Path $guestBundle 'crawler-ledger.json')) }
if ($Probe) { $runnerArgs += @('--probe','true') }
if ($Allow -ne 'safe') { $runnerArgs += @('--allow',$Allow) }
if ($GuestBaseline) { $runnerArgs += @('--baseline',$GuestBaseline,'--reuse-content','true') }
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
 $env:RIMARCHO_VM_GUEST = '1'
 $freeBefore = (Get-PSDrive C).Free
 if ($freeBefore -lt 4GB) { throw 'guest needs 4 GB free; no content copies are made' }
 $env:PATH = '__BUNDLE__\tools;__BUNDLE__\pwsh;' + $env:PATH
 Set-Location -LiteralPath '__BUNDLE__'
 & '__BUNDLE__\tools\node.exe' --version
 if ($LASTEXITCODE) { throw 'bundled Node cannot start' }
 & '__BUNDLE__\tools\sqlite3.exe' -version
 if ($LASTEXITCODE) { throw 'bundled SQLite cannot start; supply a standalone SQLite distribution' }
 $runnerArgs = @(__ARGS__)
 $seedAt=[Array]::IndexOf($runnerArgs,'--seed')+1
 $runnerExit=0
 for($round=0;$round -lt __SEEDCOUNT__;$round++) {
 $runnerArgs[$seedAt]=[string](__SEED__+$round)
 & '__BUNDLE__\tools\node.exe' @runnerArgs *> '__RUN__\runner.log'
 $runnerExit = [Math]::Max($runnerExit,$LASTEXITCODE)
 $latest = Get-ChildItem -LiteralPath '__RUN__\results' -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
 if (-not $latest -or -not (Test-Path -LiteralPath (Join-Path $latest.FullName 'result.json'))) { throw 'runner produced no result.json' }
 $evidence = '__RUN__\evidence'
 if(__SEEDCOUNT__ -gt 1){$evidence=Join-Path $evidence ('seed-'+$runnerArgs[$seedAt])}
 New-Item -ItemType Directory -Force $evidence | Out-Null
 foreach ($name in @('result.json','ledger.json','ledger.md','trace.json','minimal-trace.json','replay.json','shrink.json')) {
  $source = Join-Path $latest.FullName $name
  if (Test-Path -LiteralPath $source) { Copy-Item -LiteralPath $source -Destination $evidence }
 }
 }
 $evidence='__RUN__\evidence'
 Set-Location -LiteralPath '__RUN__'
 Compress-Archive -Path "$evidence\*" -DestinationPath '__RUN__\evidence.zip'
 @{status='completed';runnerExit=$runnerExit;result=$latest.FullName;user=[Environment]::UserName;build=$manifest.build} | ConvertTo-Json | Set-Content -Encoding UTF8 '__RUN__\completion.json'
} catch {
 @{status='failed';error=$_.Exception.Message} | ConvertTo-Json | Set-Content -Encoding UTF8 '__RUN__\completion.json'
 throw
} finally {
 Set-Location -LiteralPath '__RUN__'
 $cleanupErrors=@()
 foreach ($owned in @('__RUN__\bundle','__RUN__\results','__ZIP__')) {
  $resolved=[IO.Path]::GetFullPath($owned)
  if (-not ($resolved.StartsWith('__RUN__\',[StringComparison]::OrdinalIgnoreCase) -or $resolved -eq '__ZIP__')) { throw 'cleanup path escaped run' }
  if (Test-Path -LiteralPath $resolved) { try { Remove-Item -LiteralPath $resolved -Recurse -Force } catch { $cleanupErrors += $_.Exception.Message } }
 }
 $retained = (Get-ChildItem -LiteralPath '__RUN__' -Recurse -File | Measure-Object Length -Sum).Sum
 $disk=@{freeBefore=$freeBefore;freeAfter=(Get-PSDrive C).Free;runRetainedBytes=$retained;baseline='__BASELINE__';gitAvailable=[bool](Get-Command git -ErrorAction SilentlyContinue);cleanupErrors=$cleanupErrors}
 $disk | ConvertTo-Json | Set-Content -Encoding UTF8 '__RUN__\disk.json'
}
'@
foreach ($pair in @(@('__ZIP__',"$GuestRoot\$runName.zip"),@('__HASH__',$zipHash),@('__BUNDLE__',$guestBundle),@('__RUN__',$guestRun),@('__ARGS__',$argsPS),@('__BASELINE__',$GuestBaseline),@('__SEEDCOUNT__',[string]$SeedCount),@('__SEED__',[string]$Seed))) { $guestScript = $guestScript.Replace($pair[0],$pair[1]) }
$scriptFile = Join-Path $transfer 'run-player.ps1'
# Create the run directory before guest execution; the script itself is BOM encoded.
$guestScript = "New-Item -ItemType Directory -Force $(Literal $guestRun) | Out-Null`r`n" + $guestScript
[IO.File]::WriteAllText($scriptFile,$guestScript,[Text.UTF8Encoding]::new($true))
@{run=$runName;build=$build;zipHash=$zipHash;mode=$Mode;seed=$Seed;steps=$Steps;minutes=$Minutes;allow=$Allow;guestBaseline=$GuestBaseline;bundleOnly=[bool]$BundleOnly;seedCount=$SeedCount} | ConvertTo-Json | Set-Content -Encoding utf8 (Join-Path $transfer 'host.json')
if ($BundleOnly) { Write-Output "Bundle prepared without contacting the VM: $transfer"; return }
# Deliberately no snapshot revert, VM start, app-wide process kill or credential read.
$vm = Join-Path $PSScriptRoot 'vm.ps1'
$vmArgs = @{Transfer=$transfer;StandardUser=$StandardUser}
try {
& $vm put $zip "$GuestRoot\$runName.zip" @vmArgs
& $vm exec $scriptFile -Interactive @vmArgs
& $vm get "$guestRun\completion.json" (Join-Path $transfer 'completion.json') @vmArgs
$completion = Get-Content -Raw -LiteralPath (Join-Path $transfer 'completion.json') | ConvertFrom-Json
if ($completion.status -ne 'completed') { throw ('Guest run failed: ' + $completion.error) }
& $vm get "$guestRun\evidence.zip" (Join-Path $transfer 'evidence.zip') @vmArgs
Expand-Archive -LiteralPath (Join-Path $transfer 'evidence.zip') -DestinationPath (Join-Path $transfer 'evidence')
& $vm get "$guestRun\disk.json" (Join-Path $transfer 'evidence/disk.json') @vmArgs
Get-ChildItem -LiteralPath (Join-Path $transfer 'evidence') -Recurse -Filter replay.json | ForEach-Object {
 $evidenceFolder=$_.Directory.FullName
 $replay=Get-Content -Raw -LiteralPath $_.FullName | ConvertFrom-Json
 $traceFile=Join-Path $evidenceFolder 'minimal-trace.json'
 if(-not(Test-Path -LiteralPath $traceFile)){$traceFile=Join-Path $evidenceFolder 'trace.json'}
 $replayArgs=@('-Mode','walk','-Seed',[string]$replay.seed,'-Steps',[string]$Steps,'-Minutes',[string]$Minutes,'-ReplayTrace',$traceFile)
 if($GuestBaseline){$replayArgs+=@('-GuestBaseline',$GuestBaseline)}
 if($Allow -ne 'safe'){$replayArgs+=@('-Allow',$Allow)}
 if($StandardUser){$replayArgs+='-StandardUser'}
 $command='pwsh '+(Literal $PSCommandPath)+' '+(($replayArgs|ForEach-Object {Literal $_})-join ' ')
 @{command=$command;seed=$replay.seed;trace=$traceFile;requires='same baseline content and matching signed-in guest account'}|ConvertTo-Json|Set-Content -Encoding utf8 (Join-Path $evidenceFolder 'host-replay.json')
}

} finally {
foreach ($owned in @($bundle,$zip)) {
 $resolved=[IO.Path]::GetFullPath($owned)
 if (-not $resolved.StartsWith([IO.Path]::GetFullPath($transfer)+[IO.Path]::DirectorySeparatorChar)) { throw 'host cleanup escaped transfer' }
 Remove-Item -LiteralPath $resolved -Recurse -Force
}
}
Write-Output "Guest exit $($completion.runnerExit); evidence: $transfer\evidence"
if ($completion.runnerExit) { exit $completion.runnerExit }
