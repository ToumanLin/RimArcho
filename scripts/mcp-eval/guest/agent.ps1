# Runs Pi once, as the standard user. param: group (A = Archo MCP, B = none), minutes
param([string]$Group, [int]$Minutes = 15)
$env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
$env:DOTNET_CLI_TELEMETRY_OPTOUT = '1'; $env:DOTNET_NOLOGO = '1'
$ud = "$env:USERPROFILE\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios"
(Get-Content C:\Eval\task.md -Raw).Replace('{USERDATA}', $ud) | Set-Content C:\Eval\work\TASK.md -Encoding UTF8
$pi = "$env:USERPROFILE\.pi\agent"
$extra = ''
if ($Group -eq 'A') {
  @{mcpServers = @{archo = @{command = 'C:\Eval\tools\archo.exe'; args = @('mcp'); exposure = 'direct'}}} | ConvertTo-Json -Depth 5 | Set-Content "$pi\mcp.json"
} else {
  Remove-Item "$pi\mcp.json" -EA 0; $extra = ' --no-mcp'
}
$line = "pi -p @C:\Eval\work\TASK.md ""Do the task described in the attached file."" --mode json --no-session --no-context-files --no-skills --no-prompt-templates --model magpie/apikeyfan/deepseek-flash$extra > C:\Eval\out\transcript.jsonl 2> C:\Eval\out\stderr.txt"
$sw = [Diagnostics.Stopwatch]::StartNew()
$p = Start-Process cmd -ArgumentList '/c', $line -WorkingDirectory C:\Eval\work -WindowStyle Hidden -PassThru
$done = $p.WaitForExit($Minutes * 60 * 1000)
if (-not $done) { & taskkill /PID $p.Id /T /F | Out-Null }
@{group = $Group; finished = $done; seconds = [int]$sw.Elapsed.TotalSeconds} | ConvertTo-Json | Set-Content C:\Eval\out\run.json
