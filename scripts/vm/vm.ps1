# Test RimArcho in the Player-Test VM (VMware Workstation) from the host, with no clicks.
# Wraps D:\VMs\Setup\Invoke-VMRun.ps1, which holds the VM and guest credentials (never printed).
# See scripts/vm/README.md.
#
#   pwsh scripts/vm/vm.ps1 status            # VM running? Tools ready? guest IP
#   pwsh scripts/vm/vm.ps1 build             # go build both exes into dist/vm/RimArcho
#   pwsh scripts/vm/vm.ps1 push              # copy them into the guest ($GuestDir)
#   pwsh scripts/vm/vm.ps1 prep              # once per VM: no display-off or sleep while idle
#   pwsh scripts/vm/vm.ps1 forward           # once per VM: expose the DevTools port to the host
#   pwsh scripts/vm/vm.ps1 launch            # (re)start RimArcho.exe on the guest desktop, DevTools on
#   pwsh scripts/vm/vm.ps1 shot [file.png]   # screenshot of the guest screen
#   pwsh scripts/vm/vm.ps1 exec script.ps1   # run a PowerShell script in the guest, print its output
#   pwsh scripts/vm/vm.ps1 cli "scan"        # run archo.exe <args> in the guest, print its output
#   pwsh scripts/vm/vm.ps1 go                # build + push + launch
param(
    [Parameter(Position = 0, Mandatory = $true)][string]$Command,
    [Parameter(Position = 1, ValueFromRemainingArguments = $true)][string[]]$Rest = @(),
    [string]$VMX = 'D:\VMs\Player-Test\Win11-Player-Test.vmx',
    [string]$Wrapper = 'D:\VMs\Setup\Invoke-VMRun.ps1',
    [string]$Transfer = 'D:\VMs\Transfer\rimarcho',
    [string]$GuestDir = 'C:\Users\Public\Documents\VM-Test\RimArcho',
    [int]$DevToolsPort = 9222,   # inside the guest, on 127.0.0.1
    [int]$ForwardPort = 9223,    # what the host connects to
    [switch]$Interactive,       # exec: run a window runner in the signed-in desktop session
    [switch]$StandardUser        # run guest programs as 新玩家 instead of the logged-in Player
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$work = 'C:\Users\Public\Documents\VM-Test'
New-Item -ItemType Directory -Force $Transfer | Out-Null

function VM([string]$cmd, [string[]]$a = @(), [switch]$Guest) {
    if ($Guest) { & $Wrapper -VMX $VMX -Guest -StandardUser:$StandardUser -Command $cmd -VMArgs $a }
    else { & $Wrapper -VMX $VMX -Command $cmd -VMArgs $a }
}
function ToGuest([string]$from, [string]$to) { VM copyFileFromHostToGuest @($from, $to) -Guest }
function FromGuest([string]$from, [string]$to) { VM copyFileFromGuestToHost @($from, $to) -Guest }
function PS1InGuest([string]$file, [switch]$Interactive, [switch]$NoWait) {
    $name = Split-Path -Leaf $file
    # Windows PowerShell 5.1 in the guest needs a BOM for non-ASCII scripts.
    $bom = Join-Path $Transfer $name
    if ((Resolve-Path $file).Path -ne (Resolve-Path -LiteralPath $bom -ErrorAction SilentlyContinue).Path) {
        [IO.File]::WriteAllText($bom, [IO.File]::ReadAllText($file), [Text.UTF8Encoding]::new($true))
    }
    ToGuest $bom "$work\$name"
    $a = @()
    if ($Interactive) { $a += '-interactive' }
    if ($NoWait) { $a += '-noWait' }
    $a += @('C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "$work\$name")
    VM runProgramInGuest $a -Guest
}
function GuestIP { (VM getGuestIPAddress @('-wait')) | Select-Object -Last 1 }

switch ($Command) {
    'status' {
        & 'C:\Program Files\VMware\VMware Workstation\vmrun.exe' list
        Write-Host "tools: $(VM checkToolsState)"
        Write-Host "guest IP: $(GuestIP)"
    }
    'ip' { GuestIP }
    'build' {
        Push-Location $repo
        try {
            $out = Join-Path $repo 'dist\vm\RimArcho'
            New-Item -ItemType Directory -Force $out | Out-Null
            $env:CGO_ENABLED = '0'
            $commit = (git rev-parse --short=12 HEAD); if (git status --porcelain -- internal cmd) { $commit += '-dirty' }
            $ld = "-X github.com/ToumanLin/RimArcho/internal/buildinfo.Version=0.0.0-vm -X github.com/ToumanLin/RimArcho/internal/buildinfo.Commit=$commit"
            go build -ldflags $ld -o (Join-Path $out 'archo.exe') ./cmd/archo; if ($LASTEXITCODE) { throw 'build failed' }
            # Same as scripts/release.ps1: GUI=1 makes a bare RimArcho.exe open the window, not the TUI.
            go build -ldflags "$ld -H windowsgui -X github.com/ToumanLin/RimArcho/internal/buildinfo.GUI=1" -o (Join-Path $out 'RimArcho.exe') ./cmd/archo; if ($LASTEXITCODE) { throw 'build failed' }
            # The bundled font and Archo Monitor come from the newest release build, if there is one.
            $rel = Get-ChildItem (Join-Path $repo 'dist') -Directory -Filter 'archo-*-windows-amd64' -ErrorAction SilentlyContinue | Sort-Object LastWriteTime | Select-Object -Last 1
            if ($rel) { foreach ($d in 'fonts', 'monitor') { if (Test-Path (Join-Path $rel.FullName $d)) { Copy-Item -Recurse -Force (Join-Path $rel.FullName $d) $out } } }
            $zip = Join-Path $Transfer 'RimArcho.zip'
            Compress-Archive -Force -Path "$out\*" -DestinationPath $zip
            Write-Host "built $commit -> $zip ($([math]::Round((Get-Item $zip).Length/1MB,1)) MB)"
        } finally { Pop-Location }
    }
    'push' {
        $zip = Join-Path $Transfer 'RimArcho.zip'
        $hash = (Get-FileHash $zip -Algorithm SHA256).Hash
        $s = Join-Path $Transfer 'unpack.ps1'
        @"
Get-Process RimArcho, archo -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500
if ((Get-FileHash '$work\RimArcho.zip' -Algorithm SHA256).Hash -ne '$hash') { 'hash mismatch'; exit 1 }
if (Test-Path '$GuestDir') { Remove-Item -Recurse -Force '$GuestDir' }
Expand-Archive -Force '$work\RimArcho.zip' '$GuestDir'
'unpacked $hash'
"@ | Set-Content -Encoding UTF8 $s
        try { VM createDirectoryInGuest @($work) -Guest *>$null } catch { } # already there
        ToGuest $zip "$work\RimArcho.zip"
        PS1InGuest $s
        Write-Host "pushed to $GuestDir"
    }
    'forward' {
        # Admin in the guest: forward 0.0.0.0:$ForwardPort to the DevTools port and open the
        # firewall for it. Test VM only (NAT network).
        $s = Join-Path $Transfer 'forward.ps1'
        @"
netsh interface portproxy delete v4tov4 listenport=$ForwardPort listenaddress=0.0.0.0 2>`$null | Out-Null
netsh interface portproxy add v4tov4 listenport=$ForwardPort listenaddress=0.0.0.0 connectport=$DevToolsPort connectaddress=127.0.0.1
if (-not (Get-NetFirewallRule -DisplayName 'RimArcho DevTools (test)' -ErrorAction SilentlyContinue)) {
  New-NetFirewallRule -DisplayName 'RimArcho DevTools (test)' -Direction Inbound -Protocol TCP -LocalPort $ForwardPort -Action Allow -Profile Any | Out-Null
}
netsh interface portproxy show v4tov4
"@ | Set-Content -Encoding UTF8 $s
        & $Wrapper -VMX $VMX -Guest -Command copyFileFromHostToGuest -VMArgs @($s, "$work\forward.ps1")
        & $Wrapper -VMX $VMX -Guest -Command runProgramInGuest -VMArgs @('C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "$work\forward.ps1")
        Write-Host "host connects to $(GuestIP):$ForwardPort"
    }
    'prep' {
        # Once per VM (again after a snapshot revert): Windows turns the display off after 5
        # minutes idle and sleeps after 15, so screenshots go black and guest commands stall.
        # Test VM only.
        $s = Join-Path $Transfer 'prep.ps1'
        @"
foreach (`$x in 'monitor-timeout-ac','monitor-timeout-dc','standby-timeout-ac','standby-timeout-dc','hibernate-timeout-ac','hibernate-timeout-dc') { powercfg /change `$x 0 }
powercfg /setacvalueindex SCHEME_CURRENT SUB_NONE CONSOLELOCK 0
powercfg /setactive SCHEME_CURRENT
foreach (`$q in 'SUB_VIDEO VIDEOIDLE','SUB_SLEEP STANDBYIDLE') { "`$q " + ((powercfg /query SCHEME_CURRENT `$q.Split(' ')) | Select-String '0x' | Select-Object -Last 2 | ForEach-Object { `$_.Line.Split(':')[-1].Trim() }) -join ' ' }
"@ | Set-Content -Encoding UTF8 $s
        & $PSCommandPath exec $s -VMX $VMX -Wrapper $Wrapper -Transfer $Transfer -GuestDir $GuestDir
    }
    'launch' {
        $s = Join-Path $Transfer 'launch.ps1'
        @"
Get-Process RimArcho -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 500
`$env:RIMARCHO_DEVTOOLS_PORT = '$DevToolsPort'
Start-Process -FilePath '$GuestDir\RimArcho.exe' -WorkingDirectory '$GuestDir'
"@ | Set-Content -Encoding UTF8 $s
        PS1InGuest $s -Interactive
        Write-Host "launched; DevTools at $(GuestIP):$ForwardPort (after 'forward')"
    }
    'shot' {
        $file = if ($Rest.Count) { $Rest[0] } else { Join-Path $Transfer ("screen-" + (Get-Date -Format 'HHmmss') + '.png') }
        $py = Join-Path (Split-Path $Wrapper) 'vmcli_exec.py'
        python $py $VMX MKS captureScreenshot $file | Out-Null
        Write-Host $file
    }
    'exec' {
        $file = $Rest[0]
        $res = "$work\exec-out.txt"
        $s = Join-Path $Transfer 'exec-wrap.ps1'
        $inner = Split-Path -Leaf $file
        [IO.File]::WriteAllText((Join-Path $Transfer $inner), [IO.File]::ReadAllText($file), [Text.UTF8Encoding]::new($true))
        ToGuest (Join-Path $Transfer $inner) "$work\$inner"
        # Native programs write UTF-8 (archo) or the console code page; read them as UTF-8.
        "[Console]::OutputEncoding = [Text.Encoding]::UTF8; `$OutputEncoding = [Text.Encoding]::UTF8; & '$work\$inner' *>&1 | Out-File -Encoding UTF8 '$res'" | Set-Content -Encoding UTF8 $s
        PS1InGuest $s -Interactive:$Interactive
        FromGuest $res (Join-Path $Transfer 'exec-out.txt')
        Get-Content -Encoding UTF8 (Join-Path $Transfer 'exec-out.txt')
    }
    'cli' {
        $s = Join-Path $Transfer 'cli.ps1'
        $cliArgs = ($Rest -join ' ')
        "& '$GuestDir\archo.exe' $cliArgs 2>&1" | Set-Content -Encoding UTF8 $s
        & $PSCommandPath exec $s -VMX $VMX -Wrapper $Wrapper -Transfer $Transfer -GuestDir $GuestDir -StandardUser:$StandardUser
    }
    'go' {
        & $PSCommandPath build; & $PSCommandPath push; & $PSCommandPath launch
    }
    default { throw "unknown command: $Command" }
}
