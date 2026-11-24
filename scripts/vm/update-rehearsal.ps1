# Update rehearsal (17 §5): a stable release is published only after the candidate ZIP has
# updated itself, in the Player-Test VM, to a rehearsal build of the same commit stamped one
# patch version higher. One command; exit code = number of failed checks.
#
#   pwsh scripts/vm/update-rehearsal.ps1 -Candidate dist/archo-1.1.2-windows-amd64.zip
#   pwsh scripts/vm/update-rehearsal.ps1 -Candidate <zip> -Reuse       # the VM is already up from a rehearsal: no revert
#   pwsh scripts/vm/update-rehearsal.ps1 -Candidate <zip> -Surfaces window
#
# What it does:
#  1. Reads the candidate's version and commit (its archo.exe, with an empty data folder).
#  2. Builds the rehearsal ZIP: the candidate's files, with archo.exe and RimArcho.exe built
#     again from that commit (git archive, release.ps1's ldflags and winres) stamped
#     X.Y.(Z+1). It goes to -Work, never to dist/, and is never published.
#  3. Reverts the VM to -Snapshot (cold boot, autologon), closes the guest's Steam, and
#     installs the candidate into three program folders (window, TUI, CLI).
#  4. Serves the rehearsal ZIP in the guest as a GitHub-style release (releaseserver.exe on
#     127.0.0.1; the candidate runs with RIMARCHO_UPDATE_API pointing at it), so each
#     surface runs a player's update: check, download, checksum, extract, replace, verify,
#     restart. Only the API root differs; the Monitor item and Lanzou are asked as usual and
#     name older versions.
#  5. Window: settings › check for updates › install update › confirm, clicked over DevTools
#     (update-rehearsal.mjs). TUI: ":update", Enter, through the real-keyboard agent.
#     CLI: archo update --from <rehearsal zip>.
#  6. Checks (PASS/FAIL): the task ends in "restarting"; the old window is gone; the new one
#     reports the rehearsal version; archo ops --op update lists it with the program folder;
#     after the next start .archo-update/ holds only lock and files.json; logs/gui.log has no
#     panic. A pass writes <candidate dir>/update-rehearsal-<version>.json, which
#     scripts/publish-github.ps1 requires for a stable release.
#
# Rules (scripts/vm/README.md, 17 §4): the Player-Test VM only; the snapshot is never
# changed or deleted; the game is never started; nothing from the guest is copied to the
# host but this rehearsal's logs.
param(
    [Parameter(Mandatory = $true)][string]$Candidate,
    [string]$VMX = 'D:\VMs\Player-Test\Win11-Player-Test.vmx',
    [string]$Snapshot = 'RimArcho-Baseline',
    [string]$Work = 'D:\RimArcho-test\update-rehearsal',
    [ValidateSet('window', 'tui', 'cli')][string[]]$Surfaces = @('window', 'tui', 'cli'),
    [switch]$Reuse,          # use the running VM as it is (no revert); for iterating on this script
    [switch]$NoRecord        # do not write the pass record next to the candidate
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$vmScript = Join-Path $PSScriptRoot 'vm.ps1'
$wrapper = 'D:\VMs\Setup\Invoke-VMRun.ps1'
$transfer = Join-Path 'D:\RimArcho-test\transfer\rimarcho' (Split-Path -Leaf (Split-Path -Parent $VMX))
$G = 'C:\Users\Public\Documents\VM-Test\Rehearsal'
$port = 8471
$t0 = Get-Date
$script:fails = 0
$script:lines = @()
function Say([string]$line) {
    $line = $line.TrimStart([char]0xFEFF)
    if ($line -like 'FAIL*') { $script:fails++ }
    if ($line -match '^(PASS|FAIL) ') { $script:lines += $line }
    Write-Host $line
    Add-Content -LiteralPath $script:log -Value $line -Encoding UTF8
}
function Check([bool]$ok, [string]$name, [string]$why = '') { Say ("{0} {1}{2}" -f $(if ($ok) { 'PASS' } else { 'FAIL' }), $name, $(if (-not $ok -and $why) { " ($why)" } else { '' })) }
function VM { & $vmScript @args -VMX $VMX }
# Guest runs a script and prints its PASS/FAIL/NOTE lines; -Interactive runs it on Player's desktop.
function Guest([string]$name, [string]$body, [switch]$Interactive) {
    $f = Join-Path $runDir "$name.ps1"
    Set-Content -LiteralPath $f -Value $body -Encoding UTF8
    $out = @(& $vmScript exec $f -Interactive:$Interactive -VMX $VMX 2>&1 | ForEach-Object { "$_".TrimStart([char]0xFEFF) })
    $out | Where-Object { $_ -match '^(PASS|FAIL|NOTE|INFO) ' } | ForEach-Object { Say $_ }
    return $out
}
function Value($out, [string]$key) { ($out | Where-Object { $_ -like "$key=*" } | Select-Object -Last 1) -replace "^$key=", '' }

# --- 1. the candidate -----------------------------------------------------------------
$Candidate = (Resolve-Path -LiteralPath $Candidate).Path
if ((Split-Path -Leaf $Candidate) -notmatch '^archo-(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?-windows-amd64\.zip$') { throw "$Candidate is not a release ZIP (archo-X.Y.Z-windows-amd64.zip)" }
$cv = "$($Matches[1]).$($Matches[2]).$($Matches[3])$($Matches[4])"
$rv = "$($Matches[1]).$($Matches[2]).$([int]$Matches[3] + 1)"
$runDir = Join-Path $Work ("{0}-{1}" -f $cv, (Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Force $runDir, $transfer | Out-Null
$script:log = Join-Path $runDir 'rehearsal.log'
Say "INFO candidate $Candidate ($cv), rehearsal $rv, evidence in $runDir"
$sum = "$Candidate.sha256"
$zipHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Candidate).Hash.ToLowerInvariant()
if (Test-Path -LiteralPath $sum) {
    Check ((Get-Content -Raw -LiteralPath $sum).Trim() -eq "$zipHash  $(Split-Path -Leaf $Candidate)") 'candidate: matches its .sha256'
}
$cand = Join-Path $runDir 'candidate'
Expand-Archive -LiteralPath $Candidate -DestinationPath $cand
$empty = Join-Path $runDir 'empty-home'
New-Item -ItemType Directory -Force $empty | Out-Null
# An empty data folder and no window, as selfupdate.VersionOf runs a new program.
$env:RIMARCHO_HOME = $empty; $env:RIMARCHO_CHILD = '1'
try { $ver = (& (Join-Path $cand 'archo.exe') version | Select-Object -First 1) } finally { Remove-Item Env:RIMARCHO_HOME, Env:RIMARCHO_CHILD }
if ($ver -notmatch "^archo $([regex]::Escape($cv)) \(([0-9a-f]{12})(-dirty)?, ") { throw "the candidate's archo.exe says '$ver', not $cv" }
$commit = $Matches[1]
if ($Matches[2]) { throw "the candidate was built from a dirty tree ($ver); rebuild it from a commit" }
Say "INFO candidate commit $commit"

# --- 2. the rehearsal build ------------------------------------------------------------
Push-Location $repo
try {
    git cat-file -e "$commit^{commit}" 2>$null
    if ($LASTEXITCODE) { throw "commit $commit is not in this repository; fetch it first" }
    $src = Join-Path $runDir 'src'
    New-Item -ItemType Directory -Force $src | Out-Null
    git archive --format=tar -o (Join-Path $runDir 'src.tar') $commit
    if ($LASTEXITCODE) { throw 'git archive failed' }
    & "$env:SystemRoot\System32\tar.exe" -xf (Join-Path $runDir 'src.tar') -C $src  # not Git Bash's tar (D: is a host name there)
    if ($LASTEXITCODE) { throw 'tar failed' }
    Remove-Item -LiteralPath (Join-Path $runDir 'src.tar')
    # releaseserver: built from this checkout (test tooling, not part of the candidate)
    $env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
    go build -o (Join-Path $runDir 'releaseserver.exe') ./scripts/vm/releaseserver
    if ($LASTEXITCODE) { throw 'releaseserver build failed' }
} finally { Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue; Pop-Location }
$stage = Join-Path $runDir 'rehearsal'
Copy-Item -Recurse -LiteralPath $cand $stage
Push-Location $src
try {
    # scripts/release.ps1's executables, with the rehearsal version and the candidate's commit.
    $pkg = 'github.com/ToumanLin/RimArcho/internal/buildinfo'
    $ld = "-s -w -X $pkg.Version=$rv -X $pkg.Commit=$commit -X $pkg.Date=$((Get-Date).ToUniversalTime().ToString('yyyy-MM-dd'))"
    $syso = 'cmd/archo/rsrc_windows_amd64.syso'
    foreach ($b in @(@('archo.exe', 'RimArcho command line', ''), @('RimArcho.exe', 'RimArcho', " -H windowsgui -X $pkg.GUI=1"))) {
        go run ./scripts/winres -version $rv -exe $b[0] -desc $b[1] -o $syso
        if ($LASTEXITCODE) { throw 'winres failed' }
        $env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
        go build -trimpath -ldflags "$ld$($b[2])" -o (Join-Path $stage $b[0]) ./cmd/archo
        $code = $LASTEXITCODE
        Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
        if ($code) { throw "build of $($b[0]) failed" }
    }
} finally { Pop-Location }
Remove-Item -Recurse -Force -LiteralPath $src
$serve = Join-Path $runDir 'serve'
New-Item -ItemType Directory -Force $serve | Out-Null
$rzipName = "archo-$rv-windows-amd64.zip"
$rzip = Join-Path $serve $rzipName
Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $rzip
[IO.File]::WriteAllText("$rzip.sha256", "$((Get-FileHash -Algorithm SHA256 $rzip).Hash.ToLowerInvariant())  $rzipName`n", [Text.Encoding]::ASCII)
$env:RIMARCHO_HOME = $empty; $env:RIMARCHO_CHILD = '1'
try { $rver = (& (Join-Path $stage 'archo.exe') version | Select-Object -First 1) } finally { Remove-Item Env:RIMARCHO_HOME, Env:RIMARCHO_CHILD }
Check ($rver -match "^archo $([regex]::Escape($rv)) \($commit, ") "rehearsal build: $rv at $commit" $rver

# --- 3. the VM -------------------------------------------------------------------------
$running = @(& 'C:\Program Files\VMware\VMware Workstation\vmrun.exe' list) | Where-Object { $_.Trim() -eq [IO.Path]::GetFullPath($VMX) }
if (-not $Reuse) {
    if ($running) { throw "the VM is running: another test may be using it. Stop it, or pass -Reuse to rehearse in it as it is" }
    Say "INFO reverting to $Snapshot and starting the VM"
    & $wrapper -VMX $VMX -Command revertToSnapshot -VMArgs @($Snapshot)
    & $wrapper -VMX $VMX -Command start -VMArgs @('gui')
} elseif (-not $running) { throw '-Reuse: the VM is not running' }
$ready = $false
foreach ($i in 1..60) {
    $tools = try { (& $wrapper -VMX $VMX -Command checkToolsState 2>$null) | Select-Object -Last 1 } catch { '' }
    if ($tools -eq 'running') { $ready = $true; break }
    Start-Sleep -Seconds 5
}
if (-not $ready) { Check $false 'vm: Tools running' 'not after 5 minutes'; exit $script:fails }
# The desktop: autologon signs Player in; an interactive script runs once it has.
$desk = $false
foreach ($i in 1..30) {
    try {
        $o = Guest 'desk' "if (Get-Process explorer -ErrorAction SilentlyContinue) { 'desk=up' }" -Interactive
        if ((Value $o 'desk') -eq 'up') { $desk = $true; break }
    } catch { }
    Start-Sleep -Seconds 5
}
Check $desk 'vm: Player signed in on the guest desktop' 'interactive scripts fail: is the screen locked?'
if (-not $desk) { exit $script:fails }
VM prep *> $null
VM forward *> $null
$ip = (VM ip | Select-Object -Last 1)
Say "INFO guest $ip"

# Files into the guest.
foreach ($d in $G, "$G\serve") { try { & $wrapper -VMX $VMX -Guest -Command createDirectoryInGuest -VMArgs @($d) *> $null } catch { } } # already there
VM put $Candidate "$G\candidate.zip"
VM put $rzip "$G\serve\$rzipName"
VM put "$rzip.sha256" "$G\serve\$rzipName.sha256"
VM put (Join-Path $runDir 'releaseserver.exe') "$G\releaseserver.exe"

# Setup: Steam closed (no game runs, but its sessions must not compete with the owner's),
# the candidate in three program folders, the release service, gui.log's length.
$setup = Guest 'setup' @"
`$G = '$G'
`$steam = Join-Path `${env:ProgramFiles(x86)} 'Steam\steam.exe'
if (Get-Process steam -ErrorAction SilentlyContinue) { if (Test-Path `$steam) { Start-Process `$steam -ArgumentList '-shutdown' }; foreach (`$i in 1..30) { if (-not (Get-Process steam -ErrorAction SilentlyContinue)) { break }; Start-Sleep 1 } }
Get-Process steam, steamwebhelper -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
if (Get-Process steam -ErrorAction SilentlyContinue) { 'FAIL setup: Steam still running' } else { 'PASS setup: the guest''s Steam is closed' }
Get-Process RimArcho, archo, releaseserver -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500
foreach (`$s in 'win', 'tui', 'cli') {
  if (Test-Path "`$G\`$s") { Remove-Item -Recurse -Force "`$G\`$s" }
  Expand-Archive -Force "`$G\candidate.zip" "`$G\`$s"
}
`$v = (& "`$G\cli\archo.exe" version 2>&1 | Select-Object -First 1)
if ("`$v" -like 'archo $cv *') { 'PASS setup: the candidate installed (' + `$v + ')' } else { 'FAIL setup: the candidate says ' + `$v }
Start-Process "`$G\releaseserver.exe" -ArgumentList '-addr', '127.0.0.1:$port', '-dir', "`$G\serve", '-log', "`$G\releaseserver.log" -WindowStyle Hidden
Start-Sleep 1
try { `$r = Invoke-RestMethod 'http://127.0.0.1:$port/releases/latest' -TimeoutSec 5; if (`$r.tag_name -eq 'v$rv') { 'PASS setup: the release service names v$rv' } else { 'FAIL setup: the release service names ' + `$r.tag_name } } catch { 'FAIL setup: the release service does not answer: ' + `$_ }
`$gl = Join-Path `$env:APPDATA 'RimArcho\logs\gui.log'
'guilog=' + `$(if (Test-Path `$gl) { (Get-Item `$gl).Length } else { 0 })
"@ -Interactive
$guiLogFrom = [int64](Value $setup 'guilog')

# Checks run in the guest for one program folder after its update.
function After([string]$surface, [int]$oldPid) {
    $null = Guest "after-$surface" @"
`$d = '$G\$surface'
`$t = Get-Date
`$v = (& "`$d\archo.exe" version 2>&1 | Select-Object -First 1)
if ("`$v" -like 'archo $rv *') { 'PASS ${surface}: the program folder holds $rv' } else { 'FAIL ${surface}: the program folder holds ' + `$v }
if ($oldPid -gt 0) {
  if (Get-Process -Id $oldPid -ErrorAction SilentlyContinue) { 'FAIL ${surface}: the old process $oldPid is still running' } else { 'PASS ${surface}: the old process is gone' }
}
`$ops = (& "`$d\archo.exe" ops --op update 2>&1 | Out-String)
`$hit = @(`$ops -split "``n" | Where-Object { `$_ -like '*$rv*' })
if (`$hit -and `$ops.ToLower().Contains(`$d.ToLower())) { 'PASS ${surface}: archo ops --op update lists $rv with ' + `$d } else { 'FAIL ${surface}: archo ops --op update: ' + ((`$ops.Trim() -split "``n" | Select-Object -First 6) -join ' / ') }
"@
}
# The next start of a program folder cleans .archo-update/ (17 §5.1 Cleanup): only lock
# and files.json may stay. -Start opens the folder's window first (the TUI and CLI case).
function Clean([string]$surface, [switch]$Start) {
    $null = Guest "clean-$surface" @"
`$d = '$G\$surface'
if (`$$([bool]$Start)) {
  `$p = Start-Process "`$d\RimArcho.exe" -WorkingDirectory `$d -PassThru
  Start-Sleep 20
} else { Start-Sleep 15 }
`$names = @(Get-ChildItem -Force -LiteralPath "`$d\.archo-update" -ErrorAction SilentlyContinue | ForEach-Object Name)
`$extra = @(`$names | Where-Object { `$_ -notin 'lock', 'files.json' })
if (`$extra) { 'FAIL ${surface}: .archo-update holds ' + (`$extra -join ', ') } else { 'PASS ${surface}: after the next start .archo-update holds only ' + ((`$names | Sort-Object) -join ', ') }
if (`$$([bool]$Start)) { Get-Process RimArcho -ErrorAction SilentlyContinue | Where-Object { `$_.Path -like "`$d\*" } | Stop-Process -Force }
"@ -Interactive
}

