# Counts non-background pixels in the thumbnail column of thumbs-spike screenshots, outside
# the two text rows under the rect: rows 3–11 (the rect) and rows 14–40 (empty in the spike).
# A "no preview" shot must report 0 in both zones.
#   pwsh -File residue.ps1 <png>...
Add-Type -AssemblyName System.Drawing
# grid: title bar + padding put row 1 at y≈51; rows are 30 px, columns 13 px from x=8
$rowY = { param($r) 51 + ($r - 1) * 30 }
foreach ($f in $args) {
  $b = [Drawing.Bitmap]::FromFile($f)
  $res = foreach ($z in @(@{n = "rect rows 3-11"; r0 = 3; r1 = 11 }, @{n = "rows 14-40"; r0 = 14; r1 = 40 })) {
    $n = 0
    for ($y = (& $rowY $z.r0); $y -lt (& $rowY ($z.r1 + 1)); $y++) {
      for ($x = 726; $x -lt 960; $x += 2) {
        $c = $b.GetPixel($x, $y)
        if ([math]::Abs($c.R - 14) + [math]::Abs($c.G - 15) + [math]::Abs($c.B - 18) -gt 24) { $n++ }
      }
    }
    "{0}: {1}" -f $z.n, $n
  }
  $b.Dispose()
  "{0,-48} {1}" -f (Split-Path -Leaf $f), ($res -join "  ")
}
