# Private local baseline for Phase 1a. Requires PowerShell 7 and sqlite3; no app opens until paths are isolated.
param(
 [Parameter(Mandatory)][string]$Root,
 [string]$SourceHome = (Join-Path $env:APPDATA 'RimArcho'),
 [string]$Tools = (Join-Path ([IO.Path]::GetTempPath()) 'RimArcho-vm-tools'),
 [switch]$ResumeCopy
)
$ErrorActionPreference = 'Stop'
$Root = [IO.Path]::GetFullPath($Root)
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$common = (& git -C $repo rev-parse --path-format=absolute --git-common-dir)
if ($LASTEXITCODE) { throw 'Cannot resolve repository root' }
$mainRepo = Split-Path -Parent $common
if ($Root.StartsWith($mainRepo, [StringComparison]::OrdinalIgnoreCase) -or $Root.StartsWith($repo, [StringComparison]::OrdinalIgnoreCase) -or $Root.StartsWith($SourceHome, [StringComparison]::OrdinalIgnoreCase)) { throw 'Root must be outside the repo and source home' }
if ((Test-Path $Root) -and -not $ResumeCopy) { throw 'Root already exists; choose a new directory' }
if ($ResumeCopy -and ((Test-Path (Join-Path $Root 'relocation.sql')) -or -not (Test-Path (Join-Path $Root 'copy-source.json')))) { throw 'ResumeCopy requires an interrupted copy before relocation' }
if ($ResumeCopy -and ((Get-Content -Raw (Join-Path $Root 'copy-source.json') | ConvertFrom-Json).source -ne [IO.Path]::GetFullPath($SourceHome))) { throw 'ResumeCopy source differs' }
if (Test-Path (Join-Path $Root 'isolated.json')) { throw 'Already isolated; do not rebase it twice' }
function SQLQuote([string]$s) { "'" + $s.Replace("'", "''") + "'" }
$db = Join-Path $Root 'home/archo.db'
New-Item -ItemType Directory -Force (Split-Path $db) | Out-Null
if (-not $ResumeCopy) {
 & sqlite3 -readonly (Join-Path $SourceHome 'archo.db') ".backup $(SQLQuote $db)"
 if ($LASTEXITCODE) { throw 'SQLite backup failed' }
 @{source=[IO.Path]::GetFullPath($SourceHome)} | ConvertTo-Json | Set-Content -Encoding utf8 (Join-Path $Root 'copy-source.json')
 Copy-Item -LiteralPath (Join-Path $SourceHome 'config.toml') -Destination (Join-Path $Root 'home/config.toml')
 robocopy (Join-Path $SourceHome 'cache') (Join-Path $Root 'home/cache') /E /XJ /NFL /NDL /NJH /NJS /NP /R:1 /W:1 /MT:16 | Out-Null
 if ($LASTEXITCODE -ge 8) { throw 'cache copy failed' }
}
function Query([string]$q) { $s = & sqlite3 -readonly -json $db $q; if ($LASTEXITCODE) { throw 'SQLite query failed' }; if ($s) { ($s -join "`n") | ConvertFrom-Json } }
$envRows = @(Query 'select id,name,game_root,user_data_root from environment;')
$srcRows = @(Query "select id,root,kind,label from source where kind not in ('official','local');")
$copies = [ordered]@{}
foreach ($e in $envRows) {
 if (-not $copies.Contains($e.game_root)) { $copies[$e.game_root] = Join-Path $Root ('game-' + $e.id) }
 if ($e.user_data_root -and -not $copies.Contains($e.user_data_root)) { $copies[$e.user_data_root] = Join-Path $Root ('userdata-' + $e.id) }
}
foreach ($s in $srcRows) { if (-not $copies.Contains($s.root)) { $copies[$s.root] = Join-Path $Root ('source-' + $s.id) } }
function CopyTree([string]$from,[string]$to) {
 Write-Output "copy $from -> $to"
 robocopy $from $to /E /XJ /NFL /NDL /NJH /NJS /NP /R:1 /W:1 /MT:16 | Out-Null
 if ($LASTEXITCODE -ge 8) { throw "copy failed: $from" }
}
foreach ($from in @($copies.Keys)) { CopyTree $from $copies[$from] }
function Mapped([string]$path) {
 if (-not $path) { return $path }
 foreach ($from in @($copies.Keys | Sort-Object Length -Descending)) {
  if ($path.Equals($from,[StringComparison]::OrdinalIgnoreCase)) { return $copies[$from] }
  if ($path.StartsWith($from.TrimEnd('\','/')+'\',[StringComparison]::OrdinalIgnoreCase)) { return Join-Path $copies[$from] $path.Substring($from.Length).TrimStart('\','/') }
 }
 return $path
}
# Copy external junction targets too; recreate links only to the copied targets.
foreach ($from in @($copies.Keys)) {
 foreach ($link in @(Get-ChildItem -LiteralPath $from -Recurse -Attributes ReparsePoint -ErrorAction Stop)) {
  if (-not $link.PSIsContainer) { throw "File reparse point needs explicit handling: $($link.FullName)" }
  $target = $link.ResolveLinkTarget($true).FullName
  if (-not $target) { throw "Dangling original link: $($link.FullName)" }
  $mappedTarget = Mapped $target
  if ($mappedTarget -eq $target) {
   $bytes = [Text.Encoding]::UTF8.GetBytes($target.ToLowerInvariant())
   $token = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).Substring(0,16)
   $mappedTarget = Join-Path $Root "foreign/$token"
   CopyTree $target $mappedTarget
   $copies[$target] = $mappedTarget
  }
  $dest = Join-Path $copies[$from] ([IO.Path]::GetRelativePath($from,$link.FullName))
  if (-not (Test-Path -LiteralPath $dest)) { New-Item -ItemType Directory -Force (Split-Path $dest) | Out-Null; New-Item -ItemType Junction -Path $dest -Target $mappedTarget | Out-Null }
 }
}
# Typed fixture relocation in the COPIED DB before App.Open. IDs, lists and history are preserved.
# This avoids product folder-set's incoming-link lifecycle ever seeing writable original roots.
$updates = [Collections.Generic.List[string]]::new()
foreach ($spec in @(
 @('environment','game_root'),@('environment','user_data_root'),@('source','root'),@('source','pending_root'),
 @('instance','real_path'),@('instance_alias','real_path'),@('modsconfig_state','user_data_root'),
 @('managed_link','mods_path'),@('managed_link','target'),@('managed_link','next_path'),@('managed_link','next_target'),@('managed_link','from_path'),@('managed_link','to_path')
)) {
 $table,$column = $spec
 foreach ($row in @(Query "select distinct [$column] as path from [$table] where [$column] <> '';")) {
  $mapped = Mapped $row.path
  # Keep pre-existing case-distinct history keys distinct; product PathKey uses the lowercase row.
  if ($table -eq 'modsconfig_state' -and $row.path -ceq $row.path.ToLowerInvariant()) { $mapped = $mapped.ToLowerInvariant() }
  if ($mapped -ne $row.path) { $updates.Add("update [$table] set [$column]=$(SQLQuote $mapped) where [$column]=$(SQLQuote $row.path);") }
  elseif ([IO.Path]::IsPathRooted($row.path) -and -not $row.path.StartsWith($Root,[StringComparison]::OrdinalIgnoreCase)) { throw "Unmapped $table.${column}: $($row.path)" }
 }
}
foreach ($row in @(Query 'select key,value from meta;')) {
 $mapped = Mapped $row.value
 if ($mapped -ne $row.value) { $updates.Add("update meta set value=$(SQLQuote $mapped) where key=$(SQLQuote $row.key);") }
}
foreach ($key in @('lock.dir','pack.dir','monitor.dir','subscribed.dir')) {
 $dir = Join-Path $Root ($key -replace '\.dir$','')
 # Existing source directories take precedence (lock and monitor normally have an indexed source).
 $kind = switch ($key) { 'lock.dir' {'locked'} 'monitor.dir' {'monitor'} default {''} }
 $s = $srcRows | Where-Object label -eq $kind | Select-Object -First 1
 if ($s) { $dir = Mapped $s.root }
 New-Item -ItemType Directory -Force $dir | Out-Null
 $updates.Add("insert into meta(key,value) values($(SQLQuote "cfg:$key"),$(SQLQuote $dir)) on conflict(key) do update set value=excluded.value;")
}
$sqlFile = Join-Path $Root 'relocation.sql'
[IO.File]::WriteAllText($sqlFile, ".bail on`nbegin immediate;`n" + ($updates -join "`n") + "`ncommit;`n", [Text.UTF8Encoding]::new($false))
Get-Content -Raw $sqlFile | & sqlite3 $db
if ($LASTEXITCODE) { throw 'typed relocation failed' }
$env:RIMARCHO_HOME = Join-Path $Root 'home'
$env:APPDATA = Join-Path $Root 'appdata'
$env:LOCALAPPDATA = Join-Path $Root 'localappdata'
$archo = Join-Path $Tools 'archo.exe'
& $archo config set monitor.default off
& $archo config set ui.lang en
# Only now use product services against the fully isolated copy.
foreach ($e in $envRows) { & $archo env set $e.name --game (Mapped $e.game_root) --user-data (Mapped $e.user_data_root); if ($LASTEXITCODE) { throw 'isolated env validation failed' } }
foreach ($s in $srcRows) {
 $id = switch ($s.label) { 'locked' {'lock'} 'monitor' {'monitor'} default {"source:$($s.id)"} }
 & $archo folder set $id (Mapped $s.root)
 if ($LASTEXITCODE) { throw 'isolated source validation failed' }
}
& $archo scan
if ($LASTEXITCODE) { throw 'isolated scan failed' }
& (Join-Path $Tools 'vmstate.exe') --mode audit --home $env:RIMARCHO_HOME --root $Root
if ($LASTEXITCODE) { throw 'isolation audit failed' }
@{schema=1;root=$Root;home=$env:RIMARCHO_HOME;created=(Get-Date).ToUniversalTime().ToString('o');sourceDatabaseHash=(Get-FileHash (Join-Path $SourceHome 'archo.db')).Hash;copies=$copies} | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 (Join-Path $Root 'isolated.json')
Write-Output "isolated baseline: $Root"


