# Eval matrix on one or more VMs: run -> summarize -> grade for every cell (<case>-<group>-<run>).
#   pwsh matrix.ps1 [-VMs <vmx>,<vmx>,...] [-Runs 1,2,3] [-Cases c1,...] [-Groups A,B] [-Minutes 15] [-Archo <exe>]
# One worker per VM takes the next free cell; a cell is taken by creating results/<cell>/claim
# (atomic), so no two VMs run the same cell. Grading starts the real game on the host, so only
# one worker grades at a time. A cell with grade.json is skipped, so the matrix can be restarted.
# Logs: <root>/matrix.log (one line per run), <root>/matrix-detail-<vm>.log.
param([string[]]$VMs = @('D:\VMs\Player-Test\Win11-Player-Test.vmx'),
  [string[]]$Runs = @('1', '2', '3'), [string[]]$Cases = @('h1', 'h2', 'h5'), [string[]]$Groups = @('A', 'B'),
  [int]$Minutes = 15, [string]$Archo = 'D:\RimArcho-test\mcp-eval\tools\archo.exe',
  [string]$Worker = '')   # internal: the VM this process works on
$ErrorActionPreference = 'Stop'
# pwsh -File passes a list as one string ("1,2,3"): split every list
function Split-List($l) { @($l | ForEach-Object { "$_" -split ',' } | Where-Object { $_ }) }
$Runs = Split-List $Runs; $Cases = Split-List $Cases; $Groups = Split-List $Groups; $VMs = Split-List $VMs
# a mutex left by a killed process is abandoned: it is ours now, carry on
function Lock($name) { $m = [Threading.Mutex]::new($false, $name); try { [void]$m.WaitOne() } catch [Threading.AbandonedMutexException] {}; $m }
$repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$root = 'D:\RimArcho-test\mcp-eval'; $log = "$root\matrix.log"
function Log($s) { $m = Lock 'Global\RimArcho-eval-matrix-log'; try { "$(Get-Date -Format 'MM-dd HH:mm:ss') $s" | Tee-Object -FilePath $log -Append } finally { $m.ReleaseMutex() } }

# Cells in a fixed order; which group goes first alternates, so neither group always runs later.
$cells = foreach ($n in $Runs) { for ($i = 0; $i -lt $Cases.Count; $i++) {
  $g = if (([int]$n + $i) % 2) { $Groups[($Groups.Count - 1)..0] } else { $Groups }
  foreach ($x in $g) { "$($Cases[$i])-$x-$n" } } }

if (-not $Worker) {
  # keep the host awake while the matrix runs (a power request; no setting changes)
  Add-Type -Namespace W -Name P -MemberDefinition '[DllImport("kernel32.dll")] public static extern uint SetThreadExecutionState(uint f);'
  [void][W.P]::SetThreadExecutionState([uint32]2147483649)   # ES_CONTINUOUS | ES_SYSTEM_REQUIRED (0x80000001)
  # claims of cells that never got a grade are left from an interrupted matrix: free them
  foreach ($c in $cells) { $d = "$root\results\$c"; if ((Test-Path "$d\claim") -and -not (Test-Path "$d\grade.json")) { Remove-Item "$d\claim" -Force } }
  Log "matrix start: $($cells.Count) cells on $($VMs.Count) VMs ($((($VMs | % { Split-Path (Split-Path $_ -Parent) -Leaf }) -join ', '))); archo $((Get-Item $Archo).LastWriteTime)"
  $procs = foreach ($vm in $VMs) {
    $a = @('-NoProfile', '-File', $PSCommandPath, '-Worker', $vm, '-Runs', ($Runs -join ','), '-Cases', ($Cases -join ','),
      '-Groups', ($Groups -join ','), '-Minutes', $Minutes, '-Archo', $Archo)
    Start-Process pwsh -ArgumentList $a -PassThru -WindowStyle Hidden
    Start-Sleep 60   # stagger the boots: three VMs reverting and booting at once is slow for all
  }
  $procs | Wait-Process
  Log 'matrix end'
  return
}

$vmName = Split-Path (Split-Path $Worker -Parent) -Leaf
$detail = "$root\matrix-detail-$vmName.log"
if (-not (Test-Path $Worker)) { Log "FAIL worker ${vmName}: no VM at $Worker"; return }
$failed = 0
foreach ($c in $cells) {
  $r = "$root\results\$c"
  if (Test-Path "$r\grade.json") { continue }
  New-Item -ItemType Directory -Force $r | Out-Null
  try { [IO.File]::Open("$r\claim", 'CreateNew').Close(); Set-Content "$r\claim" $vmName } catch { continue }   # another VM has it
  $case, $g, $n = $c -split '-'
  Log "start $c on $vmName"
  "===== $c $vmName $(Get-Date)" | Add-Content $detail
  try { & pwsh -NoProfile -File "$PSScriptRoot\run-case.ps1" -Case $case -Group $g -Run $n -Minutes $Minutes -Archo $Archo -VMX $Worker *>> $detail }
  catch { "run-case threw: $_" | Add-Content $detail }
  if (-not (Test-Path "$r\transcript.jsonl")) {
    # free the cell for another VM; a VM that fails twice in a row is broken, so stop its worker
    Remove-Item "$r\claim" -Force -ErrorAction SilentlyContinue
    Log "FAIL $c on ${vmName}: no transcript (see matrix-detail-$vmName.log)"
    if (++$failed -ge 2) { Log "FAIL worker ${vmName}: stopped after 2 failed runs in a row"; return }
    continue
  }
  $failed = 0
  Push-Location $repo
  try {
    & node "$PSScriptRoot\summarize.mjs" $r *>> $detail
    $gl = Lock 'Global\RimArcho-eval-grade'   # one host game at a time
    try { & node "$PSScriptRoot\grade.mjs" $r *>> $detail } finally { $gl.ReleaseMutex() }
  } finally { Pop-Location }
  try {
    $s = Get-Content "$r\summary.json" -Raw | ConvertFrom-Json; $gr = Get-Content "$r\grade.json" -Raw | ConvertFrom-Json
    $ar = ($s.tools.PSObject.Properties | Where-Object Name -like 'mcp__archo__*' | Measure-Object Value -Sum).Sum
    Log ("done  {0} on {9}: {1}s turns {2} calls {3} (archo {4}) tokens {5:N2}M culprit {6} fix {7} finished {8}" -f $c, $s.seconds, $s.turns, $s.toolCalls, [int]$ar, ($s.usage.total / 1e6), $gr.culprit, $gr.fix, $s.finished, $vmName)
  } catch { Log "done  $c on $vmName (no summary/grade: $_)" }
}
