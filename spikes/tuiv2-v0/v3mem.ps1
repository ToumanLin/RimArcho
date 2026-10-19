# V3 memory gate (06 §11 T5): archo's working set in WezTerm after browsing with thumbnails.
# Always on a data directory copy (-DataHome is required). The cursor rests on -Mods mods in
# turn, long enough for each thumbnail to be drawn, opens one full-screen detail, then the
# TUI idles while the working set is sampled every 5 s.
#   pwsh -File v3mem.ps1 -DataHome <copy> [-Mods 40] [-RestMs 400] [-IdleSeconds 60]
param([Parameter(Mandatory)] [string]$DataHome, [int]$Mods = 40, [int]$RestMs = 400, [int]$IdleSeconds = 60)
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$wdir = "$wz\WezTerm-windows-20240203-110809-5046fc22"
if (-not (Test-Path (Join-Path $DataHome "archo.db"))) { throw "no archo.db in $DataHome" }
$env:RIMARCHO_HOME = $DataHome
Get-Process archo, wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
$p = Start-Process -FilePath "$wdir\wezterm-gui.exe" -PassThru -ArgumentList @(
  "--config-file", "$wz\wezterm.lua", "start", "--", "$wz\bin\archo.exe", "tui")
$a = $null
for ($i = 0; $i -lt 100 -and -not $a; $i++) { Start-Sleep -Milliseconds 100; $a = Get-Process archo -ErrorAction SilentlyContinue | Select-Object -First 1 }
Start-Sleep -Seconds 4
$env:WEZTERM_UNIX_SOCKET = (Get-ChildItem "$env:USERPROFILE\.local\share\wezterm" -Filter "gui-sock-$($p.Id)").FullName
function Send([string]$t) { & "$wdir\wezterm.exe" cli send-text --no-paste $t | Out-Null }
function Sample([string]$what) {
  $a.Refresh()
  "{0,-22} working set {1,6:N1} MB  private {2,6:N1} MB  peak {3,6:N1} MB" -f $what, ($a.WorkingSet64 / 1MB), ($a.PrivateMemorySize64 / 1MB), ($a.PeakWorkingSet64 / 1MB)
}
Sample "started"
for ($i = 0; $i -lt $Mods; $i++) { Send "j"; Start-Sleep -Milliseconds $RestMs }
Sample "after $Mods thumbnails"
Send "`r"; Start-Sleep -Seconds 3; Sample "full-screen detail"
Send "G"; Start-Sleep -Milliseconds 500; Send "`e"; Start-Sleep -Milliseconds 500
$t0 = Get-Date
while (((Get-Date) - $t0).TotalSeconds -lt $IdleSeconds) {
  Start-Sleep -Seconds 5
  Sample ("idle {0,3}s" -f [int]((Get-Date) - $t0).TotalSeconds)
}
$w = Get-Process wezterm-gui -ErrorAction SilentlyContinue | Select-Object -First 1
if ($w) { "wezterm-gui working set {0:N1} MB (recorded separately, 06 §1.4)" -f ($w.WorkingSet64 / 1MB) }
Send "q"; Start-Sleep -Milliseconds 800
Get-Process archo, wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
$n = @(Get-ChildItem (Join-Path $DataHome "cache\previews") -Filter *.png -ErrorAction SilentlyContinue).Count
"thumbnail cache: $n files"
