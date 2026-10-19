# Screenshots of `archo tui` in WezTerm at a given size, after an optional key script.
#   pwsh -File shot-tui.ps1 -Cols 160 -Rows 46 -Name v1v2-160 [-Keys "j|j|l"] [-DataHome <dir>] [-Wait 5]
# Keys go through `wezterm cli send-text --no-paste`; "<enter>", "<esc>", "<tab>", "<space>"
# and "<sleep N>" (milliseconds) are understood. The TUI is quit with q at the end.
param([int]$Cols = 160, [int]$Rows = 46, [Parameter(Mandatory)] [string]$Name, [string]$Keys = "",
  [string]$DataHome = "", [int]$Wait = 5, [string]$Lang = "", [string]$Icons = "")
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$wdir = "$wz\WezTerm-windows-20240203-110809-5046fc22"
if ($DataHome) { $env:RIMARCHO_HOME = $DataHome } else { Remove-Item env:RIMARCHO_HOME -ErrorAction SilentlyContinue }
if ($Lang) { $env:ARCHO_LANG = $Lang } else { Remove-Item env:ARCHO_LANG -ErrorAction SilentlyContinue }
Get-Process archo, wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
$p = Start-Process -FilePath "$wdir\wezterm-gui.exe" -PassThru -ArgumentList @(
  "--config-file", "$wz\wezterm.lua", "--config", "initial_cols=$Cols", "--config", "initial_rows=$Rows", "start", "--", "$wz\bin\archo.exe", "tui")
Start-Sleep -Seconds $Wait
$env:WEZTERM_UNIX_SOCKET = (Get-ChildItem "$env:USERPROFILE\.local\share\wezterm" -Filter "gui-sock-$($p.Id)").FullName
function Send([string]$t) { & "$wdir\wezterm.exe" cli send-text --no-paste $t | Out-Null }
foreach ($k in ($Keys -split "\|" | Where-Object { $_ -ne "" })) {
  switch -regex ($k) {
    '^<enter>$' { Send "`r" }
    '^<esc>$' { Send "`e" }
    '^<tab>$' { Send "`t" }
    '^<space>$' { Send " " }
    '^<sleep (\d+)>$' { Start-Sleep -Milliseconds ([int]$Matches[1]) }
    default { Send $k }
  }
  Start-Sleep -Milliseconds 120
}
Start-Sleep -Milliseconds 800
pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out (Join-Path $wz "shots\$Name.png") -DelayMs 200
Send "q"
Start-Sleep -Milliseconds 800
Get-Process archo, wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
