# The spikes/tuimem session (scan, browse, search, Problems view, log analysis, idle) driven
# in a real WezTerm window through `wezterm cli send-text`, sampling archo's working set.
# Run it against a copy of the data directory: the scan writes to archo.db.
#   pwsh -File archosession.ps1 -Home <copy> [-IdleSeconds 120]
param([Parameter(Mandatory)] [string]$DataHome, [int]$IdleSeconds = 120)
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$wdir = "$wz\WezTerm-windows-20240203-110809-5046fc22"
$env:RIMARCHO_HOME = $DataHome
Get-Process archo, wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
$p = Start-Process -FilePath "$wdir\wezterm-gui.exe" -PassThru -ArgumentList @("--config-file", "$wz\wezterm.lua", "start", "--", "$wz\bin\archo.exe")
Start-Sleep -Seconds 4
$sock = (Get-ChildItem "$env:USERPROFILE\.local\share\wezterm" -Filter "gui-sock-$($p.Id)").FullName
$env:WEZTERM_UNIX_SOCKET = $sock
$a = Get-Process archo | Where-Object { $_.StartTime -gt $p.StartTime } | Select-Object -First 1
function Send([string]$text) { & "$wdir\wezterm.exe" cli send-text --no-paste $text | Out-Null }
function Report([string]$what) {
  $a.Refresh()
  "{0,-26} archo ws {1,6:N1} MB  private {2,6:N1} MB  peak {3,6:N1} MB" -f $what, ($a.WorkingSet64 / 1MB), ($a.PrivateMemorySize64 / 1MB), ($a.PeakWorkingSet64 / 1MB)
}
Report "list shown"
Send "R"; Start-Sleep -Seconds 10
for ($i = 0; $i -lt 60; $i++) { Send "j"; Start-Sleep -Milliseconds 150 }
Send "/har"; Send "`r"; Send "`e"; Start-Sleep -Milliseconds 300
foreach ($k in "G", "g", "f", "f", "f", "f") { Send $k; Start-Sleep -Milliseconds 100 }
Start-Sleep -Seconds 1; Report "after scan + browsing"
Send "2"; Start-Sleep -Seconds 15; Report "problems view shown"
for ($i = 0; $i -lt 40; $i++) { Send "j"; if ($i % 5 -eq 0) { Send "`r" }; Start-Sleep -Milliseconds 100 }
Send "1"; Start-Sleep -Seconds 1; Report "after problems"
Send "l"; Start-Sleep -Milliseconds 500; Send "`r"; Start-Sleep -Seconds 6; Report "log analysed (l)"
Send "`e"; Send "1"; Start-Sleep -Seconds 1
pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out "$wz\shots\archosession.png" -DelayMs 200 | Out-Null
$step = [int]($IdleSeconds / 4)
for ($i = 1; $i -le 4; $i++) { Start-Sleep -Seconds $step; Report "idle $($i * $step)s" }
Get-Process archo, wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
