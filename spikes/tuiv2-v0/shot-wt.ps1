# Screenshot of `archo tui` in a new Windows Terminal window (the player's terminal with
# its default Cascadia Mono: compatibility mode, 06 §1.3). Only that window is closed again.
#   pwsh -File shot-wt.ps1 -DataHome <copy> -Name v1v2-wt-compat [-Cols 160 -Rows 46]
param([Parameter(Mandatory)] [string]$DataHome, [Parameter(Mandatory)] [string]$Name, [int]$Cols = 160, [int]$Rows = 46, [string]$Lang = "zh")
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$wz = Join-Path $root "spikes\wezterm-v0"
$title = "RimArcho-compat-$PID"
$cmd = "title $title & set RIMARCHO_HOME=$DataHome& set ARCHO_LANG=$Lang& `"$wz\bin\archo.exe`" tui"
Start-Process wt.exe -ArgumentList @("-w", "new", "--size", "$Cols,$Rows", "--title", $title, "--suppressApplicationTitle", "cmd.exe", "/c", $cmd)
Start-Sleep -Seconds 6
pwsh -NoProfile -File (Join-Path $PSScriptRoot "shot.ps1") -Title $title -Out (Join-Path $wz "shots\$Name.png") -DelayMs 300
# quit archo (its cmd then exits and the tab closes); then close the window if it is still there
Get-Process archo -ErrorAction SilentlyContinue | Where-Object { $_.Path -like "$wz\bin\*" } | Stop-Process -Force
Start-Sleep -Seconds 1
Add-Type @"
using System; using System.Runtime.InteropServices; using System.Text;
public static class WtClose {
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc f, IntPtr l);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  public static int Close(string title) {
    int n = 0;
    EnumWindows((h, l) => { var sb = new StringBuilder(256); GetWindowText(h, sb, 256);
      if (sb.ToString().Contains(title)) { PostMessage(h, 0x0010, IntPtr.Zero, IntPtr.Zero); n++; } return true; }, IntPtr.Zero);
    return n;
  }
}
"@
"closed windows: $([WtClose]::Close($title))"
