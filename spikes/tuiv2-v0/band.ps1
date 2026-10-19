# Non-background pixels in a horizontal band of the thumbnail column (default: the top 6 px
# of row 12, just under the 18x9 rect, where no glyph reaches). Prints one line per file.
#   pwsh -File band.ps1 [-Y0 381] [-Y1 386] <png>...
param([Parameter(Position = 0, ValueFromRemainingArguments)] [string[]]$Files, [int]$Y0 = 381, [int]$Y1 = 386)
Add-Type -AssemblyName System.Drawing
foreach ($f in $Files) {
  $b = [Drawing.Bitmap]::FromFile($f); $n = 0
  for ($y = $Y0; $y -le $Y1; $y++) { for ($x = 726; $x -lt 960; $x++) {
    $c = $b.GetPixel($x, $y); if ([math]::Abs($c.R - 14) + [math]::Abs($c.G - 15) + [math]::Abs($c.B - 18) -gt 24) { $n++ } } }
  $b.Dispose()
  "{0,-52} {1}" -f (Split-Path -Leaf $f), $n
}
