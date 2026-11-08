# In the guest (Windows PowerShell 5), pushed by setup-ilspy-vm.ps1. ASCII only: the host passes it through vmrun.
#   -Mode Install (as the administrator): replace the contents of C:\Tools\ilspycmd (a dotnet tool install, 9.1) with the
#                 single self-contained ilspycmd.exe from C:\Users\Public\ilspycmd.exe, then run Verify.
#   -Mode Verify  (any user; the host runs it as the standard user, in session 0 and on the desktop): ilspycmd --version
#                 through the PATH, then decompile one type of the game's Assembly-CSharp.dll, to stdout and to a file.
# Log: C:\Users\Public\ilspy-<Mode>-<Tag>.log, last line "ilspy <Mode> ok" or "FAILED: <reason>".
param([ValidateSet('Install', 'Verify')][string]$Mode = 'Verify', [string]$Tag = 'admin', [string]$Version = '11.2.0.10050', [string]$Sha256 = '')
# native tools write to stderr and Windows PowerShell 5 turns that into errors under Stop: check exit codes by hand
$ErrorActionPreference = 'Continue'
$log = "C:\Users\Public\ilspy-$Mode-$Tag.log"
$dir = 'C:\Tools\ilspycmd'
$dll = 'C:\Eval\RimWorld\RimWorldWin64_Data\Managed\Assembly-CSharp.dll'
$type = 'Verse.IntVec3'
Start-Transcript $log -Force | Out-Null

function Verify {
  "whoami: $(whoami)"
  $cmd = Get-Command ilspycmd -ErrorAction SilentlyContinue
  if (-not $cmd) { throw 'ilspycmd is not on the PATH' }
  "ilspycmd resolves to: $($cmd.Source) ($((Get-Item $cmd.Source).Length) bytes)"
  if ($cmd.Source -ne "$dir\ilspycmd.exe") { throw "ilspycmd resolves to $($cmd.Source), not $dir\ilspycmd.exe" }
  $ver = (& ilspycmd --version 2>&1 | ForEach-Object { "$_" }) -join ' | '
  "ilspycmd --version: $ver"
  if ($LASTEXITCODE -ne 0 -or $ver -notmatch [regex]::Escape("ilspycmd: $Version")) { throw "wrong version (wanted $Version)" }
  if (-not (Test-Path $dll)) { throw "$dll not found" }
  $t0 = Get-Date
  $src = @(& ilspycmd -t $type $dll 2>&1 | ForEach-Object { "$_" })
  "decompile $type to stdout: exit $LASTEXITCODE, $($src.Count) lines, $([int]((Get-Date) - $t0).TotalMilliseconds) ms"
  if ($LASTEXITCODE -ne 0 -or -not ($src -match 'struct IntVec3')) { throw "decompiling $type failed" }
  $src | Select-Object -First 12 | ForEach-Object { "  | $_" }
  $out = "C:\Users\Public\ilspy-out-$Tag"
  Remove-Item $out -Recurse -Force -ErrorAction SilentlyContinue
  & ilspycmd -t $type -o $out $dll 2>&1 | ForEach-Object { "$_" }
  $cs = @(Get-ChildItem $out -Filter *.cs -Recurse -ErrorAction SilentlyContinue)
  "decompile $type to a file: exit $LASTEXITCODE, $($cs.Count) .cs file(s) $(($cs | ForEach-Object { "$($_.Name) $($_.Length)" }) -join ', ')"
  if ($LASTEXITCODE -ne 0 -or $cs.Count -lt 1) { throw "decompiling $type to a file failed" }
  Remove-Item $out -Recurse -Force -ErrorAction SilentlyContinue
}

try {
  if ($Mode -eq 'Install') {
    $src = 'C:\Users\Public\ilspycmd.exe'
    if (-not (Test-Path $src)) { throw "$src not found" }
    if ($Sha256 -and (Get-FileHash $src -Algorithm SHA256).Hash -ne $Sha256) { throw 'ilspycmd.exe hash differs from the host copy' }
    "before: $((Get-ChildItem $dir -Force -ErrorAction SilentlyContinue | ForEach-Object Name) -join ', ')"
    "before version: $((& "$dir\ilspycmd.exe" --version 2>&1 | ForEach-Object { "$_" }) -join ' | ')"
    "dotnet: $((& dotnet --list-runtimes 2>&1 | ForEach-Object { "$_" }) -join '; ')"
    # keep the folder (the machine PATH names it); drop the apphost shim and .store of the dotnet tool install
    New-Item -ItemType Directory -Force $dir | Out-Null
    Get-ChildItem $dir -Force | Remove-Item -Recurse -Force
    Copy-Item $src "$dir\ilspycmd.exe"
    icacls $dir /grant 'Users:(OI)(CI)RX' /T /C /Q | Out-Null
    $mp = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    if (($mp -split ';') -notcontains $dir) { [Environment]::SetEnvironmentVariable('Path', "$mp;$dir", 'Machine'); 'machine PATH: added ' + $dir }
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    "after: $((Get-ChildItem $dir -Force | ForEach-Object { "$($_.Name) $($_.Length)" }) -join ', ')"
    Verify
    'ilspy Install ok'
  } else {
    Verify
    'ilspy Verify ok'
  }
} catch { "FAILED: $_" } finally { Stop-Transcript | Out-Null }
