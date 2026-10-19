# Crops the same region out of several screenshots and lays the crops out in a grid.
#   pwsh -File montage.ps1 -Out grid.png -X 600 -Y 40 -W 410 -H 510 -Cols 3 a.png b.png ...
param([string]$Out, [int]$X, [int]$Y, [int]$W, [int]$H, [int]$Cols = 3, [Parameter(ValueFromRemainingArguments)] [string[]]$Files)
Add-Type -AssemblyName System.Drawing
$rows = [math]::Ceiling($Files.Count / $Cols)
$bmp = New-Object Drawing.Bitmap ($Cols * ($W + 10)), ($rows * ($H + 10))
$g = [Drawing.Graphics]::FromImage($bmp)
$g.Clear([Drawing.Color]::White)
for ($i = 0; $i -lt $Files.Count; $i++) {
  $src = [Drawing.Bitmap]::FromFile($Files[$i])
  $dst = New-Object Drawing.Rectangle (($i % $Cols) * ($W + 10)), ([math]::Floor($i / $Cols) * ($H + 10)), $W, $H
  $g.DrawImage($src, $dst, (New-Object Drawing.Rectangle $X, $Y, $W, $H), [Drawing.GraphicsUnit]::Pixel)
  $src.Dispose()
}
$bmp.Save($Out)
"saved $Out"
