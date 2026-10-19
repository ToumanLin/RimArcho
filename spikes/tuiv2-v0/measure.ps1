# TUI v2 V0 checks 3 and 4: wezterm-gui startup time, the environment the child sees, and the
# working set of the process tree while it hosts archo on the real data (idle).
#   pwsh -File measure.ps1 [-IdleSeconds 300] [-SampleSeconds 15]
param([int]$IdleSeconds = 300, [int]$SampleSeconds = 15)
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$gui = "$wz\WezTerm-windows-20240203-110809-5046fc22\wezterm-gui.exe"
$cfg = "$wz\wezterm.lua"
$out = Join-Path $wz "shots"
$archo = "$wz\bin\archo.exe"

function Kill-All { Get-Process archo, wezterm-gui, wezterm-mux-server -ErrorAction SilentlyContinue | Stop-Process -Force; Start-Sleep -Milliseconds 500 }

# time from process start until its top-level window is visible
function Start-Timed([string[]]$prog) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  $p = Start-Process -FilePath $gui -PassThru -ArgumentList (@("--config-file", $cfg, "start", "--") + $prog)
  while ($sw.ElapsedMilliseconds -lt 20000) {
    $p.Refresh()
    if ($p.MainWindowHandle -ne 0) { break }
    Start-Sleep -Milliseconds 10
  }
  return @{ p = $p; ms = $sw.ElapsedMilliseconds }
}

function Tree([int]$id) {
  $all = Get-CimInstance Win32_Process
  $ids = @($id); $i = 0
  while ($i -lt $ids.Count) { $ids += @($all | Where-Object { $_.ParentProcessId -eq $ids[$i] } | ForEach-Object { [int]$_.ProcessId }); $i++ }
  $ids | ForEach-Object { Get-Process -Id $_ -ErrorAction SilentlyContinue }
}

Kill-All
"== startup (window visible), idle child =="
$times = @()
foreach ($i in 1..3) {
  $r = Start-Timed @("cmd.exe", "/c", "pause>nul")
  $times += $r.ms; "run ${i}: $($r.ms) ms"
  Start-Sleep -Seconds 2; Kill-All
}
"== environment seen by the child =="
$envf = Join-Path $out "child-env.txt"; Remove-Item $envf -ErrorAction SilentlyContinue
$r = Start-Timed @("cmd.exe", "/c", "set > `"$envf`"")
Start-Sleep -Seconds 3; Kill-All
Get-Content $envf | Select-String -Pattern "^(TERM|TERM_PROGRAM|TERM_PROGRAM_VERSION|COLORTERM|WEZTERM_[A-Z_]+)=" | ForEach-Object { $_.Line }

"== archo (no args = TUI) on the real data =="
$r = Start-Timed @($archo)
"window visible after $($r.ms) ms"
$p = $r.p
Start-Sleep -Seconds 8
pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out (Join-Path $out "archo-in-wezterm.png") -DelayMs 300 | Out-Null
$samples = @()
$t0 = Get-Date
while (((Get-Date) - $t0).TotalSeconds -lt $IdleSeconds) {
  $procs = Tree $p.Id
  $row = [ordered]@{ t = [int]((Get-Date) - $t0).TotalSeconds }
  foreach ($q in $procs) { $row[$q.ProcessName] = [math]::Round($q.WorkingSet64 / 1MB, 1) }
  $row.gui_private = [math]::Round(($procs | Where-Object ProcessName -eq "wezterm-gui").PrivateMemorySize64 / 1MB, 1)
  $samples += [pscustomobject]$row
  ($row.GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" }) -join " "
  Start-Sleep -Seconds $SampleSeconds
}
pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out (Join-Path $out "archo-in-wezterm-idle.png") -DelayMs 300 | Out-Null
$cpu = (Get-Process -Id $p.Id).TotalProcessorTime.TotalSeconds
"wezterm-gui total CPU after idle: $([math]::Round($cpu, 2)) s"
Kill-All
"startup ms: $($times -join ', ')"