# --- 5a. window --------------------------------------------------------------------------
if ('window' -in $Surfaces) {
    $o = Guest 'win-start' @"
Get-Process RimArcho -ErrorAction SilentlyContinue | Stop-Process -Force
`$env:RIMARCHO_DEVTOOLS_PORT = '9222'
`$env:RIMARCHO_UPDATE_API = 'http://127.0.0.1:$port'
`$p = Start-Process '$G\win\RimArcho.exe' -WorkingDirectory '$G\win' -PassThru
'pid=' + `$p.Id
"@ -Interactive
    $winPid = [int](Value $o 'pid')
    node (Join-Path $PSScriptRoot 'update-rehearsal.mjs') "${ip}:9223" $cv $rv 2>&1 | ForEach-Object { if ($_ -match '^RESULT ') { Set-Content -LiteralPath (Join-Path $runDir 'window.json') ($_ -replace '^RESULT ', '') } else { Say "$_" } }
    After 'win' $winPid
    Clean 'win'
    Guest 'win-stop' "Get-Process RimArcho -ErrorAction SilentlyContinue | Where-Object { `$_.Path -like '$G\win\*' -or `$_.Path -like '$G\win\.archo-update\*' } | Stop-Process -Force" | Out-Null
}

# --- 5b. TUI -----------------------------------------------------------------------------
if ('tui' -in $Surfaces) {
    VM input start *> $null
    $env:RIMARCHO_VM_INPUT_TOKEN = (Get-Content -Raw (Join-Path $transfer 'input-token.txt')).Trim()
    $inp = { node (Join-Path $PSScriptRoot 'input.mjs') "${ip}:9231" @args 2>&1 }
    $o = Guest 'tui-start' @"
`$env:RIMARCHO_UPDATE_API = 'http://127.0.0.1:$port'
`$p = Start-Process '$G\tui\archo.exe' -ArgumentList 'tui' -WorkingDirectory '$G\tui' -PassThru
'pid=' + `$p.Id
"@ -Interactive
    $tuiPid = [int](Value $o 'pid')
    $term = ''
    foreach ($i in 1..20) {
        Start-Sleep -Seconds 2
        foreach ($name in 'WindowsTerminal', 'OpenConsole', 'conhost', 'archo') {
            $w = (& $inp windows $name | Out-String)
            if ($w -match '"hwnd"') { $term = $name; break }
        }
        if ($term -and ((& $inp text $term | Out-String) -match '\S')) { break }
    }
    Check ([bool]$term) 'tui: the TUI''s terminal window is up' 'no WindowsTerminal/conhost window found'
    if ($term) {
        & $inp focus $term | Out-Null
        Start-Sleep -Seconds 3
        & $inp type ':update' | Out-Null
        & $inp key enter | Out-Null
        $panel = ''
        foreach ($i in 1..15) { Start-Sleep -Seconds 2; $panel = (& $inp text $term | Out-String); if ($panel.Contains($rv)) { break } }
        Set-Content -LiteralPath (Join-Path $runDir 'tui-panel.txt') $panel -Encoding UTF8
        Check $panel.Contains($rv) "tui: the update panel offers $rv" 'see tui-panel.txt'
        if ($panel.Contains($rv)) {
            $started = Get-Date
            & $inp key enter | Out-Null
            $done = $false
            foreach ($i in 1..36) {
                Start-Sleep -Seconds 5
                $o = Guest 'tui-poll' @"
`$p = @(Get-CimInstance Win32_Process -Filter "Name='archo.exe'" | Where-Object { `$_.ExecutablePath -eq '$G\tui\archo.exe' -and `$_.ProcessId -ne $tuiPid })
'new=' + `$p.Count
"@
                if ([int](Value $o 'new') -gt 0) { $done = $true; break }
            }
            Check $done 'tui: the new TUI started in the same terminal' 'no archo.exe from the program folder after 3 minutes'
            $after = (& $inp text $term | Out-String)
            Set-Content -LiteralPath (Join-Path $runDir 'tui-after.txt') $after -Encoding UTF8
            # the old TUI waits for the new one in its terminal (restartTUI): the window stays
            $open = $after -notmatch 'no visible window'
            Check $open 'tui: the terminal stays open' 'it closed: the TUI exited (crashed?)'
            if ($open) { Check ($after -notmatch 'panic|nil pointer') 'tui: no panic on the terminal' 'see tui-after.txt' }
        }
    }
    After 'tui' 0
    Guest 'tui-stop' "Get-CimInstance Win32_Process -Filter `"Name='archo.exe'`" | Where-Object { `$_.ExecutablePath -like '$G\tui\*' } | ForEach-Object { Stop-Process -Id `$_.ProcessId -Force }" | Out-Null
    Start-Sleep -Seconds 2
    Clean 'tui' -Start
}

