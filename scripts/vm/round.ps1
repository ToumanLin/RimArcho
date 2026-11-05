# One real game round in the Player-Test VM, with no screenshots and no clicks: launch from
# RimArcho, main menu, a new colony, play, save, load that save, play, main menu, delete the
# save (the guest's saves sync to the player's Steam Cloud), quit to the desktop
# (docs/release-vm-tests.md, "launch the actual game"). Archo Drive (scripts/vm/drive) plays it
# inside the game; this script places it, launches, waits, collects the evidence and checks
# the driver's steps and Archo Monitor's phases. Prints PASS / FAIL / NOTE lines; exit code =
# number of failures. See README.md, "Game rounds".
#
#   pwsh scripts/vm/round.ps1                       # the standard round, launched by `archo launch`
#   pwsh scripts/vm/round.ps1 -Launch gui           # press the launch button in the RimArcho window
#   pwsh scripts/vm/round.ps1 -Play 300 -Coverage 0.3
#   pwsh scripts/vm/round.ps1 -Script my-round.txt  # any round script (drive/src/Script.cs)
#   pwsh scripts/vm/round.ps1 -KeepMonitor          # the installed Monitor, not the bundled one
#   pwsh scripts/vm/round.ps1 -Cleanup              # only remove a driver left in the guest
#
# Needs: the VM running with Player signed in on the guest screen, RimArcho pushed
# (vm.ps1 push), and an environment and a list with Harmony in the guest's RimArcho. The
# round first runs `archo monitor install`, as the window does when it opens, so the game
# gets the Monitor this build bundles. -Launch gui also needs 'vm.ps1 launch' and
# 'vm.ps1 forward'. Evidence goes to D:\RimArcho-test\rounds\<run> (a few KB).
[CmdletBinding()]
param(
    [string]$VMX = 'D:\VMs\Player-Test\Win11-Player-Test.vmx',   # which test VM (Player-Test, Eval-2, Eval-3)
    [ValidateSet('cli', 'gui')][string]$Launch = 'cli',
    [int]$Play = 120,                 # seconds of play before the save; half that after the load
    [string]$Difficulty = 'Peaceful',
    [double]$Coverage = 0,            # planet coverage for the new world; 0 keeps the game's default
    [string]$SaveName = '',
    [string]$Script = '',
    [int]$TimeoutMinutes = 60,
    [int]$HangSeconds = 300,          # no line from the driver for this long: the game is stuck
    [switch]$NoBuild,
    [switch]$NoShot,                  # no guest screenshot when the round fails
    [switch]$KeepMonitor,             # run the installed Archo Monitor; no `archo monitor install` first
    [switch]$Cleanup
)
$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'Run this with PowerShell 7 (pwsh)' }
$vmScript = Join-Path $PSScriptRoot 'vm.ps1'
$vm = { & $vmScript @args -VMX $VMX }
$work = 'C:\Users\Public\Documents\VM-Test'
$archo = "$work\RimArcho\archo.exe"
$transfer = 'D:\RimArcho-test\transfer\round'
$runName = 'round-' + (Get-Date -Format 'yyyyMMdd-HHmmss')
$guestRun = "$work\rounds\$runName"
$evidence = "D:\RimArcho-test\rounds\$runName"
New-Item -ItemType Directory -Force $transfer | Out-Null
$script:fails = 0
function Say([string]$line) {
    if ($line -like 'FAIL*') { $script:fails++ }
    Write-Host $line
}
function Guest([string]$name, [string]$body, [switch]$Interactive) {
    $file = Join-Path $transfer "$runName-$name.ps1"
    Set-Content -Encoding UTF8 -LiteralPath $file -Value ($guestLib + "`r`n" + $body)
    $out = if ($Interactive) { & $vmScript exec $file -Interactive -VMX $VMX 2>$null } else { & $vmScript exec $file -VMX $VMX 2>$null }
    @($out) | ForEach-Object { $_.TrimStart([char]0xFEFF) }
}
function Fill([string]$s) {
    $s.Replace('__RUN__', $guestRun).Replace('__WORK__', $work).Replace('__NAME__', $runName).Replace('__ARCHO__', $archo)
}

