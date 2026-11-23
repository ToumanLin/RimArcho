# Rebuilds the startup hook that makes RimWorld 1.7 test-run logs UTF-8 (docs/specs 20 §2).
# Output: ArchoConsoleUtf8.dll and source.sha256 (both committed, so `go build` needs no .NET SDK;
# TestConsoleHookBuilt fails when StartupHook.cs changed without a rebuild).
#
#   pwsh internal/gametest/consolehook/build.ps1
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
$out = Join-Path $here "obj-out"
dotnet build (Join-Path $here "ArchoConsoleUtf8.csproj") -c Release -o $out --nologo -v q
if ($LASTEXITCODE -ne 0) { throw "dotnet build failed" }
Copy-Item (Join-Path $out "ArchoConsoleUtf8.dll") $here -Force
# Hash with LF line endings, as the Go test does, whatever the checkout's line endings.
$src = [System.IO.File]::ReadAllText((Join-Path $here "StartupHook.cs")).Replace("`r`n", "`n")
$bytes = [System.Security.Cryptography.SHA256]::HashData([System.Text.Encoding]::UTF8.GetBytes($src))
$hash = [System.Convert]::ToHexString($bytes).ToLowerInvariant()
[System.IO.File]::WriteAllText((Join-Path $here "source.sha256"), "$hash`n")
Remove-Item -Recurse -Force $out, (Join-Path $here "obj"), (Join-Path $here "bin") -ErrorAction SilentlyContinue
