# Captures the on-screen rectangle of the first visible window of a process into a PNG
# (PrintWindow with PW_RENDERFULLCONTENT: works while the window is covered and does not
# take the focus from whatever the user is doing).
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
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, uint flags);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
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
Start-Sleep -Milliseconds $DelayMs
$r = New-Object W+RECT
[W]::GetWindowRect($found, [ref]$r) | Out-Null
$w = $r.R - $r.L; $h = $r.B - $r.T
$full = New-Object System.Drawing.Bitmap $w, $h
$g = [System.Drawing.Graphics]::FromImage($full)
$hdc = $g.GetHdc()
$ok = [W]::PrintWindow($found, $hdc, 2)
$g.ReleaseHdc($hdc); $g.Dispose()
if (-not $ok) { throw "PrintWindow failed" }
# crop the invisible resize borders: GetWindowRect includes them, the frame bounds do not
$f = New-Object W+RECT
[W]::DwmGetWindowAttribute($found, 9, [ref]$f, 16) | Out-Null   # DWMWA_EXTENDED_FRAME_BOUNDS
$crop = New-Object System.Drawing.Rectangle ($f.L - $r.L), ($f.T - $r.T), ($f.R - $f.L), ($f.B - $f.T)
$bmp = $full.Clone($crop, $full.PixelFormat); $full.Dispose()
$w = $crop.Width; $h = $crop.Height
New-Item -ItemType Directory -Force (Split-Path -Parent $Out) | Out-Null
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
"saved $Out ($w x $h)"
