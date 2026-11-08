# From the host: give one eval VM ilspycmd 11.2 and take snapshot RimArcho-Eval-Staged5 (powered off) from Staged4.
#   pwsh setup-ilspy-vm.ps1 -VMX D:\VMs\Eval-2\Win11-Player-Test.vmx
# Staged4 has the dotnet-tool ilspycmd 9.1 in C:\Tools\ilspycmd (needs the guest's .NET 8). The host's ilspycmd 11.2 ships
# a self-contained win-x64 build, one exe of 22 MB with no runtime needed; it replaces the folder's contents (the machine
# PATH keeps naming the folder). Refuses a VM that is running: another session may be using it.
# Verification (guest/setup-ilspy.ps1): the version through the PATH and one type decompiled from the guest game's
# Assembly-CSharp.dll, as the administrator, as the standard user in session 0 and as the standard user on the desktop
# (the agent's session). Logs: D:\VMs\Transfer\ilspy-<vm>-<step>.log.
param([Parameter(Mandatory)][string]$VMX, [string]$From = 'RimArcho-Eval-Staged4', [string]$To = 'RimArcho-Eval-Staged5',
  [string]$Version = '11.2.0.10050',
  [string]$Exe = "$env:USERPROFILE\.dotnet\tools\.store\ilspycmd\$Version\ilspycmd.win-x64\$Version\tools\any\win-x64\ilspycmd.exe")
$ErrorActionPreference = 'Stop'
$vmx = [IO.Path]::GetFullPath($VMX); $vmName = Split-Path (Split-Path $vmx -Parent) -Leaf
$lock = New-Object System.Threading.Mutex($false, "Global\RimArcho-eval-vm-$vmName")
if (-not $lock.WaitOne(0)) { throw "another eval run is using VM $vmName" }
$inv = 'D:\VMs\Setup\Invoke-VMRun.ps1'; $vr = 'C:\Program Files\VMware\VMware Workstation\vmrun.exe'
$ps5 = 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'; $xfer = 'D:\VMs\Transfer'
function Running { [bool](& $vr list | Where-Object { $_.Trim() -ieq $vmx }) }
function Guest([switch]$Admin, [string[]]$Rest) {
  & $inv -VMX $vmx -Guest -StandardUser:(-not $Admin) -Command runProgramInGuest -VMArgs (@($ps5, '-NoProfile', '-ExecutionPolicy', 'Bypass') + $Rest)
}
function Put($src, $dst) { & $inv -VMX $vmx -Guest -StandardUser -Command copyFileFromHostToGuest -VMArgs @($src, $dst) }
function Pull($src, $dst) { & $inv -VMX $vmx -Guest -StandardUser -Command copyFileFromGuestToHost -VMArgs @($src, $dst) }
function Fail($msg) { throw "$vmName : $msg" }

if (-not (Test-Path $Exe)) { throw "ilspycmd.exe not found: $Exe" }
if (((& $Exe --version) -join ' ') -notmatch [regex]::Escape("ilspycmd: $Version")) { throw "$Exe is not ilspycmd $Version" }
$sha = (Get-FileHash $Exe -Algorithm SHA256).Hash
if (Running) { throw "$vmName is running (another session may be using it); not touching it" }
$snaps = & $inv -VMX $vmx -Command listSnapshots
if ($snaps -contains $To) { throw "$vmName already has snapshot $To" }
if ($snaps -notcontains $From) { throw "$vmName has no snapshot $From" }

