# Per run, as the standard user: build the case's Mods folder, ModsConfig.xml and Player.log. param: case id, group (A|B|N)
# Every run starts from the snapshot, so Mods\ must be empty (never Remove-Item -Recurse over junctions).
param([string]$Case, [string]$Group)
$ErrorActionPreference = 'Stop'
$pkg = "C:\Eval\pkg\$Case"
$ws = 'C:\Program Files (x86)\Steam\steamapps\workshop\content\294100'
$mods = 'C:\Eval\RimWorld\Mods'
if (Get-ChildItem $mods -Force) { throw 'Mods folder is not empty: every run starts from the snapshot' }
$list = Get-Content "$pkg\manifest.json" -Raw | ConvertFrom-Json | ForEach-Object { $_ }   # Windows PowerShell 5 emits the array as one object
foreach ($m in $list) {
  $i = $m.src.IndexOf(":"); $kind = $m.src.Substring(0, $i); $id = $m.src.Substring($i + 1)
  $target = if ($kind -eq 'ws') { Join-Path $ws $id } else { Join-Path C:\Eval\lib $id }
  # every Mod is a junction; overlay.ps1 has already written the faulty files into the target in place
  New-Item -ItemType Junction -Path (Join-Path $mods $m.pkg) -Target $target | Out-Null
}
$ud = "$env:USERPROFILE\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios"
New-Item -ItemType Directory -Force "$ud\Config" | Out-Null
Copy-Item "$pkg\ModsConfig.xml" "$ud\Config\ModsConfig.xml" -Force
Copy-Item "$pkg\Player.log" "$ud\Player.log" -Force
if ($Group -ne 'N') {   # A (Pi) and B (dsh) both get Archo
  & C:\Eval\tools\archo.exe env add main --game C:\Eval\RimWorld --user-data $ud --launch direct 2>&1 | Out-File C:\Eval\setup-archo.log
  & C:\Eval\tools\archo.exe scan 2>&1 | Out-File -Append C:\Eval\setup-archo.log
}
'ready' | Set-Content C:\Eval\case-ready.txt
if ($Group -eq 'N') {
  # group N must not find Archo anywhere on disk
  Remove-Item C:\Eval\tools\archo.exe, C:\Users\Public\archo.exe -Force -EA 0
}
