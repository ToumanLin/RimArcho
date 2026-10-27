# Launch a transferred script through the already signed-in test VM desktop.
# File transfer still uses vm.ps1. No guest login or credentials are changed.
param([Parameter(Mandatory)][string]$GuestScript)
$ErrorActionPreference='Stop'
if ($GuestScript -notmatch '^C:\\Users\\Public\\Documents\\VM-Test\\[a-zA-Z0-9_-]+\.ps1$') { throw 'Desktop launch requires an owned script in VM-Test' }
$window=Get-Process vmware | Where-Object MainWindowTitle -eq 'Win11-Player-Test - VMware Workstation' | Select-Object -First 1
if (-not $window) { throw 'Open the released Win11-Player-Test desktop in VMware first' }
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class PlayerDesktopInput {
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h,int n);
 [DllImport("user32.dll")] public static extern void keybd_event(byte vk,byte scan,uint flags,UIntPtr extra);
}
"@
[PlayerDesktopInput]::ShowWindow($window.MainWindowHandle,9)|Out-Null
$shell=New-Object -ComObject WScript.Shell
$shell.AppActivate($window.Id)|Out-Null
[PlayerDesktopInput]::keybd_event(0x12,0,0,[UIntPtr]::Zero)
try{[PlayerDesktopInput]::SetForegroundWindow($window.MainWindowHandle)|Out-Null}finally{[PlayerDesktopInput]::keybd_event(0x12,0,2,[UIntPtr]::Zero)}
Start-Sleep -Milliseconds 300
if ([PlayerDesktopInput]::GetForegroundWindow() -ne $window.MainWindowHandle) { throw 'Test VM window could not take keyboard focus' }
$shell.SendKeys('^g')
Start-Sleep -Milliseconds 300
[PlayerDesktopInput]::keybd_event(0x5b,0,0,[UIntPtr]::Zero)
[PlayerDesktopInput]::keybd_event(0x52,0,0,[UIntPtr]::Zero)
[PlayerDesktopInput]::keybd_event(0x52,0,2,[UIntPtr]::Zero)
[PlayerDesktopInput]::keybd_event(0x5b,0,2,[UIntPtr]::Zero)
Start-Sleep -Milliseconds 500
$command='powershell.exe -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "'+$GuestScript+'"'
$shell.SendKeys($command)
$shell.SendKeys('{ENTER}')
Start-Sleep -Milliseconds 300
foreach($vk in @(0x11,0x12)){[PlayerDesktopInput]::keybd_event($vk,0,0,[UIntPtr]::Zero)}
foreach($vk in @(0x12,0x11)){[PlayerDesktopInput]::keybd_event($vk,0,2,[UIntPtr]::Zero)}