& $inv -VMX $vmx -Command revertToSnapshot -VMArgs @($From)
& $inv -VMX $vmx -Command start -VMArgs @('nogui')
try {
  # checkToolsState says "installed" for these encrypted VMs even with the tools running (run-case.ps1's wait for "running"
  # just times out after 200 s): wait until the standard user can run a program instead
  $ready = $false
  for ($i = 0; $i -lt 60 -and -not $ready; $i++) { try { Guest -Rest @('-Command', 'exit 0') | Out-Null; $ready = $true } catch { Start-Sleep 5 } }
  if (-not $ready) { Fail 'the guest never became ready for the standard user' }
  Start-Sleep 15

  Put $Exe 'C:\Users\Public\ilspycmd.exe'
  Put "$PSScriptRoot\guest\setup-ilspy.ps1" 'C:\Users\Public\setup-ilspy.ps1'
  try { Guest -Admin -Rest @('-File', 'C:\Users\Public\setup-ilspy.ps1', '-Mode', 'Install', '-Tag', 'admin', '-Version', $Version, '-Sha256', $sha) } catch { "install run: $_" }
  Pull 'C:\Users\Public\ilspy-Install-admin.log' "$xfer\ilspy-$vmName-install.log"
  if ((Get-Content "$xfer\ilspy-$vmName-install.log" -Raw) -notmatch 'ilspy Install ok') { Fail "install failed (see ilspy-$vmName-install.log)" }

  # the standard user in session 0 (what vmrun gives without -interactive)
  try { Guest -Rest @('-File', 'C:\Users\Public\setup-ilspy.ps1', '-Mode', 'Verify', '-Tag', 'user', '-Version', $Version) } catch { "verify run: $_" }
  Pull 'C:\Users\Public\ilspy-Verify-user.log' "$xfer\ilspy-$vmName-verify-user.log"
  if ((Get-Content "$xfer\ilspy-$vmName-verify-user.log" -Raw) -notmatch 'ilspy Verify ok') { Fail "verify as the standard user failed (see ilspy-$vmName-verify-user.log)" }

  # the standard user on the desktop, as run-case.ps1 starts the agent (-interactive); vmrun refuses until autologon has signed in
  $launched = $false
  for ($i = 0; $i -lt 36 -and -not $launched; $i++) {
    try {
      & $inv -VMX $vmx -Guest -StandardUser -Command runProgramInGuest -VMArgs @('-noWait', '-interactive', $ps5, '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File',
        'C:\Users\Public\setup-ilspy.ps1', '-Mode', 'Verify', '-Tag', 'desktop', '-Version', $Version)
      $launched = $true
    } catch { Start-Sleep 5 }
  }
  if (-not $launched) { Fail 'the standard user never signed in on the desktop' }
  $ok = $false
  for ($i = 0; $i -lt 24 -and -not $ok; $i++) {
    Start-Sleep 5
    try { Pull 'C:\Users\Public\ilspy-Verify-desktop.log' "$xfer\ilspy-$vmName-verify-desktop.log" 2>$null } catch { continue }
    $t = Get-Content "$xfer\ilspy-$vmName-verify-desktop.log" -Raw -ErrorAction SilentlyContinue
    if ($t -match 'ilspy Verify ok') { $ok = $true } elseif ($t -match 'FAILED') { break }
  }
  if (-not $ok) { Fail "verify on the desktop failed (see ilspy-$vmName-verify-desktop.log)" }

  # drop the setup leftovers, then a clean shutdown
  Guest -Admin -Rest @('-Command', 'Remove-Item C:\Users\Public\ilspycmd.exe, C:\Users\Public\setup-ilspy.ps1, C:\Users\Public\ilspy-* -Recurse -Force -ErrorAction SilentlyContinue; exit 0')
  & $inv -VMX $vmx -Command stop -VMArgs @('soft')
  for ($i = 0; $i -lt 36; $i++) { if (-not (Running)) { break }; Start-Sleep 5 }
  if (Running) { Fail 'the guest did not shut down cleanly; no snapshot taken' }
} catch {
  # this run started the VM, so it is this run's to stop; nothing was snapshotted
  if (Running) { & $inv -VMX $vmx -Command stop -VMArgs @('hard') | Out-Null }
  throw
}
& $inv -VMX $vmx -Command snapshot -VMArgs @($To)
if ((& $inv -VMX $vmx -Command listSnapshots) -notcontains $To) { throw "$vmName : snapshot $To missing after the snapshot call" }
"$vmName -> $To ok (ilspycmd $Version, sha256 $sha)"
