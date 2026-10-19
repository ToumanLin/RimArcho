# Runs the thumbnail spike in WezTerm (scripted scroll) and screenshots each checkpoint,
# plus three shots during the fast scroll.
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$log = Join-Path $wz "shots\thumbs.log"
Remove-Item $log -ErrorAction SilentlyContinue
$shots = Join-Path $wz "shots"
Get-ChildItem $shots -Filter "thumbs-*.png" -ErrorAction SilentlyContinue | Remove-Item
$p = Start-Process -FilePath "$wz\WezTerm-windows-20240203-110809-5046fc22\wezterm-gui.exe" -PassThru -ArgumentList @(
  "--config-file", "$wz\wezterm.lua", "start", "--", "$wz\bin\thumbs.exe", "-auto", "-log", $log)
$seen = 0; $fast = 0; $t0 = Get-Date
while (-not $p.HasExited -and ((Get-Date) - $t0).TotalSeconds -lt 120) {
  Start-Sleep -Milliseconds 200
  if (-not (Test-Path $log)) { continue }
  $cps = @(Select-String -Path $log -Pattern "CHECKPOINT (.*)$" | ForEach-Object { $_.Matches[0].Groups[1].Value })
  if ($cps.Count -gt $seen) {
    $name = ($cps[$seen] -replace '[^A-Za-z0-9]+', '-').Trim('-')
    Start-Sleep -Milliseconds 900
    pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out (Join-Path $shots ("thumbs-{0}-{1}.png" -f $seen, $name)) -DelayMs 50
    $seen++
    if ($seen -eq 1) {
      # during the fast scroll (≈ 10 s of 20 ms steps)
      foreach ($i in 1..3) {
        Start-Sleep -Milliseconds 1800
        pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -ProcessId $p.Id -Out (Join-Path $shots ("thumbs-fast-{0}.png" -f $i)) -DelayMs 0
      }
    }
  }
}
"checkpoints: $seen"
Get-Content $log | Select-Object -Last 8
