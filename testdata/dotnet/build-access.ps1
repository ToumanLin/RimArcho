# Rebuilds the member access fixtures used by internal/dotnet and internal/app tests (05 §5.6).
# Output: testdata/dotnet/bin/access (committed, so tests need no .NET SDK).
#   Assembly-CSharp.dll  AccessGame: the stand-in game (a protected field, an internal method)
#   plain/AccessMod.dll  built against the publicized copy, as csc builds it
#   unsafe/AccessMod.dll the same with AllowUnsafeBlocks (skips verification, as Krafs.Publicizer builds it)
#   sub/AccessMod.dll    only a subclass reading the protected field
$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$out = "$here/bin/access"
New-Item -ItemType Directory -Force "$out/plain", "$out/unsafe", "$out/sub" | Out-Null
dotnet build "$here/AccessGame/AccessGame.csproj" -c Release -o "$here/obj-out/game" --nologo -v q | Out-Null
Copy-Item "$here/obj-out/game/Assembly-CSharp.dll" "$out/" -Force
Remove-Item -Recurse -Force "$here/AccessGame/obj" -ErrorAction SilentlyContinue
dotnet build "$here/AccessGame/AccessGame.csproj" -c Release -o "$here/obj-out/pub" -p:DefineConstants=PUB --nologo -v q | Out-Null
$pub = "$here/obj-out/pub"
foreach ($v in @(@{n = 'plain'; p = @()}, @{n = 'unsafe'; p = @('-p:AllowUnsafeBlocks=true')}, @{n = 'sub'; p = @('-p:DefineConstants=SUBONLY')})) {
  Remove-Item -Recurse -Force "$here/AccessMod/obj" -ErrorAction SilentlyContinue
  dotnet build "$here/AccessMod/AccessMod.csproj" -c Release -o "$here/obj-out/$($v.n)" "-p:PubDir=$pub" @($v.p) --nologo -v q | Out-Null
  Copy-Item "$here/obj-out/$($v.n)/AccessMod.dll" "$out/$($v.n)/" -Force
}
Remove-Item -Recurse -Force "$here/obj-out", "$here/Access*/obj", "$here/Access*/bin" -ErrorAction SilentlyContinue
