package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func managerProcessFolders() []string {
	var out []string
	for _, p := range listProcesses(func(name string) bool {
		return strings.HasPrefix(name, "rimcrow") || strings.HasPrefix(name, "rimmodmanager") || strings.HasPrefix(name, "rimsort")
	}, false) {
		out = append(out, filepath.Dir(p.Exe))
	}
	return out
}

// Resolve named Start-menu shortcuts and recent folder shortcuts without opening
// their targets. A deadline bounds slow COM providers and disconnected drives.
func managerShortcutFolders() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	script := `[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
$w=New-Object -ComObject WScript.Shell
$roots=@([Environment]::GetFolderPath('StartMenu'),[Environment]::GetFolderPath('CommonStartMenu'),[Environment]::GetFolderPath('Recent'))
$result=@(); foreach($r in $roots){ if(!$r){continue}; Get-ChildItem -LiteralPath $r -Filter *.lnk -Recurse -ErrorAction SilentlyContinue | Where-Object {$_.BaseName -match 'RimCrow|RimModManager|RimSort' -or $r -eq [Environment]::GetFolderPath('Recent')} | Select-Object -First 200 | ForEach-Object {
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