# Shared by the guest scripts (Windows PowerShell 5.1 as Player).
$guestLib = Fill @'
$ErrorActionPreference = 'Continue'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$archo = '__ARCHO__'
# The installed Archo Monitor folders (09 §6). Only the directory `archo monitor` names:
# never a guessed default, since the driver must not land in some other install.
function MonitorDir {
  $m = & $archo monitor 2>&1 | Out-String
  if ($m -match '(?m)^\s*installed:[^\r\n]*\(([^)\r\n]+)\)\s*$') { return $Matches[1].Trim() }
  $script:monitorOut = (($m.Trim() -split "`n")[0]).Trim()
  $null
}
function MonitorFolders {
  $dir = MonitorDir
  if (-not $dir) { return @() }
  @(Get-ChildItem -LiteralPath $dir -Directory -Filter 'ArchoMonitor-*' -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch '\.archo-' })
}
function DriverFiles {
  foreach ($f in MonitorFolders) {
    foreach ($p in @('ArchoDrive.txt', '1.6\Assemblies\ArchoDrive.dll', '1.7\Assemblies\ArchoDrive.dll')) {
      $full = Join-Path $f.FullName $p
      if (Test-Path -LiteralPath $full) { $full }
    }
  }
}
function RemoveDriver {
  $left = @()
  foreach ($p in @(DriverFiles)) {
    try { Remove-Item -LiteralPath $p -Force -ErrorAction Stop } catch { $left += $p }
  }
  $left
}
# The game keeps its logs open for writing; read them without asking it to stop.
function ReadShared([string]$path) {
  if (-not $path -or -not (Test-Path -LiteralPath $path)) { return '' }
  try {
    $fs = New-Object IO.FileStream($path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::ReadWrite)
    try { (New-Object IO.StreamReader($fs, [Text.Encoding]::UTF8)).ReadToEnd() } finally { $fs.Dispose() }
  } catch { '' }
}
function Games { @(Get-Process RimWorldWin64, RimWorld -ErrorAction SilentlyContinue) }
'@

if ($Cleanup) {
    $out = Guest 'cleanup' @'
$before = @(DriverFiles).Count
$left = @(RemoveDriver)
if ($left) { "FAIL cleanup: could not remove $($left -join ', ') (is the game still running?)" } else { "PASS cleanup: removed $before driver file(s)" }
'@
    $out | Where-Object { $_ -match '^(PASS|FAIL|NOTE) ' } | ForEach-Object { Say $_ }
    exit $script:fails
}

# The VM and its Tools.
$tools = try { (& 'D:\VMs\Setup\Invoke-VMRun.ps1' -VMX $VMX -Command checkToolsState) 2>$null | Select-Object -Last 1 } catch { 'error' }
if ($tools -ne 'running') { Say "FAIL vm: tools running (tools: $tools)"; exit $script:fails }

# The driver, for both engines.
$drive = Join-Path $PSScriptRoot 'drive'
if (-not $NoBuild) {
    foreach ($gv in '1.6', '1.7') {
        $o = & dotnet build (Join-Path $drive 'ArchoDrive.csproj') -c Release -nologo -v q "-p:GameVersion=$gv" 2>&1
        if ($LASTEXITCODE) { Say "FAIL build: ArchoDrive $gv ($(($o | Select-String 'error' | Select-Object -First 1)))"; exit $script:fails }
    }
}
$stage = Join-Path $transfer "$runName-drive"
foreach ($gv in '1.6', '1.7') {
    $dll = Join-Path $drive "bin\$gv\ArchoDrive.dll"
    if (-not (Test-Path $dll)) { Say "FAIL build: $dll missing (run without -NoBuild)"; exit $script:fails }
    New-Item -ItemType Directory -Force (Join-Path $stage $gv) | Out-Null
    Copy-Item $dll (Join-Path $stage $gv)
}
$zip = Join-Path $transfer "$runName-drive.zip"
Compress-Archive -Force -Path "$stage\*" -DestinationPath $zip
Remove-Item -Recurse -Force $stage

