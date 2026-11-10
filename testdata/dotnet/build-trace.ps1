# Rebuilds the null-trace, call-site and assembly-diff fixtures used by internal/dotnet and
# internal/app tests. Output: testdata/dotnet/bin/trace (committed, so tests need no .NET SDK).
#   Assembly-CSharp.dll  TraceGame: a stand-in game (Verse.PlayLogEntry_Interaction, Lord …)
#   TraceA.dll           the caller (passes null), lord AddPawn sites
#   TraceB.dll           the thrower (a postfix on the game constructor)
#   diff/old, diff/new   TraceDiff.dll built without and with V2
$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$out = "$here/bin/trace"
New-Item -ItemType Directory -Force "$out/diff/old", "$out/diff/new" | Out-Null
foreach ($p in "TraceA", "TraceB") {
  dotnet build "$here/$p/$p.csproj" -c Release -o "$here/obj-out/$p" --nologo -v q | Out-Null
  Copy-Item "$here/obj-out/$p/$p.dll" "$out/" -Force
}
Copy-Item "$here/obj-out/TraceA/Assembly-CSharp.dll" "$out/" -Force
dotnet build "$here/TraceDiff/TraceDiff.csproj" -c Release -o "$here/obj-out/diff-old" -p:Version=1.0.0 --nologo -v q | Out-Null
Copy-Item "$here/obj-out/diff-old/TraceDiff.dll" "$out/diff/old/" -Force
Remove-Item -Recurse -Force "$here/TraceDiff/obj" -ErrorAction SilentlyContinue
dotnet build "$here/TraceDiff/TraceDiff.csproj" -c Release -o "$here/obj-out/diff-new" -p:DefineConstants=V2 -p:Version=1.1.0 --nologo -v q | Out-Null
Copy-Item "$here/obj-out/diff-new/TraceDiff.dll" "$out/diff/new/" -Force
Remove-Item -Recurse -Force "$here/obj-out", "$here/Trace*/obj", "$here/Trace*/bin", "$here/FakeHarmony/obj", "$here/FakeHarmony/bin" -ErrorAction SilentlyContinue
