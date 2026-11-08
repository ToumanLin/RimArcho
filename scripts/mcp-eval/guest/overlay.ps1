# Per run, as admin, before case.ps1: write the case's faulty files into the Mods' real folders (Workshop or C:\Eval\lib).
# In place, with the replaced file's timestamps, so the broken Mod looks like every other Mod (same junction, no new dates).
# Every run starts from the snapshot, so the originals come back on revert. param: case id
param([string]$Case)
$ErrorActionPreference = 'Stop'
$pkg = "C:\Eval\pkg\$Case"
$ws = 'C:\Program Files (x86)\Steam\steamapps\workshop\content\294100'
# Mods the guest's library lacks come with the case (package.mjs `ship:`)
if (Test-Path "$pkg\lib") { Copy-Item "$pkg\lib\*" C:\Eval\lib -Recurse -Force }
$list = Get-Content "$pkg\manifest.json" -Raw | ConvertFrom-Json | ForEach-Object { $_ }   # Windows PowerShell 5 emits the array as one object
foreach ($m in $list | Where-Object { $_.overlay }) {
  $i = $m.src.IndexOf(":"); $kind = $m.src.Substring(0, $i); $id = $m.src.Substring($i + 1)
  $target = if ($kind -eq 'ws') { Join-Path $ws $id } else { Join-Path C:\Eval\lib $id }
  $src = Get-ChildItem "$pkg\overlay" -Directory | Where-Object { $_.Name -eq $m.pkg } | Select-Object -First 1   # -eq ignores case
  $newest = (Get-ChildItem $target -Recurse -File | Sort-Object LastWriteTime -Descending | Select-Object -First 1).LastWriteTime
  foreach ($f in Get-ChildItem $src.FullName -Recurse -File) {
    $dest = Join-Path $target $f.FullName.Substring($src.FullName.Length + 1)
    $stamp = if (Test-Path $dest) { (Get-Item $dest).LastWriteTime } else { $newest }
    New-Item -ItemType Directory -Force (Split-Path $dest) | Out-Null
    Copy-Item $f.FullName $dest -Force
    $d = Get-Item $dest; $d.LastWriteTime = $stamp; $d.CreationTime = $stamp
  }
}
'overlay ok'
