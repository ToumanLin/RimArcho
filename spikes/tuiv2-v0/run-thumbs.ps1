# Runs the thumbnail spike in WezTerm (scripted scroll) and screenshots each checkpoint,
# plus three shots during the fast scroll.
#   pwsh -File run-thumbs.ps1 [-Ghost] [-OldFit] [-Prefix thumbs]
#   -Ghost   tallest preview → 16:9 → no preview, instead of the scroll test
#   -OldFit  the first V0 sizing (preserveAspectRatio=1), which leaves ghosts
param([switch]$Ghost, [switch]$OldFit, [string]$Prefix = "thumbs")
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$log = Join-Path $wz "shots\$Prefix.log"
Remove-Item $log -ErrorAction SilentlyContinue
$shots = Join-Path $wz "shots"
Get-ChildItem $shots -Filter "$Prefix-*.png" -ErrorAction SilentlyContinue | Remove-Item
$env:THUMBS_GHOST = $(if ($Ghost) { "1" } else { "" })
$env:THUMBS_OLDFIT = $(if ($OldFit) { "1" } else { "" })
$p = Start-Process -FilePath "$wz\WezTerm-windows-20240203-110809-5046fc22\wezterm-gui.exe" -PassThru -ArgumentList @(
  "--config-file", "$wz\wezterm.lua", "start", "--", "$wz\bin\thumbs.exe", "-auto", "-log", $log)
$seen = 0; $t0 = Get-Date
while (-not $p.HasExited -and ((Get-Date) - $t0).TotalSeconds -lt 120) {
  Start-Sleep -Milliseconds 200
  if (-not (Test-Path $log)) { continue }
  $cps = @(Select-String -Path $log -Pattern "CHECKPOINT (.*)$" | ForEach-Object { $_.Matches[0].Groups[1].Value })
  if ($cps.Count -gt $seen) {
    $name = ($cps[$seen] -replace '[^A-Za-z0-9]+', '-').Trim('-')
    Start-Sleep -Milliseconds 900
    pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out (Join-Path $shots ("{0}-{1}-{2}.png" -f $Prefix, $seen, $name)) -DelayMs 50
    $seen++
    if ($seen -eq 1 -and -not $Ghost) {
      # during the fast scroll (≈ 10 s of 20 ms steps)
      foreach ($i in 1..3) {
        Start-Sleep -Milliseconds 1800
        pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out (Join-Path $shots ("{0}-fast-{1}.png" -f $Prefix, $i)) -DelayMs 0
      }
    }
  }
}
"checkpoints: $seen"
Get-Content $log | Select-String "ghost rows|draw gen|CHECKPOINT" | Select-Object -Last 14 | ForEach-Object { $_.Line }
