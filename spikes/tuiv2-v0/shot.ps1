# Captures the on-screen rectangle of the first visible window of a process into a PNG
# (System.Drawing CopyFromScreen; the window is brought to the front first).
#   pwsh spikes/tuiv2-v0/shot.ps1 -ProcessId 1234 -Out shots/x.png
#   pwsh spikes/tuiv2-v0/shot.ps1 -Title "RimArcho" -Out shots/x.png
param([int]$ProcessId = 0, [string]$Title = "", [Parameter(Mandatory = $true)][string]$Out, [int]$DelayMs = 400)
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class W {
  public delegate bool EnumProc(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc f, IntPtr p);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int cmd);
  [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr h, int a, out RECT r, int size);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L, T, R, B; }
}
"@
[W]::SetProcessDPIAware() | Out-Null
$found = [IntPtr]::Zero
$cb = [W+EnumProc]{
  param($h, $p)
  if (-not [W]::IsWindowVisible($h)) { return $true }
  $sb = New-Object System.Text.StringBuilder 512
  [W]::GetWindowText($h, $sb, 512) | Out-Null
  $pid2 = 0; [W]::GetWindowThreadProcessId($h, [ref]$pid2) | Out-Null
  $ok = ($ProcessId -ne 0 -and $pid2 -eq $ProcessId) -or ($Title -ne "" -and $sb.ToString() -like "*$Title*")
  if ($ok -and $sb.Length -gt 0) { $script:found = $h; return $false }
  return $true
}
[W]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
if ($found -eq [IntPtr]::Zero) { throw "window not found" }
[W]::ShowWindow($found, 9) | Out-Null
[W]::SetForegroundWindow($found) | Out-Null
Start-Sleep -Milliseconds $DelayMs
$r = New-Object W+RECT
[W]::DwmGetWindowAttribute($found, 9, [ref]$r, 16) | Out-Null   # DWMWA_EXTENDED_FRAME_BOUNDS
$w = $r.R - $r.L; $h = $r.B - $r.T
$bmp = New-Object System.Drawing.Bitmap $w, $h
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($r.L, $r.T, 0, 0, (New-Object System.Drawing.Size $w, $h))
New-Item -ItemType Directory -Force (Split-Path -Parent $Out) | Out-Null
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()
"saved $Out ($w x $h)"