# The round script.
if (-not $SaveName) { $SaveName = 'ArchoRound-' + (Get-Date -Format 'MMdd-HHmm') }
if ($Script) {
    $body = (Get-Content -Raw -LiteralPath $Script) -replace '(?im)^\s*out\s.*$', ''
} else {
    $new = "new difficulty=$Difficulty" + $(if ($Coverage -gt 0) { " coverage=$Coverage" } else { '' })
    $body = @"
menu
$new
play $Play
save $SaveName
load $SaveName
play $([Math]::Max(10, [int]($Play / 2)))
menu
delete $SaveName
quit
"@
}
$roundTxt = Join-Path $transfer "$runName-round.txt"
Set-Content -Encoding UTF8 -LiteralPath $roundTxt -Value ("out $guestRun`r`n" + $body.Trim() + "`r`n")
New-Item -ItemType Directory -Force $evidence | Out-Null
Copy-Item $roundTxt (Join-Path $evidence 'round.txt')
$quits = if ((($body.Trim() -split "`r?`n") | Where-Object { $_ -notmatch '^\s*(#|$)' } | Select-Object -Last 1) -match '^\s*quit\b') { '1' } else { '0' }

# Start: place the driver in every installed Archo Monitor folder (the list keeps the same
# mods), then launch. The game must start in the signed-in desktop session.
& $vm put $zip "$work\$runName-drive.zip" *> $null
& $vm put $roundTxt "$work\$runName-round.txt" *> $null
$start = (Fill @'
$run = '__RUN__'
New-Item -ItemType Directory -Force $run | Out-Null
Move-Item -Force '__WORK__\__NAME__-drive.zip' "$run\drive.zip"
Move-Item -Force '__WORK__\__NAME__-round.txt' "$run\round.txt"
Expand-Archive -Force "$run\drive.zip" "$run\drive"
if (Games) { 'FAIL start: a game is already running in the guest'; return }
$stale = @(RemoveDriver)
if ('__KEEPMON__' -ne '1') {
  # What the RimArcho window does when it opens (09 §6): install or update the bundled
  # Monitor, so the round runs the Monitor this build ships. The driver goes in after this,
  # because a development Monitor folder whose files differ is replaced.
  $inst = @((& $archo monitor install 2>&1 | Out-String).Trim() -split "`r?`n" | Where-Object { $_ -match '^\s*(monitor|bundled):' } | ForEach-Object { ($_.Trim() -replace '^monitor:\s*', '') })
  "NOTE start: archo monitor install: " + $(if ($inst) { $inst -join '; ' } else { '(nothing)' })
}
$mon = (& $archo monitor 2>&1 | Out-String)
$(if ('__KEEPMON__' -eq '1') { "kept: the installed Monitor (-KeepMonitor)`r`n" } else { '' }) + $mon | Set-Content -Encoding UTF8 "$run\monitor.txt"
$folders = MonitorFolders
if (-not $folders) {
  if ($script:monitorOut) { "FAIL start: archo monitor names no Monitor directory ($($script:monitorOut))" }
  else { 'FAIL start: no installed Archo Monitor (launch once from RimArcho, or archo monitor install)' }
  return
}
foreach ($f in $folders) {
  foreach ($gv in '1.6', '1.7') {
    $asm = Join-Path $f.FullName "$gv\Assemblies"
    if (Test-Path -LiteralPath $asm) { Copy-Item -Force "$run\drive\$gv\ArchoDrive.dll" $asm }
  }
  Copy-Item -Force "$run\round.txt" (Join-Path $f.FullName 'ArchoDrive.txt')
}
'PASS start: driver placed in ' + (($folders | ForEach-Object { $_.FullName }) -join ', ')
if ('__LAUNCH__' -eq 'cli') {
  $out = & $archo launch 2>&1 | Out-String
  $code = $LASTEXITCODE
  $out | Set-Content -Encoding UTF8 "$run\launch.txt"
  if ($code) { "FAIL start: archo launch exit $code (" + (($out.Trim() -split "`n")[-1]).Trim() + ')'; return }
  'PASS start: archo launch'
}
'@).Replace('__LAUNCH__', $Launch).Replace('__KEEPMON__', $(if ($KeepMonitor) { '1' } else { '0' }))
$started = $false
Guest 'start' $start -Interactive | Where-Object { $_ -match '^(PASS|FAIL|NOTE) ' } | ForEach-Object { Say $_; if ($_ -like 'PASS start: driver*') { $started = $true } }

try {
    if (-not $started -or $script:fails) { throw 'start failed' }
    if ($Launch -eq 'gui') {
        $ip = (& $vm ip 2>$null | Select-Object -Last 1)
        $addr = "${ip}:9223"
        $cdp = Join-Path $PSScriptRoot 'cdp.mjs'
        node $cdp $addr click '#nav [data-id=home]' *> $null
        Start-Sleep -Milliseconds 800
        $o = node $cdp $addr click '.hm-go[data-env-write=launch]' 2>&1
        if ($LASTEXITCODE) { Say "FAIL start: the launch button ($(($o | Select-Object -Last 1)); 'vm.ps1 launch' and 'vm.ps1 forward' first)"; throw 'start failed' }
        Say 'PASS start: launch button pressed'
    }

    # Watch: the driver writes a line at least every 5 s while the game runs.
    $watch = (Fill @'
$run = '__RUN__'
$log = "$run\drive.log"
$t0 = Get-Date; $deadline = $t0.AddMinutes(__TIMEOUT__)
$saw = $false; $len = -1; $changed = $t0; $doneAt = $null; $status = $null; $step = ''
while (-not $status) {
  Start-Sleep -Seconds 5
  $now = Get-Date
  $game = Games
  if ($game) { $saw = $true }
  $text = ReadShared $log
  if ($text.Length -ne $len) { $len = $text.Length; $changed = $now }
  $done = $text -match '"ev":"done"'
  if ($done -and -not $doneAt) { $doneAt = $now }
  if ($text -match '"ev":"fail"') { $status = 'fail' }
  elseif ($done -and '__QUITS__' -ne '1') { $status = 'done' }
  elseif ($saw -and -not $game) { $status = $(if ($done) { 'exited' } else { 'crashed' }) }
  elseif ($doneAt -and ($now - $doneAt).TotalSeconds -gt 180) { $status = 'no-exit' }
  elseif (-not $saw -and ($now - $t0).TotalSeconds -gt 180) { $status = 'no-game' }
  elseif ($saw -and $len -le 0 -and ($now - $t0).TotalSeconds -gt 300) {
    $status = 'no-driver'
    $n = @(DriverFiles).Count
    "NOTE watch: the game runs but ArchoDrive never started; $n driver file(s) still in the Monitor folders" + $(if ($n -eq 0) { ' (RimArcho replaced the Monitor folder at launch?)' } else { '' })
  }
  elseif ($len -gt 0 -and ($now - $changed).TotalSeconds -gt __HANG__) { $status = 'hung' }
  elseif ($now -gt $deadline) { $status = 'timeout' }
}
@{ status = $status; seconds = [int]((Get-Date) - $t0).TotalSeconds; sawGame = $saw } | ConvertTo-Json -Compress | Set-Content -Encoding UTF8 "$run\watch.json"
"STATUS $status"
'@).Replace('__TIMEOUT__', [string]$TimeoutMinutes).Replace('__HANG__', [string]$HangSeconds).Replace('__QUITS__', $quits)
    $wout = @(Guest 'watch' $watch)
    $wout | Where-Object { $_ -match '^NOTE ' } | ForEach-Object { Say $_ }
    $status = ($wout | Where-Object { $_ -match '^STATUS ' } | Select-Object -Last 1) -replace '^STATUS ', ''
    if (-not $status) { $status = 'unknown' }
    Say "NOTE watch: $status"
    if ($status -notin 'exited', 'done' -and -not $NoShot) {
        New-Item -ItemType Directory -Force $evidence | Out-Null
        & $vm shot (Join-Path $evidence 'failure.png') *> $null
        Say "NOTE screenshot of the failure: $evidence\failure.png"
    }
} catch {
    if ($_.Exception.Message -ne 'start failed') { Say "FAIL round: $($_.Exception.Message)" }
} finally {
    # Finish: stop a game that did not quit, remove the driver, collect the evidence.
    $finish = Fill @'
$run = '__RUN__'
if (-not (Test-Path -LiteralPath $run)) { 'NOTE finish: no run folder'; return }
$w = $null
if (Test-Path "$run\watch.json") { $w = Get-Content -Raw "$run\watch.json" | ConvertFrom-Json }
$start = $null
foreach ($l in ((ReadShared "$run\drive.log") -split "`r?`n")) { if ($l -match '"ev":"start"') { $start = $l | ConvertFrom-Json; break } }
$data = if ($start -and $start.data) { $start.data } else { Join-Path $env:USERPROFILE 'AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios' }
$keep = $w -and $w.status -eq 'done'  # the script left the game running on purpose
# The guest's saves sync to the player's Steam Cloud when the game exits: delete the
# round's own saves that are still there, before a game that did not quit is stopped.
foreach ($l in (Get-Content -Encoding UTF8 "$run\round.txt")) {
  $words = @((($l -replace '#.*', '').Trim() -split '\s+') | Where-Object { $_ -and $_ -notmatch '=' })
  if ($words.Count -lt 2 -or $words[0] -ne 'save') { continue }
  $name = ($words[1..($words.Count - 1)] -join ' ')
  $p = Join-Path $data "Saves\$name.rws"
  if (-not (Test-Path -LiteralPath $p)) { continue }
  if ($keep) { "NOTE finish: the game is still running with the round's save '$name'; delete it before the game quits, or Steam Cloud gets it" }
  else { Remove-Item -LiteralPath $p -Force; "NOTE finish: deleted the round's save '$name' so Steam Cloud does not get it" }
}
$killed = $false
if (Games) {
  if (-not $keep) { Games | Stop-Process -Force; $killed = $true; Start-Sleep -Seconds 3 }
}
if ($w) { $w | Add-Member -NotePropertyName killed -NotePropertyValue $killed -Force; $w | ConvertTo-Json -Compress | Set-Content -Encoding UTF8 "$run\watch.json" }
else { @{ status = 'not watched'; killed = $killed } | ConvertTo-Json -Compress | Set-Content -Encoding UTF8 "$run\watch.json" }
$left = @(RemoveDriver)
if ($left) { "FAIL finish: driver still in $($left -join ', ') (round.ps1 -Cleanup once the game is closed)" } else { 'PASS finish: driver removed' }
$mon = @((ReadShared (Join-Path $data 'ArchoMonitor.log')) -split "`r?`n" | Where-Object { $_ -match '"kind":"(session|phase|exit)"' })
$mon | Set-Content -Encoding UTF8 "$run\monitor.jsonl"
$player = if ($start -and $start.playerLog) { $start.playerLog } else { '' }
if (-not $player -and $mon) { try { $player = ($mon[0] | ConvertFrom-Json).context.playerLog } catch { } }
if (-not $player) { $player = Join-Path $data 'Player.log' }
@((ReadShared $player) -split "`r?`n" | Where-Object { $_.Contains('[Archo Monitor]') }) | Set-Content -Encoding UTF8 "$run\player-phases.txt"
# What RimArcho makes of the run: the checks in docs/release-vm-tests.md read these.
& $archo problems 2>&1 | Out-File -Encoding UTF8 "$run\problems.txt"
& $archo log 2>&1 | Out-File -Encoding UTF8 "$run\log.txt"
Remove-Item -Recurse -Force "$run\drive", "$run\drive.zip" -ErrorAction SilentlyContinue
$files = @(Get-ChildItem -LiteralPath $run -File | Where-Object { $_.Name -ne 'evidence.zip' } | ForEach-Object { $_.FullName })
Compress-Archive -Force -Path $files -DestinationPath "$run\evidence.zip"
'PASS finish: evidence collected'
'@
    Guest 'finish' $finish | Where-Object { $_ -match '^(PASS|FAIL|NOTE) ' } | ForEach-Object { Say $_ }
    New-Item -ItemType Directory -Force $evidence | Out-Null
    try {
        & $vm get "$guestRun\evidence.zip" (Join-Path $evidence 'evidence.zip') *> $null
        Expand-Archive -Force (Join-Path $evidence 'evidence.zip') $evidence
        Remove-Item (Join-Path $evidence 'evidence.zip')
    } catch { Say "FAIL evidence: could not fetch $guestRun\evidence.zip ($($_.Exception.Message))" }
    Remove-Item -Force $zip, $roundTxt -ErrorAction SilentlyContinue
}

if (Test-Path (Join-Path $evidence 'drive.log')) {
    node (Join-Path $PSScriptRoot 'round-check.mjs') $evidence | ForEach-Object { if ($_ -notmatch '^(PASS|FAIL) round:') { Say $_ } }
}
Say "NOTE evidence: $evidence (problems.txt and log.txt are what RimArcho says about the run)"
Write-Host ("{0} round: {1} failed" -f $(if ($script:fails) { 'FAIL' } else { 'PASS' }), $script:fails)
exit $script:fails