# --- 5c. CLI -----------------------------------------------------------------------------
if ('cli' -in $Surfaces) {
    Guest 'cli' @"
`$out = (& '$G\cli\archo.exe' update --from '$G\serve\$rzipName' 2>&1 | Out-String)
`$code = `$LASTEXITCODE
if (`$code -eq 0) { 'PASS cli: archo update --from exits 0' } else { 'FAIL cli: archo update --from exits ' + `$code + ': ' + ((`$out.Trim() -split "``n" | Select-Object -Last 3) -join ' / ') }
if (`$out -match 'panic|nil pointer') { 'FAIL cli: the output has a panic' }
"@ | Out-Null
    After 'cli' 0
    Clean 'cli' -Start
}

# --- 6. gui.log, evidence, record -----------------------------------------------------------
$o = Guest 'collect' @"
`$gl = Join-Path `$env:APPDATA 'RimArcho\logs\gui.log'
`$text = ''
if (Test-Path `$gl) { `$fs = [IO.File]::Open(`$gl, 'Open', 'Read', 'ReadWrite'); `$fs.Seek($guiLogFrom, 'Begin') | Out-Null; `$text = (New-Object IO.StreamReader(`$fs)).ReadToEnd(); `$fs.Close() }
Set-Content -Encoding UTF8 '$G\gui-log-part.txt' `$text
`$bad = @(`$text -split "``n" | Where-Object { `$_ -match 'panic|nil pointer dereference|goroutine \d+ \[' })
if (`$bad) { 'FAIL logs: gui.log has a panic: ' + `$bad[0].Trim() } else { 'PASS logs: gui.log has no panic' }
Get-Process RimArcho, archo, releaseserver -ErrorAction SilentlyContinue | Stop-Process -Force
"@ -Interactive
foreach ($f in 'gui-log-part.txt', 'releaseserver.log') { try { VM get "$G\$f" (Join-Path $runDir $f) *> $null } catch { Say "NOTE could not copy $f out of the guest" } }
if (Test-Path (Join-Path $runDir 'releaseserver.log')) {
    $dl = @(Get-Content (Join-Path $runDir 'releaseserver.log') | Where-Object { $_ -like "*GET /download/$rzipName" }).Count
    Say "INFO the release service served the rehearsal ZIP $dl times"
}
Remove-Item -Recurse -Force -LiteralPath $cand, $stage, $empty -ErrorAction SilentlyContinue  # a just-built exe may still be scanned

$min = [math]::Round(((Get-Date) - $t0).TotalMinutes, 1)
$passed = $script:fails -eq 0 -and $Surfaces.Count -eq 3
$record = [ordered]@{
    version = $cv; commit = $commit; zip = (Split-Path -Leaf $Candidate); sha256 = $zipHash; rehearsal = $rv
    date = (Get-Date).ToString('o'); passed = $passed; failed = $script:fails; checks = $script:lines; evidence = $runDir
}
$record | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $runDir 'record.json') -Encoding UTF8
if ($passed -and -not $NoRecord) {
    $rec = Join-Path (Split-Path -Parent $Candidate) "update-rehearsal-$cv.json"
    $record | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $rec -Encoding UTF8
    Say "INFO pass recorded in $rec (publish-github.ps1 requires it)"
}
$fails = $script:fails
Say ("{0} update rehearsal {1} -> {2}: {3} failed, {4} min" -f $(if ($script:fails) { 'FAIL' } else { 'PASS' }), $cv, $rv, $script:fails, $min)
exit $fails
