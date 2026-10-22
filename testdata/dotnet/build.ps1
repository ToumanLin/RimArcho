# Rebuilds the fixture assemblies used by internal/harmony and internal/app tests.
# Output: testdata/dotnet/bin/*.dll (committed, so tests need no .NET SDK).
$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
New-Item -ItemType Directory -Force "$here/bin" | Out-Null
foreach ($p in "ModA","ModB","ModC","ModManual","ModManualIL","ModWrapperUser","LogRefs","LogLoader") {
  dotnet build "$here/$p/$p.csproj" -c Release -o "$here/obj-out/$p" --nologo -v q | Out-Null
  Copy-Item "$here/obj-out/$p/$p.dll" "$here/bin/" -Force
}
Copy-Item "$here/obj-out/ModA/0Harmony.dll" "$here/bin/" -Force
Copy-Item "$here/obj-out/ModA/Assembly-CSharp.dll" "$here/bin/" -Force
Remove-Item -Recurse -Force "$here/obj-out", "$here/*/obj", "$here/*/bin" -ErrorAction SilentlyContinue
