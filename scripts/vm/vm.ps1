# Test RimArcho in the Player-Test VM (VMware Workstation) from the host, with no clicks.
# Wraps D:\VMs\Setup\Invoke-VMRun.ps1, which holds the VM and guest credentials (never printed).
# See scripts/vm/README.md.
#
#   pwsh scripts/vm/vm.ps1 status            # VM running? Tools ready? guest IP
#   pwsh scripts/vm/vm.ps1 build             # go build both exes into dist/vm/RimArcho, with Archo Monitor from monitor/
#   pwsh scripts/vm/vm.ps1 build -Monitor <dir>  # ... bundling that Archo Monitor mod folder instead
#   pwsh scripts/vm/vm.ps1 push              # copy them into the guest ($GuestDir)
#   pwsh scripts/vm/vm.ps1 prep              # once per VM: no display-off or sleep while idle
#   pwsh scripts/vm/vm.ps1 forward           # once per VM: expose the DevTools port to the host
#   pwsh scripts/vm/vm.ps1 launch            # (re)start RimArcho.exe on the guest desktop, DevTools on
#   pwsh scripts/vm/vm.ps1 shot [file.png]   # screenshot of the guest screen
#   pwsh scripts/vm/vm.ps1 exec script.ps1   # run a PowerShell script in the guest, print its output
#   pwsh scripts/vm/vm.ps1 cli "scan"        # run archo.exe <args> in the guest, print its output
#   pwsh scripts/vm/vm.ps1 quitgame [sec]    # close the game's window like the title bar's X, wait for it to exit
#   pwsh scripts/vm/vm.ps1 input start|stop|status  # real mouse and keyboard agent for input.mjs
#   pwsh scripts/vm/vm.ps1 go                # build + push + launch
param(
    [Parameter(Position = 0, Mandatory = $true)][string]$Command,
    [Parameter(Position = 1, ValueFromRemainingArguments = $true)][string[]]$Rest = @(),
    [string]$VMX = 'D:\VMs\Player-Test\Win11-Player-Test.vmx',
    [string]$Wrapper = 'D:\VMs\Setup\Invoke-VMRun.ps1',
    [string]$Transfer = '',      # guest scripts; default: a folder per VM, so several VMs can run at once
    [string]$GuestDir = 'C:\Users\Public\Documents\VM-Test\RimArcho',
    [int]$DevToolsPort = 9222,   # inside the guest, on 127.0.0.1
    [int]$ForwardPort = 9223,    # what the host connects to
    [int]$InputPort = 9230,      # input agent, inside the guest, on 127.0.0.1
    [int]$InputForwardPort = 9231, # what input.mjs connects to
    [switch]$Interactive,       # exec: run a window runner in the signed-in desktop session
    [switch]$StandardUser,       # run guest programs as the standard test user instead of the logged-in Player
    [string]$Monitor = ''        # build: bundle this Archo Monitor mod folder instead of building monitor/
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$work = 'C:\Users\Public\Documents\VM-Test'
# The build (RimArcho.zip) is shared by all VMs; the guest scripts are per VM.
$shared = 'D:\RimArcho-test\transfer\rimarcho'
if (-not $Transfer) { $Transfer = Join-Path $shared (Split-Path -Leaf (Split-Path -Parent $VMX)) }
New-Item -ItemType Directory -Force $shared, $Transfer | Out-Null

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
    'put' {
        if ($Rest.Count -ne 2) { throw 'put requires host source and guest destination' }
        ToGuest $Rest[0] $Rest[1]
    }
    'get' {
        if ($Rest.Count -ne 2) { throw 'get requires guest source and host destination' }
        FromGuest $Rest[0] $Rest[1]
    }
    'status' {
        $list = @(& 'C:\Program Files\VMware\VMware Workstation\vmrun.exe' list)
        $list | Write-Host
        $vmxFull = [IO.Path]::GetFullPath($VMX)
        if (-not ($list | Where-Object { $_.Trim() -eq $vmxFull })) {
            Write-Host "the VM is off ($VMX)"
            break
        }
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
            # Archo Monitor: built from monitor/ like scripts/release.ps1 does (again only when
            # its source changed), or the mod folder -Monitor names.
            $want = (Get-Content -Raw (Join-Path $repo 'monitor\VERSION')).Trim()
            if ($Monitor) {
                $mon = (Resolve-Path -LiteralPath $Monitor).Path
            } else {
                $mon = Join-Path $repo 'monitor\out\ArchoMonitor'
                $built = Get-Item -LiteralPath (Join-Path $mon 'About\About.xml') -ErrorAction SilentlyContinue
                $src = Get-ChildItem -Recurse -File (Join-Path $repo 'monitor') | Where-Object { $_.FullName -notmatch '\\monitor\\(out|bin|obj|test)\\' } | Sort-Object LastWriteTime | Select-Object -Last 1
                if (-not $built -or $src.LastWriteTime -gt $built.LastWriteTime) {
                    Write-Host 'building Archo Monitor (monitor/build.ps1)'
                    & (Join-Path $repo 'monitor\build.ps1') -Out $mon | Out-Null
                }
            }
            $about = Join-Path $mon 'About\About.xml'
            if (-not (Test-Path -LiteralPath $about)) { throw "$mon is not an Archo Monitor mod folder (no About\About.xml)" }
            $meta = ([xml](Get-Content -Raw -LiteralPath $about)).ModMetaData
            $pkg = "$($meta.SelectSingleNode('packageId').InnerText)".Trim()
            if ($pkg -ne 'toumanlin.archomonitor') { throw "$mon is not Archo Monitor (packageId $pkg)" }
            $monVer = "$($meta.SelectSingleNode('modVersion').InnerText)".Trim()
            if (Test-Path (Join-Path $out 'monitor')) { Remove-Item -Recurse -Force (Join-Path $out 'monitor') }
            New-Item -ItemType Directory -Force (Join-Path $out 'monitor') | Out-Null
            Copy-Item -Recurse -Force $mon (Join-Path $out 'monitor\ArchoMonitor')
            if ($monVer -ne $want) { Write-Warning "the bundled Archo Monitor is $monVer, but monitor/VERSION is $want" }
            $zip = Join-Path $shared 'RimArcho.zip'
            Compress-Archive -Force -Path "$out\*" -DestinationPath $zip
            Write-Host "built $commit with Archo Monitor $monVer -> $zip ($([math]::Round((Get-Item $zip).Length/1MB,1)) MB)"
        } finally { Pop-Location }
    }
    'push' {
        $zip = Join-Path $shared 'RimArcho.zip'
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
        # Admin in the guest: forward 0.0.0.0:$ForwardPort to the DevTools port and
        # 0.0.0.0:$InputForwardPort to the input agent, and open the firewall for both.
        # Test VM only (NAT network).
        $s = Join-Path $Transfer 'forward.ps1'
        @"
foreach (`$p in @(@($ForwardPort, $DevToolsPort, 'RimArcho DevTools (test)'), @($InputForwardPort, $InputPort, 'RimArcho input agent (test)'))) {
  netsh interface portproxy delete v4tov4 listenport=`$(`$p[0]) listenaddress=0.0.0.0 2>`$null | Out-Null
  netsh interface portproxy add v4tov4 listenport=`$(`$p[0]) listenaddress=0.0.0.0 connectport=`$(`$p[1]) connectaddress=127.0.0.1
  if (-not (Get-NetFirewallRule -DisplayName `$p[2] -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName `$p[2] -Direction Inbound -Protocol TCP -LocalPort `$p[0] -Action Allow -Profile Any | Out-Null
  }
}
netsh interface portproxy show v4tov4
"@ | Set-Content -Encoding UTF8 $s
        & $Wrapper -VMX $VMX -Guest -Command copyFileFromHostToGuest -VMArgs @($s, "$work\forward.ps1")
        & $Wrapper -VMX $VMX -Guest -Command runProgramInGuest -VMArgs @('C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "$work\forward.ps1")
        Write-Host "host connects to $(GuestIP):$ForwardPort (DevTools) and :$InputForwardPort (input agent)"
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
        $token=[guid]::NewGuid().ToString('N').Substring(0,10)
        $res = "$work\exec-out-$token.txt"
        $s = Join-Path $Transfer "exec-wrap-$token.ps1"
        $inner = Split-Path -Leaf $file
        [IO.File]::WriteAllText((Join-Path $Transfer $inner), [IO.File]::ReadAllText($file), [Text.UTF8Encoding]::new($true))
        ToGuest (Join-Path $Transfer $inner) "$work\$inner"
        # Native programs write UTF-8 (archo) or the console code page; read them as UTF-8.
        "[Console]::OutputEncoding = [Text.Encoding]::UTF8; `$OutputEncoding = [Text.Encoding]::UTF8; & '$work\$inner' *>&1 | Out-File -Encoding UTF8 '$res'" | Set-Content -Encoding UTF8 $s
        PS1InGuest $s -Interactive:$Interactive
        FromGuest $res (Join-Path $Transfer "exec-out-$token.txt")
        Get-Content -Encoding UTF8 (Join-Path $Transfer "exec-out-$token.txt")
    }
    'cli' {
        $s = Join-Path $Transfer 'cli.ps1'
        $cliArgs = ($Rest -join ' ')
        # archo writes progress to stderr; pass it through as text, not as red error records.
        "& '$GuestDir\archo.exe' $cliArgs 2>&1 | ForEach-Object { `"`$_`" }" | Set-Content -Encoding UTF8 $s
        & $PSCommandPath exec $s -VMX $VMX -Wrapper $Wrapper -Transfer $Transfer -GuestDir $GuestDir -StandardUser:$StandardUser
    }
    'quitgame' {
        # Close the game's window the way its title bar X does (WM_CLOSE in the signed-in
        # session), so the game quits as for a player: Archo Monitor writes its exit line and
        # Steam syncs the saves. No clicks, any resolution, any window on top. Waits up to
        # [sec] (60) for the process to end; prints FAIL and leaves the game alone if it doesn't.
        $wait = if ($Rest.Count) { [int]$Rest[0] } else { 60 }
        $s = Join-Path $Transfer 'quitgame.ps1'
        @"
`$games = @(Get-Process RimWorldWin64, RimWorld -ErrorAction SilentlyContinue)
if (-not `$games) { 'PASS quitgame: no game running'; return }
`$data = Join-Path `$env:USERPROFILE 'AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios'
`$cmd = (Get-CimInstance Win32_Process -Filter "ProcessId=`$(`$games[0].Id)").CommandLine
if (`$cmd -match '-savedatafolder=(?:"([^"]+)"|(\S+))') { `$data = if (`$Matches[1]) { `$Matches[1] } else { `$Matches[2] } }
`$t0 = Get-Date
foreach (`$g in `$games) { if (-not `$g.CloseMainWindow()) { "NOTE quitgame: `$(`$g.Name) `$(`$g.Id) has no window in this session" } }
while (((Get-Date) - `$t0).TotalSeconds -lt $wait -and @(Get-Process -Id `$games.Id -ErrorAction SilentlyContinue).Count) { Start-Sleep -Milliseconds 500 }
`$left = @(Get-Process -Id `$games.Id -ErrorAction SilentlyContinue)
if (`$left) { "FAIL quitgame: `$((`$left | ForEach-Object { `$_.Name + ' ' + `$_.Id }) -join ', ') still running after $wait s (a dialog asking something? vm.ps1 shot)"; return }
"PASS quitgame: the game exited in `$([int]((Get-Date) - `$t0).TotalSeconds) s"
`$log = Join-Path `$data 'ArchoMonitor.log'
if (-not (Test-Path -LiteralPath `$log)) { "NOTE quitgame: no ArchoMonitor.log in `$data (no Archo Monitor in the list?)"; return }
`$last = @(Get-Content -Encoding UTF8 -LiteralPath `$log | Where-Object { `$_.Trim() }) | Select-Object -Last 1
if (`$last -match '"kind":"exit"') { 'PASS quitgame: Archo Monitor wrote its exit line' } else { "FAIL quitgame: ArchoMonitor.log does not end with an exit line (last: `$(if (`$last.Length -gt 120) { `$last.Substring(0, 120) } else { `$last }))" }
"@ | Set-Content -Encoding UTF8 $s
        & $PSCommandPath exec $s -Interactive -VMX $VMX -Wrapper $Wrapper -Transfer $Transfer -GuestDir $GuestDir
    }
    'input' {
        # The real-input agent (input-agent.ps1) in the signed-in desktop session; input.mjs
        # drives it. A new token each start, kept in $Transfer for input.mjs.
        $sub = if ($Rest.Count) { $Rest[0] } else { 'status' }
        $pidFile = "$work\input-agent.pid"; $logFile = "$work\input-agent.log"
        $stop = "`$old = Get-Content -LiteralPath '$pidFile' -ErrorAction SilentlyContinue; if (`$old) { Stop-Process -Id `$old -Force -ErrorAction SilentlyContinue; Remove-Item -LiteralPath '$pidFile' -ErrorAction SilentlyContinue; 'stopped agent ' + `$old } else { 'no agent running' }"
        switch ($sub) {
            'start' {
                $agent = Join-Path $Transfer 'input-agent.ps1'
                [IO.File]::WriteAllText($agent, [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'input-agent.ps1')), [Text.UTF8Encoding]::new($true))
                try { VM createDirectoryInGuest @($work) -Guest *>$null } catch { }
                ToGuest $agent "$work\input-agent.ps1"
                $token = [guid]::NewGuid().ToString('N')
                $s = Join-Path $Transfer 'input-start.ps1'
                @"
$stop
Remove-Item -LiteralPath '$logFile' -ErrorAction SilentlyContinue
& '$work\input-agent.ps1' -Port $InputPort -Token '$token' -PidFile '$pidFile' -LogFile '$logFile'
"@ | Set-Content -Encoding UTF8 $s
                PS1InGuest $s -Interactive -NoWait
                Set-Content -Encoding ASCII -NoNewline -LiteralPath (Join-Path $Transfer 'input-token.txt') -Value $token
                $ip = GuestIP
                $env:RIMARCHO_VM_INPUT_TOKEN = $token
                for ($i = 0; $i -lt 30; $i++) {
                    $r = node (Join-Path $PSScriptRoot 'input.mjs') "${ip}:$InputForwardPort" ping 2>&1
                    if ($LASTEXITCODE -eq 0) { Write-Host "input agent ready at ${ip}:$InputForwardPort"; return }
                    Start-Sleep -Seconds 1
                }
                Write-Host "FAIL input agent did not answer at ${ip}:$InputForwardPort ($r). Run 'forward' once; 'input status' shows its log."
                exit 1
            }
            'stop' {
                $s = Join-Path $Transfer 'input-stop.ps1'
                $stop | Set-Content -Encoding UTF8 $s
                & $PSCommandPath exec $s -VMX $VMX -Wrapper $Wrapper -Transfer $Transfer -GuestDir $GuestDir
            }
            'status' {
                $s = Join-Path $Transfer 'input-status.ps1'
                @"
`$old = Get-Content -LiteralPath '$pidFile' -ErrorAction SilentlyContinue
if (`$old -and (Get-Process -Id `$old -ErrorAction SilentlyContinue)) { 'agent running: ' + `$old } else { 'agent not running' }
if (Test-Path -LiteralPath '$logFile') { Get-Content -Encoding UTF8 -LiteralPath '$logFile' -Tail 15 }
"@ | Set-Content -Encoding UTF8 $s
                & $PSCommandPath exec $s -VMX $VMX -Wrapper $Wrapper -Transfer $Transfer -GuestDir $GuestDir
            }
            default { throw "input start|stop|status, not $sub" }
        }
    }
    'go' {
        & $PSCommandPath build; & $PSCommandPath push; & $PSCommandPath launch
    }
    default { throw "unknown command: $Command" }
}
