# One-time staging inside the VM (run as the administrator Player). Builds C:\Eval.
$ErrorActionPreference = 'Stop'
$steam = 'C:\Program Files (x86)\Steam\steamapps\common\RimWorld'
New-Item -ItemType Directory -Force C:\Eval\lib, C:\Eval\RimWorld\Mods, C:\Eval\tools, C:\Eval\work, C:\Eval\submit, C:\Eval\out, C:\Eval\pkg | Out-Null
& tar -xf C:\Users\Public\evallib.tar -C C:\Eval\lib
foreach ($i in Get-ChildItem $steam -Force) {
  if ($i.Name -in 'Mods', 'steamapps') { continue }
  $dest = Join-Path C:\Eval\RimWorld $i.Name
  if (Test-Path $dest) { continue }
  if ($i.PSIsContainer) { New-Item -ItemType Junction -Path $dest -Target $i.FullName | Out-Null } else { Copy-Item $i.FullName $dest }
}
Copy-Item C:\Users\Public\archo.exe C:\Eval\tools\archo.exe -Force
icacls C:\Eval /grant 'Users:(OI)(CI)M' /T /C /Q | Out-Null
'staged' | Set-Content C:\Eval\staged.txt
