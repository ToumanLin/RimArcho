# Windows PowerShell 5.1: native controls owned exclusively by the runner's PID.
param([Parameter(Mandatory)][int]$AppProcessId, [Parameter(Mandatory)][string]$ExistingFile,
      [Parameter(Mandatory)][string]$Evidence)
$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class NativeSave {
 public delegate bool EnumProc(IntPtr h, IntPtr p);
 [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc f, IntPtr p);
 [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr h, EnumProc f, IntPtr p);
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr h,StringBuilder s,int n);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr h,StringBuilder s,int n);
 [DllImport("user32.dll")] public static extern int GetDlgCtrlID(IntPtr h);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h,uint m,IntPtr w,string l);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h,uint m,IntPtr w,IntPtr l);
}
"@
$trace = [Collections.Generic.List[object]]::new()
function Describe([IntPtr]$h) {
 $name=[Text.StringBuilder]::new(1024);$class=[Text.StringBuilder]::new(256)
 [void][NativeSave]::GetWindowText($h,$name,1024);[void][NativeSave]::GetClassName($h,$class,256)
 @{handle=$h;name=$name.ToString();class=$class.ToString();id=[NativeSave]::GetDlgCtrlID($h)}
}
function Dialogs {
 $found=[Collections.Generic.List[object]]::new()
 [void][NativeSave]::EnumWindows({param($h,$p)
  [uint32]$owner=0;[void][NativeSave]::GetWindowThreadProcessId($h,[ref]$owner)
  if ($owner -eq $AppProcessId -and [NativeSave]::IsWindowVisible($h)) { $d=Describe $h; if($d.class -eq '#32770'){$found.Add($d)} }
  return $true
 },[IntPtr]::Zero)
 $found.ToArray()
}
function Controls($dialog) {
 $found=[Collections.Generic.List[object]]::new()
 [void][NativeSave]::EnumChildWindows($dialog.handle,{param($h,$p) $found.Add((Describe $h));return $true},[IntPtr]::Zero)
 $found.ToArray()
}
function WaitFor([scriptblock]$Read) {
 $end=[DateTime]::UtcNow.AddSeconds(15)
 do { $v=& $Read; if($v){return $v};Start-Sleep -Milliseconds 100 } while([DateTime]::UtcNow -lt $end)
 throw 'Owned native dialog/control not ready within 15s'
}
function Button($dialog,[int[]]$ids) { Controls $dialog | Where-Object {$_.class -eq 'Button' -and $ids -contains $_.id} | Select-Object -First 1 }
function Invoke($control) {
 if(-not $control){throw 'Owned native button not found'}
 $trace.Add(@{action='native-button';name=$control.name;id=$control.id})
 [void][NativeSave]::PostMessage($control.handle,0x00F5,[IntPtr]::Zero,[IntPtr]::Zero)
}
try {
 $dialog=WaitFor {Dialogs | Where-Object {Button $_ @(1)} | Select-Object -First 1}
 $trace.Add(@{action='save-dialog';title=$dialog.name;controls=@(Controls $dialog)})
 $edit=Controls $dialog | Where-Object {$_.class -eq 'Edit' -and $_.id -eq 1001} | Select-Object -First 1
 if(-not $edit){throw 'Native filename edit (1001) not found'}
 [void][NativeSave]::SendMessage($edit.handle,0x000C,[IntPtr]::Zero,[IO.Path]::GetFullPath($ExistingFile))
 Invoke (Button $dialog @(1))
 $no=WaitFor {foreach($d in (Dialogs)) {
  Controls $d | Where-Object {$_.class -eq 'Button' -and $_.name -match '^&?(No|\u5426)(\s|\(|$)'} | Select-Object -First 1
 }}
 Invoke $no
 $trace.Add(@{action='overwrite-declined'})
 $dialog=WaitFor {Dialogs | Where-Object {Button $_ @(2)} | Select-Object -First 1}
 Invoke (Button $dialog @(2))
 $trace.Add(@{action='save-cancelled'})
} catch {$trace.Add(@{error=$_.Exception.Message;dialogs=@(Dialogs | ForEach-Object {@{dialog=$_;controls=@(Controls $_)}})});throw}
finally {$trace | ConvertTo-Json -Depth 8 | Set-Content -Encoding UTF8 -LiteralPath $Evidence}
