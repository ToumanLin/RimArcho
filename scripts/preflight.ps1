# Runs CI's check and linux jobs locally, so a pushed commit does not come back
# with a gofmt, vet, staticcheck or Linux-only failure (17 §5, "Before pushing").
#
#   pwsh scripts/preflight.ps1             # everything (about 10 minutes)
#   pwsh scripts/preflight.ps1 -Quick      # gofmt, vet, staticcheck, Linux vet: no tests
#   pwsh scripts/preflight.ps1 -SkipLinux  # no WSL: Linux is only cross-vetted
#
# Packages are the folders that hold git-tracked .go files, the same set a CI checkout
# has, so ignored scratch folders (tmp/) never fail the run. Linux tests run in WSL
# (-Distro) with its own Go, on the same working tree; a distribution without Go
# fails the run unless -SkipLinux is given. Exit code 0 only when every step passed.
param(
    [switch]$Quick,
    [switch]$SkipLinux,
    [string]$Distro = "Ubuntu-22.04"
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $root

$failed = @()
function Invoke-Step([string]$name, [scriptblock]$body) {
    Write-Host "== $name"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    & $body
    $ok = $LASTEXITCODE -eq 0
    Write-Host ("== {0}: {1} ({2:n0} s)" -f $name, $(if ($ok) { "ok" } else { "FAILED" }), $sw.Elapsed.TotalSeconds)
    if (-not $ok) { $script:failed += $name }
    $global:LASTEXITCODE = 0
}

$goFiles = @(git ls-files '*.go')
$pkgs = @($goFiles | ForEach-Object { "./" + (Split-Path $_ -Parent).Replace('\', '/') } |
    ForEach-Object { if ($_ -eq "./") { "." } else { $_ } } | Sort-Object -Unique)
# CI's linux job leaves out the Windows/cgo GUI and the spikes.
$linuxPkgs = @($pkgs | Where-Object { $_ -notmatch '^\./(internal/surface/gui|spikes)(/|$)' })
$staticcheck = (Select-String -Path .github/workflows/ci.yml -Pattern 'staticcheck@[\w.\-]+' |
    Select-Object -First 1).Matches[0].Value

Invoke-Step "gofmt" {
    $out = @()
    for ($i = 0; $i -lt $goFiles.Count; $i += 200) { $out += @(gofmt -l $goFiles[$i..([Math]::Min($i + 199, $goFiles.Count - 1))]) }
    if ($out.Count) { Write-Host "These files need gofmt:"; $out | ForEach-Object { Write-Host "  $_" }; $global:LASTEXITCODE = 1 }
}
Invoke-Step "go vet" { go vet @pkgs }
Invoke-Step "staticcheck" { go run "honnef.co/go/tools/cmd/$staticcheck" @pkgs }
if (-not $Quick) {
    Invoke-Step "go test" { go test -count=1 -timeout 20m @pkgs }
    Invoke-Step "node --test" { node --test "scripts/vm/*.test.mjs" "feedback-relay/*.test.mjs" }
}

# Linux: cross-vet always; vet, staticcheck and tests in WSL unless skipped.
Invoke-Step "go vet (GOOS=linux, cross)" {
    $env:GOOS = "linux"; $env:CGO_ENABLED = "0"
    try { go vet @linuxPkgs } finally { Remove-Item Env:GOOS, Env:CGO_ENABLED -ErrorAction SilentlyContinue }
}
if (-not $SkipLinux) {
    $wslRoot = "/mnt/" + $root.Substring(0, 1).ToLower() + $root.Substring(2).Replace('\', '/')
    $list = ($linuxPkgs -join ' ')
    $tests = if ($Quick) { "" } else { "&& `$GO test -count=1 -timeout 20m $list" }
    $script = "set -e; GO=`$(command -v go || echo ~/sdk/go/bin/go); [ -x `"`$GO`" ] || { echo 'no Go in $Distro (install it or pass -SkipLinux)'; exit 2; }; " +
        "cd '$wslRoot' && export CGO_ENABLED=0 && `$GO vet $list && `$GO run honnef.co/go/tools/cmd/$staticcheck $list $tests"
    Invoke-Step "Linux in WSL ($Distro)" { wsl -d $Distro -e bash -lc $script }
} else {
    Write-Host "== Linux tests skipped (-SkipLinux): CI's linux job is the only check for them"
}

if ($failed.Count) {
    Write-Host "preflight FAILED: $($failed -join ', ')"
    exit 1
}
Write-Host "preflight ok"
