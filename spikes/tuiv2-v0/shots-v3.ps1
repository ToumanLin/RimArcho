# The V3 screenshots (06 §10): detail column with the thumbnail, the full-screen detail
# with its bigger thumbnail and sub-sections, and wrapping of mixed Chinese and English.
# Always on a data directory copy. Needs the display on: Windows does not draw windows
# while the screen is off, and every capture then comes out blank (checked below).
#   pwsh -File shots-v3.ps1 -DataHome <copy> [-Ref 839005762]
param([Parameter(Mandatory)] [string]$DataHome, [string]$Ref = "839005762")
if (-not (Test-Path (Join-Path $DataHome "archo.db"))) { throw "no archo.db in $DataHome" }
$shot = Join-Path $PSScriptRoot "shot-tui.ps1"
$shots = Join-Path (Split-Path -Parent (Split-Path -Parent $PSScriptRoot)) "spikes\wezterm-v0\shots"
$find = "h|/|id:$Ref|<enter>|<sleep 1500>"   # the mod, in the not-enabled column
$runs = @(
  @{ Name = "v3-column"; Cols = 160; Keys = "$find|<sleep 2500>" },
  @{ Name = "v3-full"; Cols = 160; Keys = "$find|<enter>|<sleep 4000>" },
  @{ Name = "v3-full-sections"; Cols = 160; Keys = "$find|<enter>|<sleep 3000>|1|<sleep 800>" },
  @{ Name = "v3-column-130"; Cols = 130; Keys = "j|<sleep 2500>" },   # RimCrow: CJK and English mixed
  @{ Name = "v3-full-100"; Cols = 100; Keys = "j|<enter>|<sleep 2500>" }   # narrow: no detail column, enter opens it
)
Add-Type -AssemblyName System.Drawing
foreach ($r in $runs) {
  pwsh -NoProfile -File $shot -Cols $r.Cols -Rows 46 -Name $r.Name -Lang zh -DataHome $DataHome -Keys $r.Keys
  $png = Join-Path $shots "$($r.Name).png"
  $bmp = [System.Drawing.Bitmap]::FromFile($png)
  $c = $bmp.GetPixel([int]($bmp.Width / 2), [int]($bmp.Height / 2)); $bmp.Dispose()
  if ($c.R -gt 240 -and $c.G -gt 240 -and $c.B -gt 240) { Write-Warning "$($r.Name): blank capture (is the display off?)" }
}
