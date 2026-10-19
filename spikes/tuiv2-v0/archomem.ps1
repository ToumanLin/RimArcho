# archo's own working set when it runs the TUI on the real data: in WezTerm and in a plain
# console window, with the Go GC trace written to a file (GODEBUG=gctrace=1, stderr only).
#   pwsh -File archomem.ps1 [-Seconds 60] [-Host wezterm|console] [-Env "NAME=value;..."]
param([int]$Seconds = 60, [string]$Where = "wezterm", [string]$Env = "", [string]$Tag = "")
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$archo = "$wz\bin\archo.exe"
$trace = Join-Path $wz ("shots\gctrace-{0}{1}.txt" -f $Where, $Tag)
Remove-Item $trace -ErrorAction SilentlyContinue
$env:GODEBUG = "gctrace=1"
foreach ($kv in ($Env -split ";" | Where-Object { $_ })) { $k, $v = $kv -split "=", 2; Set-Item "env:$k" $v }
$cmdline = "`"$archo`" 2> `"$trace`""
if ($Where -eq "wezterm") {
  $p = Start-Process -FilePath "$wz\WezTerm-windows-20240203-110809-5046fc22\wezterm-gui.exe" -PassThru -ArgumentList @(
    "--config-file", "$wz\wezterm.lua", "start", "--", "cmd.exe", "/c", $cmdline)
} else {
  $p = Start-Process -FilePath "cmd.exe" -PassThru -ArgumentList @("/c", $cmdline)
}
$a = $null
for ($i = 0; $i -lt 100 -and -not $a; $i++) { Start-Sleep -Milliseconds 100; $a = Get-Process archo -ErrorAction SilentlyContinue | Select-Object -First 1 }
$t0 = Get-Date
while (((Get-Date) - $t0).TotalSeconds -lt $Seconds) {
  Start-Sleep -Seconds 10
  $a.Refresh()
  "{0,4}s  archo working set {1,6:N1} MB  private {2,6:N1} MB  peak {3,6:N1} MB" -f [int]((Get-Date) - $t0).TotalSeconds, ($a.WorkingSet64 / 1MB), ($a.PrivateMemorySize64 / 1MB), ($a.PeakWorkingSet64 / 1MB)
}
Get-Process archo, wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 300
"gc trace (last 3):"; Get-Content $trace -ErrorAction SilentlyContinue | Select-String "^gc " | Select-Object -Last 3 | ForEach-Object { $_.Line }
"gc cycles: $(@(Get-Content $trace -ErrorAction SilentlyContinue | Select-String '^gc ').Count)"
