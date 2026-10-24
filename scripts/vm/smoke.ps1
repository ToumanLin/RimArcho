# One-step smoke test in the Player-Test VM: build, push, check the CLI in the guest,
# launch, check the real window over DevTools. Prints only PASS / FAIL lines (and SHOT
# with -Shot); exit code = number of failures. Cheap to read: no screenshot unless asked.
#
#   pwsh scripts/vm/smoke.ps1                 # everything
#   pwsh scripts/vm/smoke.ps1 -NoBuild        # reuse what is already in the guest
#   pwsh scripts/vm/smoke.ps1 -Shot page.png  # also one screenshot of the window
#
# Needs: the VM running, 'vm.ps1 prep' and 'vm.ps1 forward' done once, Player signed in
# on the guest screen (for the window part). See README.md.
param(
    [switch]$NoBuild,
    [string]$Shot = ''
)
$ErrorActionPreference = 'Stop'
$vm = Join-Path $PSScriptRoot 'vm.ps1'
$transfer = 'D:\VMs\Transfer\rimarcho'
$script:fails = 0
function Report([bool]$ok, [string]$name, [string]$why = '') {
    if (-not $ok) { $script:fails++ }
    $line = ($(if ($ok) { 'PASS' } else { 'FAIL' })) + " $name"
    if (-not $ok -and $why) { $line += " ($why)" }
    Write-Host $line
}
function Step([string]$name, [scriptblock]$do) {
    try { & $do *> $null; Report $true $name; return $true }
    catch { Report $false $name (($_.Exception.Message -replace '\x1b\[[0-9;]*m', '') -split "`n")[0]; return $false }
}

# The VM and its Tools; nothing else works without them.
$tools = try { (& 'D:\VMs\Setup\Invoke-VMRun.ps1' -VMX 'D:\VMs\Player-Test\Win11-Player-Test.vmx' -Command checkToolsState) 2>$null | Select-Object -Last 1 } catch { 'error' }
Report ($tools -eq 'running') 'vm: tools running' "tools: $tools"
if ($tools -ne 'running') { exit $script:fails }

if (-not $NoBuild) {
    if (-not (Step 'build' { & $vm build })) { exit $script:fails }
    if (-not (Step 'push' { & $vm push })) { exit $script:fails }
}

# CLI in the guest, as Player (the same user as the window): one round trip, the guest
# script prints its own PASS / FAIL lines.
$cli = Join-Path $transfer 'smoke-cli.ps1'
@'
$d = 'C:\Users\Public\Documents\VM-Test\RimArcho'
function Check($ok, $name, $why) { if ($ok) { "PASS cli: $name" } else { "FAIL cli: $name ($why)" } }
$v = & "$d\archo.exe" version 2>&1 | Out-String
Check ($LASTEXITCODE -eq 0 -and $v -match 'archo 0\.0\.0-vm') 'version' (($v -split "`n")[0])
$det = & "$d\archo.exe" env detect 2>&1 | Out-String
Check ($LASTEXITCODE -eq 0 -and $det -match 'game:\s+\S') 'env detect finds the game' (($det -split "`n")[0])
$list = & "$d\archo.exe" env list 2>&1 | Out-String
if ($list -match 'environments: 0') {
  $add = & "$d\archo.exe" env add Steam --detected 2>&1 | Out-String
  Check ($LASTEXITCODE -eq 0) 'env add (first run)' (($add -split "`n")[0])
}
$scan = & "$d\archo.exe" scan 2>&1 | Out-String
Check ($LASTEXITCODE -eq 0) 'scan' (($scan.Trim() -split "`n")[-1])
$st = & "$d\archo.exe" status 2>&1 | Out-String
Check ($LASTEXITCODE -eq 0) 'status' (($st.Trim() -split "`n")[0])
'@ | Set-Content -Encoding UTF8 $cli
try {
    $lines = @((& $vm exec $cli 2>$null) | ForEach-Object { $_.TrimStart([char]0xFEFF) } | Where-Object { $_ -match '^(PASS|FAIL) ' })
    if (-not $lines) { Report $false 'cli: guest script' 'no result lines; run vm.ps1 exec on it to see why' }
    $lines | ForEach-Object {
        if ($_ -like 'FAIL*') { $script:fails++ }
        Write-Host $_
    }
} catch { Report $false 'cli: guest script' (($_.Exception.Message -replace '\x1b\[[0-9;]*m', '') -split "`n")[0] }

# The window: needs someone signed in on the guest screen.
if (-not (Step 'launch' { & $vm launch })) {
    Write-Host 'FAIL gui: skipped (is Player signed in on the guest screen?)'
    exit ($script:fails + 1)
}
$ip = (& $vm ip 2>$null | Select-Object -Last 1)
$addr = "${ip}:9223"
$up = $false
foreach ($i in 1..40) {
    try { Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 "http://$addr/json/version" *> $null; $up = $true; break } catch { Start-Sleep -Milliseconds 500 }
}
Report $up 'gui: DevTools reachable' "nothing on $addr after 20 s (run 'vm.ps1 forward')"
if ($up) {
    $a = @((Join-Path $PSScriptRoot 'smoke.mjs'), $addr)
    if ($Shot) { $a += $Shot }
    node @a | ForEach-Object { if ($_ -like 'FAIL*') { $script:fails++ }; Write-Host $_ }
}
Write-Host ("{0} {1} failed" -f ($(if ($script:fails) { 'FAIL' } else { 'PASS' })), $script:fails)
exit $script:fails
