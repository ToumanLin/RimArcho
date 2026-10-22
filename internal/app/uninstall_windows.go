package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// documentsDir is the player's Documents folder (where it really is, OneDrive included).
func documentsDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err == nil && p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Documents")
}

// archoProcesses lists the running archo.exe and RimArcho.exe processes.
func archoProcesses() []*GameRun {
	return listProcesses(func(name string) bool { return name == "archo.exe" || name == "rimarcho.exe" }, false)
}

// AfterExitScript is the cmd script that deletes x once process pid has ended. It waits up to
// five minutes, tries each folder ten times (a file the closing window still holds), writes
// what it could not delete to log, then deletes itself.
func AfterExitScript(x AfterExit, pid int, log string) string {
	q := func(p string) string { return `"` + strings.ReplaceAll(p, "%", "%%") + `"` }
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\r\n", a...) }
	w("@echo off")
	w("chcp 65001 >nul")
	w("rem RimArcho uninstall: deletes RimArcho's folders and program once it has closed.")
	w("set n=0")
	w(":wait")
	w(`tasklist /fi "PID eq %d" /nh 2>nul | find " %d " >nul`, pid, pid)
	w("if errorlevel 1 goto gone")
	w("set /a n+=1")
	w("if %%n%% geq 300 goto gone")
	w("ping -n 2 127.0.0.1 >nul")
	w("goto wait")
	w(":gone")
	w("del /f /q %s 2>nul", q(log))
	for _, d := range x.Dirs {
		w(`for /l %%%%i in (1,1,10) do if exist %s (rd /s /q %s 2>nul & if exist %s ping -n 2 127.0.0.1 >nul)`, q(d), q(d), q(d))
	}
	for _, f := range x.Files {
		w(`for /l %%%%i in (1,1,10) do if exist %s (del /f /q %s 2>nul & if exist %s ping -n 2 127.0.0.1 >nul)`, q(f), q(f), q(f))
	}
	for _, d := range x.Empty {
		w("rd %s 2>nul", q(d))
	}
	for _, p := range append(append([]string{}, x.Dirs...), x.Files...) {
		w("if exist %s echo left: %s>>%s", q(p), q(p), q(log))
	}
	w(`(goto) 2>nul & del "%%~f0"`)
	return b.String()
}

// startAfterExit writes the script to the temporary folder and starts it detached and
// without a window, outside any job this process runs in, so it outlives RimArcho.
func startAfterExit(x AfterExit, log string) error {
	f, err := os.CreateTemp("", "RimArcho-uninstall-*.cmd")
	if err != nil {
		return err
	}
	if _, err := f.WriteString(AfterExitScript(x, os.Getpid(), log)); err != nil {
		f.Close()
		return err
	}
	f.Close()
	const (
		newProcessGroup  = 0x00000200
		breakawayFromJob = 0x01000000
		createNoWindow   = 0x08000000
	)
	start := func(flags uint32) error {
		c := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), "/d", "/c", f.Name())
		c.Dir = os.TempDir()
		c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		if err := c.Start(); err != nil {
			return err
		}
		return c.Process.Release()
	}
	if err := start(newProcessGroup | createNoWindow | breakawayFromJob); err != nil {
		// A job that does not allow breaking away: start inside it.
		return start(newProcessGroup | createNoWindow)
	}
	return nil
}
