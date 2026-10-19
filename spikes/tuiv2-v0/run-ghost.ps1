# Runs ghost.exe in WezTerm, screenshots every phase and reports the magenta bounding box in
# window pixels and in cells (row/col of the terminal grid).
#   pwsh -File run-ghost.ps1 [-Margin 0] [-Params preserveAspectRatio=1]
param([int]$Margin = 0, [string]$Params = "preserveAspectRatio=1", [switch]$Fit)
Add-Type -AssemblyName System.Drawing
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$shots = Join-Path $wz "shots\ghost"
New-Item -ItemType Directory -Force $shots | Out-Null
$log = Join-Path $shots "phases.log"; $go = Join-Path $shots "next.flag"
Remove-Item $log, $go -ErrorAction SilentlyContinue
$p = Start-Process -FilePath "$wz\WezTerm-windows-20240203-110809-5046fc22\wezterm-gui.exe" -PassThru -ArgumentList @(
  "--config-file", "$wz\wezterm.lua", "start", "--", "$wz\bin\ghost.exe", "-log", $log, "-go", $go, "-margin", $Margin, "-params", $Params, "-fit=$([bool]$Fit)")
Start-Sleep -Seconds 3
# grid geometry from the config: padding 8/6 px; title bar measured from the first shot
$seen = 0; $t0 = Get-Date
while (-not $p.HasExited -and ((Get-Date) - $t0).TotalSeconds -lt 120) {
  Start-Sleep -Milliseconds 150
  if (-not (Test-Path $log)) { continue }
  $ph = @(Get-Content $log | Where-Object { $_ -like "PHASE *" } | ForEach-Object { $_.Substring(6) })
  if ($ph.Count -le $seen) { continue }
  $name = $ph[$seen]; $file = Join-Path $shots (("{3}m{0}-{1:d2}-{2}.png" -f $Margin, $seen, ($name -replace "[^A-Za-z0-9]+", "-"), $(if ($Fit) { "fit-" } else { "" })))
  Start-Sleep -Milliseconds 400
  pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out $file -DelayMs 100 | Out-Null
  $bmp = [Drawing.Bitmap]::FromFile($file)
  $x0 = 99999; $y0 = 99999; $x1 = -1; $y1 = -1
  for ($y = 40; $y -lt 1150; $y += 1) { for ($x = 600; $x -lt 1400; $x += 3) {
    $c = $bmp.GetPixel($x, $y); if ($c.R -gt 200 -and $c.G -lt 60 -and $c.B -gt 200) { if ($x -lt $x0) { $x0 = $x }; if ($x -gt $x1) { $x1 = $x }; if ($y -lt $y0) { $y0 = $y }; if ($y -gt $y1) { $y1 = $y } } } }
  $bmp.Dispose()
  if ($x1 -lt 0) { "{0,-34} no magenta" -f $name } else { "{0,-34} magenta x {1}-{2} y {3}-{4} ({5}x{6} px)" -f $name, $x0, $x1, $y0, $y1, ($x1 - $x0 + 1), ($y1 - $y0 + 1) }
  New-Item -ItemType File $go | Out-Null
  $seen++
}
Get-Process wezterm-gui -ErrorAction SilentlyContinue | Stop-Process -Force
