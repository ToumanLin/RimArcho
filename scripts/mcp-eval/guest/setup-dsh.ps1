# One-time, as the administrator, before a new eval snapshot: Node 26 (dsh needs >= 24.9; its entry point uses
# import.meta.main) and dsh in C:\Tools\npm next to Pi. Input: C:\Users\Public\node.msi. Log: C:\Users\Public\setup-dsh.log
param([string]$DshVersion = '0.2.1-rc.5')
$ErrorActionPreference = 'Stop'
Start-Transcript C:\Users\Public\setup-dsh.log -Force | Out-Null
try {
  "node before: $(& node --version)"
  if ((& node --version) -notlike 'v26.*') {
    $p = Start-Process msiexec.exe -ArgumentList '/i', 'C:\Users\Public\node.msi', '/qn', '/norestart' -Wait -PassThru
    if ($p.ExitCode -ne 0) { throw "node msi exit $($p.ExitCode)" }
  }
  $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine')
  "node after: $(& node --version)"
  # npm writes warnings to stderr; Windows PowerShell 5 turns those into errors under Stop
  $ErrorActionPreference = 'Continue'
  # npm 11 skips install scripts unless allowed; dsh-subprocess-local's builds the Windows spawn helper
  & npm install -g --prefix C:\Tools\npm "@deepseek-ai/dsh@$DshVersion" '--allow-scripts=@deepseek-ai/dsh-subprocess-local,koffi,node-pty,@google/genai,protobufjs' 2>&1 | ForEach-Object { "$_" } | Select-Object -Last 5
  if ($LASTEXITCODE -ne 0) { throw "npm install exit $LASTEXITCODE" }
  "dsh: $(& C:\Tools\npm\dsh.cmd --version)"
  "pi: $(& C:\Tools\npm\pi.cmd --version 2>&1)"
  icacls C:\Tools\npm /grant 'Users:(OI)(CI)RX' /T /C /Q | Out-Null
  'setup-dsh ok'
} catch { "FAILED: $_" } finally { Stop-Transcript | Out-Null }
