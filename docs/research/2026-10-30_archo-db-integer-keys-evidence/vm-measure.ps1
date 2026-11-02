# archo.db v27 / prototype measurement on the VM's real library. Works only on copies in
# $w; the real home is read once through sqlite3 -readonly .backup.
# Continue: in Windows PowerShell 5.1 a native program's stderr line is an error record, and
# Stop would end the script on archo's progress output. Exit codes are checked instead.
$ErrorActionPreference = 'Continue'
$w = 'C:\Users\Public\Documents\VM-Test\dbint'
$sq = 'C:\Users\Public\Documents\VM-Test\phase1a\node\sqlite3.exe'
$real = 'C:\Users\Player\AppData\Roaming\RimArcho'
$mc = 'C:\Users\Player\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios\Config\ModsConfig.xml'
$log = Join-Path $w 'measure.txt'
function L([string]$s) { Add-Content -Encoding UTF8 -Path $log -Value $s; $s }
function Size($p) { $n = 0; foreach ($f in Get-ChildItem "$p*") { $n += $f.Length }; [math]::Round($n / 1MB, 2) }
if (Get-Process -Name RimArcho, archo, RimWorldWin64 -ErrorAction SilentlyContinue) { L 'RimArcho or the game is running'; exit 1 }
foreach ($d in 'homeA', 'homeB', 'proto', 'env', 'tmp', 'measure.txt') { Remove-Item -Recurse -Force (Join-Path $w $d) -ErrorAction SilentlyContinue }
L "start $(Get-Date -Format o)"
$dbBefore = (Get-FileHash -Algorithm MD5 (Join-Path $real 'archo.db')).Hash
$mcBefore = if (Test-Path $mc) { (Get-FileHash -Algorithm MD5 $mc).Hash } else { 'none' }
foreach ($d in 'homeA', 'homeB', 'proto', 'env\appdata', 'env\local', 'tmp') { New-Item -ItemType Directory -Force (Join-Path $w $d) | Out-Null }
$wf = $w.Replace([string][char]92, '/')
& $sq -readonly (Join-Path $real 'archo.db') ".backup '$wf/homeA/archo.db'"
if ($LASTEXITCODE -or -not (Test-Path (Join-Path $w 'homeA\archo.db'))) { L "backup failed ($LASTEXITCODE)"; exit 2 }
Copy-Item (Join-Path $real 'config.toml'), (Join-Path $real 'tasks.json') (Join-Path $w 'homeA')
$schema0 = & $sq -readonly (Join-Path $w 'homeA\archo.db') "SELECT value FROM meta WHERE key='schema_version'"
L "copy: $(Size (Join-Path $w 'homeA\archo.db')) MB, schema $schema0"
$env:APPDATA = Join-Path $w 'env\appdata'; $env:LOCALAPPDATA = Join-Path $w 'env\local'
$env:TEMP = Join-Path $w 'tmp'; $env:TMP = $env:TEMP
function Timed($exe, $h, [string[]]$a) {
    $env:RIMARCHO_HOME = $h
    $t = Measure-Command { & $exe @a *> $null }
    if ($LASTEXITCODE) { L "$exe $a exit $LASTEXITCODE"; exit 3 }
    [math]::Round($t.TotalMilliseconds)
}
foreach ($n in 'v26', 'v27') {
    $h = Join-Path $w $(if ($n -eq 'v26') { 'homeA' } else { 'homeB' })
    $exe = Join-Path $w "bin\archo-$n.exe"
    if ($n -eq 'v27') { Copy-Item (Join-Path $w 'homeA\*') $h -Force }
    $db = Join-Path $h 'archo.db'
    $before = Size $db
    $open = Timed $exe $h @('db', 'status')
    $schema = & $sq -readonly $db "SELECT value FROM meta WHERE key='schema_version'"
    L "${n}: first open (upgrade) $open ms; db $before -> $(Size $db) MB (with WAL), schema $schema"
    $ver = @(1..3 | ForEach-Object { Timed $exe $h @('version') })
    $lib = @(1..5 | ForEach-Object { Timed $exe $h @('library') })
    L "${n}: archo version ms $($ver -join ' '); archo library ms $($lib -join ' ')"
    $env:RIMARCHO_HOME = $h
    & $exe library 2>&1 | ForEach-Object { "$_" } | Out-File -Encoding UTF8 (Join-Path $w "library-$n.txt")
    & $exe db status 2>&1 | ForEach-Object { "$_" } | Out-File -Encoding UTF8 (Join-Path $w "dbstatus-$n.txt")
}
$d = Compare-Object (Get-Content (Join-Path $w 'library-v26.txt')) (Get-Content (Join-Path $w 'library-v27.txt'))
L "library output v26 vs v27: $(@($d).Count) differing lines of $((Get-Content (Join-Path $w 'library-v27.txt')).Count)"
Copy-Item (Join-Path $w 'homeB\archo.db*') (Join-Path $w 'proto')
$env:RIMARCHO_HOME = ''
$env:RIMARCHO_INTKEY_DB = Join-Path $w 'proto\archo.db'
& (Join-Path $w 'bin\store.test.exe') '-test.run' '^TestIntKeyConvertFile$' '-test.v' '-test.count=1' 2>&1 | ForEach-Object { "$_" } | Out-File -Encoding UTF8 (Join-Path $w 'convert.txt')
L "convert exit $LASTEXITCODE; proto $(Size (Join-Path $w 'proto\archo.db')) MB"
$dbAfter = (Get-FileHash -Algorithm MD5 (Join-Path $real 'archo.db')).Hash
$mcAfter = if (Test-Path $mc) { (Get-FileHash -Algorithm MD5 $mc).Hash } else { 'none' }
L "real archo.db unchanged: $($dbBefore -eq $dbAfter); ModsConfig unchanged: $($mcBefore -eq $mcAfter)"
L "end $(Get-Date -Format o)"
