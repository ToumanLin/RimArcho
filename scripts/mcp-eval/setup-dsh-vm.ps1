# From the host: give one eval VM Node 26 + dsh and take snapshot RimArcho-Eval-Staged4 (powered off) from Staged3.
#   pwsh setup-dsh-vm.ps1 -VMX D:\VMs\Eval-2\Win11-Player-Test.vmx
# Needs D:\VMs\Transfer\node-v26.13.0-x64.msi (checked against nodejs.org's SHASUMS256.txt). The guest needs npm registry access.
param([Parameter(Mandatory)][string]$VMX, [string]$From = 'RimArcho-Eval-Staged3', [string]$To = 'RimArcho-Eval-Staged4')
$ErrorActionPreference = 'Stop'
$vmx = [IO.Path]::GetFullPath($VMX); $vmName = Split-Path (Split-Path $vmx -Parent) -Leaf
$lock = New-Object System.Threading.Mutex($false, "Global\RimArcho-eval-vm-$vmName")
if (-not $lock.WaitOne(0)) { throw "another eval run is using VM $vmName" }
$inv = 'D:\VMs\Setup\Invoke-VMRun.ps1'; $vr = 'C:\Program Files\VMware\VMware Workstation\vmrun.exe'
$ps5 = 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe'
if ((& $inv -VMX $vmx -Command listSnapshots) -contains $To) { throw "$vmName already has snapshot $To" }
& $inv -VMX $vmx -Command revertToSnapshot -VMArgs @($From)
& $inv -VMX $vmx -Command start -VMArgs @('nogui')
for ($i = 0; $i -lt 40; $i++) { if ((& $inv -VMX $vmx -Command checkToolsState) -match 'running') { break }; Start-Sleep 5 }
Start-Sleep 20
$put = { param($a, $b) & $inv -VMX $vmx -Guest -StandardUser -Command copyFileFromHostToGuest -VMArgs @($a, $b) }
& $put 'D:\VMs\Transfer\node-v26.13.0-x64.msi' 'C:\Users\Public\node.msi'
& $put "$PSScriptRoot\guest\setup-dsh.ps1" 'C:\Users\Public\setup-dsh.ps1'
& $put 'D:\VMs\Transfer\clean-setup.ps1' 'C:\Users\Public\clean-setup.ps1'
try { & $inv -VMX $vmx -Guest -Command runProgramInGuest -VMArgs @($ps5, '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', 'C:\Users\Public\setup-dsh.ps1') } catch { "setup run: $_" }
& $inv -VMX $vmx -Guest -StandardUser -Command copyFileFromGuestToHost -VMArgs @('C:\Users\Public\setup-dsh.log', "D:\VMs\Transfer\setup-dsh-$vmName.log")
$log = Get-Content "D:\VMs\Transfer\setup-dsh-$vmName.log" -Raw
if ($log -notmatch 'setup-dsh ok') { & $inv -VMX $vmx -Command stop -VMArgs @('hard'); throw "$vmName setup failed (see setup-dsh-$vmName.log)" }
& $inv -VMX $vmx -Guest -Command runProgramInGuest -VMArgs @($ps5, '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', 'C:\Users\Public\clean-setup.ps1')
& $inv -VMX $vmx -Command stop -VMArgs @('soft')
for ($i = 0; $i -lt 36; $i++) { if (-not (& $vr list | Where-Object { $_.Trim() -ieq $vmx })) { break }; Start-Sleep 5 }
& $inv -VMX $vmx -Command snapshot -VMArgs @($To)
"$vmName -> $To ok"
