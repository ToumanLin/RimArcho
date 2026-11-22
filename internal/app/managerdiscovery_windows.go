package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
)

func managerProcessFolders() []string {
	var out []string
	for _, p := range listProcesses(managerExe, false) {
		out = append(out, filepath.Dir(p.Exe))
	}
	return out
}

// muiCacheKey is where Windows records the programs this user has run (each value is
// "<exe path>.FriendlyAppName"), so it names a portable manager's folder after one run.
const muiCacheKey = `Software\Classes\Local Settings\Software\Microsoft\Windows\Shell\MuiCache`

// managerRanFolders are the folders of manager executables this user has run, read-only
// from MuiCache (at most 5,000 values), kept only while the exe is still there.
func managerRanFolders() []string {
	k, err := registry.OpenKey(registry.CURRENT_USER, muiCacheKey, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadValueNames(5000)
	var out []string
	for _, n := range names {
		i := strings.Index(strings.ToLower(n), ".exe.")
		if i < 0 {
			continue
		}
		exe := n[:i+len(".exe")]
		if !managerExe(filepath.Base(exe)) {
			continue
		}
		if st, err := os.Stat(exe); err == nil && st.Mode().IsRegular() {
			out = append(out, filepath.Dir(exe))
		}
	}
	return out
}

// Resolve shortcuts named for a manager in the Start menu, on the desktop, on the taskbar
// and among recent items, without opening their targets. A deadline bounds slow COM
// providers and disconnected drives.
func managerShortcutFolders() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	script := `[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
$w=New-Object -ComObject WScript.Shell
$roots=@([Environment]::GetFolderPath('StartMenu'),[Environment]::GetFolderPath('CommonStartMenu'),[Environment]::GetFolderPath('Desktop'),[Environment]::GetFolderPath('CommonDesktopDirectory'),(Join-Path $env:APPDATA 'Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar'),[Environment]::GetFolderPath('Recent'))
$result=@(); foreach($r in $roots){ if(!$r -or !(Test-Path -LiteralPath $r)){continue}; Get-ChildItem -LiteralPath $r -Filter *.lnk -Recurse -ErrorAction SilentlyContinue | Where-Object {$_.BaseName -match 'RimCrow|RimModManager|RimSort'} | Select-Object -First 200 | ForEach-Object {
$t=$w.CreateShortcut($_.FullName).TargetPath; if($t){if([IO.Path]::GetExtension($t) -eq '.exe'){$t=[IO.Path]::GetDirectoryName($t)}; $result+=$t}
}}; ConvertTo-Json -Compress -InputObject @($result)`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	b, err := cmd.Output()
	if err != nil {
		return nil
	}
	var out []string
	json.Unmarshal(b, &out)
	return out
}
