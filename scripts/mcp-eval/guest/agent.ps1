# Runs the agent once, as the standard user. param: group, minutes
#   A = Pi + Archo MCP, B = dsh + Archo MCP (since 2026-11-04), N = Pi without MCP (group B before 2026-11-04),
#   P = Pi + Archo MCP + Archo's MCP server instructions in the system prompt, as dsh puts them (Pi 1.4 drops them)
param([string]$Group, [int]$Minutes = 15)
$env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
$env:DOTNET_CLI_TELEMETRY_OPTOUT = '1'; $env:DOTNET_NOLOGO = '1'
$ud = "$env:USERPROFILE\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios"
$task = (Get-Content C:\Eval\task.md -Raw).Replace('{USERDATA}', $ud).Replace('{MINUTES}', "$Minutes")
$task | Set-Content C:\Eval\work\TASK.md -Encoding UTF8
$pi = "$env:USERPROFILE\.pi\agent"
if ($Group -eq 'B') {
  # dsh gets the same task text (no BOM) and the gateway, model and key Pi uses
  $gw = (Get-Content "$pi\models.json" -Raw | ConvertFrom-Json).providers.magpie
  $env:EVAL_GATEWAY_URL = $gw.baseUrl; $env:MAGPIE_GATEWAY_KEY = $gw.apiKey; $env:EVAL_ARCHO = 'C:\Eval\tools\archo.exe'
  $env:DSH_HOME = 'C:\Eval\dsh'   # an ASCII path: the guest user's profile path is not, and reaches node garbled
  $env:DSH_PERMISSION_MODE = 'danger-full-access'; $env:DSH_TELEMETRY_MODE = 'DISABLED'
  [IO.File]::WriteAllText('C:\Eval\work\prompt.txt', "$task`n`nDo the task described above.", (New-Object Text.UTF8Encoding($false)))
  $line = 'dsh --profile headless --patch C:\Eval\dsh-eval.yml --json < C:\Eval\work\prompt.txt > C:\Eval\out\transcript.jsonl 2> C:\Eval\out\stderr.txt'
} else {
  $extra = ''
  if ($Group -eq 'A' -or $Group -eq 'P') {
    @{mcpServers = @{archo = @{command = 'C:\Eval\tools\archo.exe'; args = @('mcp'); exposure = 'direct'}}} | ConvertTo-Json -Depth 5 | Set-Content "$pi\mcp.json"
  } else {
    Remove-Item "$pi\mcp.json" -EA 0; $extra = ' --no-mcp'
  }
  if ($Group -eq 'P') {
    # the instructions this archo build sends at initialize, read from the server itself
    $psi = New-Object Diagnostics.ProcessStartInfo 'C:\Eval\tools\archo.exe', 'mcp'
    $psi.UseShellExecute = $false; $psi.RedirectStandardInput = $true; $psi.RedirectStandardOutput = $true; $psi.RedirectStandardError = $true
    $mp = [Diagnostics.Process]::Start($psi)
    $mp.StandardInput.WriteLine('{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"eval","version":"1"}}}'); $mp.StandardInput.Flush()
    $init = $mp.StandardOutput.ReadLine(); try { $mp.Kill() } catch {}
    $ins = ($init | ConvertFrom-Json).result.instructions
    if (-not $ins) { throw "no instructions from archo mcp: $init" }
    [IO.File]::WriteAllText('C:\Eval\archo-instructions.md', "# Instructions from the MCP server archo`n`n$ins`n", (New-Object Text.UTF8Encoding($false)))
    New-Item -ItemType Directory -Force C:\Eval\out | Out-Null; Copy-Item C:\Eval\archo-instructions.md C:\Eval\out\archo-instructions.md
    $extra = ' --append-system-prompt C:\Eval\archo-instructions.md'
  }
  $line = "pi -p @C:\Eval\work\TASK.md ""Do the task described in the attached file."" --mode json --no-session --no-context-files --no-skills --no-prompt-templates --model magpie/apikeyfan/deepseek-flash$extra > C:\Eval\out\transcript.jsonl 2> C:\Eval\out\stderr.txt"
}
$sw = [Diagnostics.Stopwatch]::StartNew()
$p = Start-Process cmd -ArgumentList '/c', $line -WorkingDirectory C:\Eval\work -WindowStyle Hidden -PassThru
$done = $p.WaitForExit($Minutes * 60 * 1000)
if (-not $done) { & taskkill /PID $p.Id /T /F | Out-Null }
if ($Group -eq 'B') {
  # the full session log (tool results uncut; --json caps them at 8 KiB)
  Get-ChildItem C:\Eval\dsh\sessions -Recurse -Filter 'session.v4.jsonl.zstd' -EA 0 | Sort-Object LastWriteTime | Select-Object -Last 1 | Copy-Item -Destination C:\Eval\out\session.v4.jsonl.zstd
}
@{group = $Group; finished = $done; seconds = [int]$sw.Elapsed.TotalSeconds} | ConvertTo-Json | Set-Content C:\Eval\out\run.json
