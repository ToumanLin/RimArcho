# One eval run, from the host: revert the VM, stage the case, run Pi, collect the evidence, stop the VM.
#   pwsh run-case.ps1 -Case c1 -Group A -Run 1 [-Minutes 15] [-Archo D:\RimArcho-test\mcp-eval\tools\archo.exe] [-StageOnly]
# Group A = Pi with the Archo MCP server, B = dsh with it (since 2026-11-04), N = Pi without it (B before 2026-11-04). Results: D:\RimArcho-test\mcp-eval\results\<case>-<group>-<run>\
# -Archo replaces the snapshot's archo.exe with this build; -StageOnly stops after staging and leaves the VM running.
param([Parameter(Mandatory)][string]$Case, [Parameter(Mandatory)][ValidateSet('A', 'B', 'N')][string]$Group, [int]$Run = 1, [int]$Minutes = 15,
  [string]$Snapshot = 'RimArcho-Eval-Staged5', [string]$Archo = '', [switch]$KeepRunning, [switch]$StageOnly,
  [string]$VMX = 'D:\VMs\Player-Test\Win11-Player-Test.vmx')
$ErrorActionPreference = 'Stop'
# -VMX picks one of the eval VMs (Player-Test, or a copy such as D:\VMs\Eval-2\Win11-Player-Test.vmx).
$vmx = [IO.Path]::GetFullPath($VMX); $vmName = Split-Path (Split-Path $vmx -Parent) -Leaf
# one run per VM at a time: a second run would revert the VM under the first and corrupt both
$lock = New-Object System.Threading.Mutex($false, "Global\RimArcho-eval-vm-$vmName")
if (-not $lock.WaitOne(0)) { throw "another eval run is using VM $vmName" }
$inv = 'D:\VMs\Setup\Invoke-VMRun.ps1'
$vr = 'C:\Program Files\VMware\VMware Workstation\vmrun.exe'
$root = 'D:\RimArcho-test\mcp-eval'; $xfer = 'D:\VMs\Transfer'
$ps5 = 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'
$out = "$root\results\$Case-$Group-$Run"
New-Item -ItemType Directory -Force $out | Out-Null
function Guest([switch]$Admin, [string[]]$Rest) {
  & $inv -VMX $vmx -Guest -StandardUser:(-not $Admin) -Command runProgramInGuest -VMArgs (@($ps5, '-NoProfile', '-ExecutionPolicy', 'Bypass') + $Rest)
}
function Put($src, $dst) { & $inv -VMX $vmx -Guest -StandardUser -Command copyFileFromHostToGuest -VMArgs @($src, $dst) }
function Pull($src, $dst) { & $inv -VMX $vmx -Guest -StandardUser -Command copyFileFromGuestToHost -VMArgs @($src, $dst) }

# 1. clean state (stop this VM only: the other eval VMs may be mid-run)
if (& $vr list | Where-Object { $_.Trim() -ieq $vmx }) { & $inv -VMX $vmx -Command stop -VMArgs @('hard') | Out-Null; Start-Sleep 5 }
& $inv -VMX $vmx -Command revertToSnapshot -VMArgs @($Snapshot)
& $inv -VMX $vmx -Command start -VMArgs @('nogui')
for ($i = 0; $i -lt 40; $i++) { if ((& $inv -VMX $vmx -Command checkToolsState) -match 'running') { break }; Start-Sleep 5 }
Start-Sleep 20

# 2. stage the case (the guest scripts are pushed every run, so the snapshot never needs refreshing for them)
foreach ($f in 'overlay.ps1', 'case.ps1', 'agent.ps1', 'task.md', 'dsh-eval.yml') { Put "$PSScriptRoot\guest\$f" "C:\Eval\$f" }
if ($Archo) { Put $Archo 'C:\Eval\tools\archo.exe'; Copy-Item $Archo "$out\archo.exe" }   # keep the evaluated build with the results
Remove-Item "$xfer\pkg-$Case-$vmName.tar" -Force -ErrorAction SilentlyContinue
& C:\Windows\System32\tar.exe -cf "$xfer\pkg-$Case-$vmName.tar" -C "$root\pkg" $Case   # not Git's tar: it mis-parses D:
if (-not (Test-Path "$xfer\pkg-$Case-$vmName.tar")) { throw 'packing the case failed' }
Put "$xfer\pkg-$Case-$vmName.tar" 'C:\Users\Public\pkg.tar'
Guest -Rest @('-Command', 'tar -xf C:\Users\Public\pkg.tar -C C:\Eval\pkg')
Guest -Admin -Rest @('-File', 'C:\Eval\overlay.ps1', '-Case', $Case)
Guest -Rest @('-File', 'C:\Eval\case.ps1', '-Case', $Case, '-Group', $Group)
Pull 'C:\Eval\case-ready.txt' "$out\case-ready.txt"
if ($Group -ne 'N') { Pull 'C:\Eval\setup-archo.log' "$out\setup-archo.log" }
# the agent must not find the staging package (its overlay names the broken files) or the staging scripts
Guest -Admin -Rest @('-Command', 'Remove-Item C:\Eval\pkg\*, C:\Users\Public\*.tar, C:\Users\Public\*.ps1, C:\Users\Public\*.txt, C:\Users\Public\vmx.*,C:\Eval\overlay.ps1, C:\Eval\case.ps1, C:\Eval\case-ready.txt, C:\Eval\setup-archo.log, C:\Eval\staged.txt, C:\Eval\w.txt -Recurse -Force -ErrorAction SilentlyContinue; exit 0')
if ($StageOnly) { "staged $Case $Group; the VM is still running"; return }

# 3. the agent run (the guest script enforces the time limit; the host waits a little longer)
$t0 = Get-Date
# -interactive: the logged-in desktop session. Without it the agent runs in session 0, where the
# game it starts (test_run or by hand) has no display and hangs at "could not switch resolution".
& $inv -VMX $vmx -Guest -StandardUser -Command runProgramInGuest -VMArgs @('-noWait', '-interactive', $ps5, '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', 'C:\Eval\agent.ps1', '-Group', $Group, '-Minutes', $Minutes)
$deadline = $t0.AddMinutes($Minutes + 3)
while ((Get-Date) -lt $deadline) {
  Start-Sleep 20
  try { Pull 'C:\Eval\out\run.json' "$out\run.json" 2>$null; if (Test-Path "$out\run.json") { break } } catch {}
}

# 4. evidence
Guest -Rest @('-Command', 'Compress-Archive -Path C:\Eval\submit\* -DestinationPath C:\Eval\submit.zip -Force; Copy-Item C:\Eval\work\TASK.md C:\Eval\out\TASK.md')
foreach ($f in 'transcript.jsonl', 'stderr.txt', 'TASK.md', 'session.v4.jsonl.zstd') { if ($f -like 'session*' -and $Group -ne 'B') { continue }; try { Pull "C:\Eval\out\$f" "$out\$f" } catch { Write-Warning "missing $f" } }
try { Pull 'C:\Eval\submit.zip' "$out\submit.zip" } catch { Write-Warning 'nothing submitted' }
if (-not $KeepRunning) { & $inv -VMX $vmx -Command stop -VMArgs @('hard') | Out-Null }
"done $Case $Group $Run -> $out"
