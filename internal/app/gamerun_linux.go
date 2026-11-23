package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func gameProcesses() []*GameRun { return gameProcessesAt("/proc") }

// gameProcessesAt also accepts a synthetic proc tree. Wine's host executable
// identifies the loader; its argv identifies the game and its installation.
func gameProcessesAt(proc string) []*GameRun {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return nil
	}
	boot := procBoot(proc)
	var out []*GameRun
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || !entry.IsDir() {
			continue
		}
		dir := filepath.Join(proc, entry.Name())
		exe, err := os.Readlink(filepath.Join(dir, "exe"))
		if err != nil {
			continue // Exited, a kernel thread, or inaccessible (as on Windows).
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		name := strings.ToLower(filepath.Base(exe))
		if name != "rimworldlinux" && name != "rimworldwin64.exe" && name != "wine" && name != "wine64" && name != "wine-preloader" && name != "wine64-preloader" {
			continue
		}
		line, lineErr := os.ReadFile(filepath.Join(dir, "cmdline"))
		args := bytes.Split(bytes.TrimRight(line, "\x00"), []byte{0})
		wine := strings.HasPrefix(name, "wine")
		if wine {
			exe = ""
			for _, arg := range args {
				p := strings.ReplaceAll(string(arg), `\`, "/")
				if strings.EqualFold(filepath.Base(p), "RimWorldWin64.exe") {
					exe = winePath(p, dir)
					break
				}
			}
			if exe == "" {
				continue
			}
		}
		r := &GameRun{PID: pid, Exe: exe, ArgsKnown: lineErr == nil && len(line) > 0}
		for _, arg := range args {
			const prefix = "-savedatafolder="
			if len(arg) >= len(prefix) && strings.EqualFold(string(arg[:len(prefix)]), prefix) {
				p := string(arg[len(prefix):])
				if wine {
					p = winePath(strings.ReplaceAll(p, `\`, "/"), dir)
					if p == "" {
						r.ArgsKnown = false
					}
				}
				r.SaveData = p
			}
		}
		r.Started = procStarted(dir, boot)
		out = append(out, r)
	}
	return out
}

// winePath maps Linux argv paths and Wine drive paths back to host paths.
func winePath(p, procDir string) string {
	if len(p) >= 3 && p[1] == ':' && p[2] == '/' {
		if p[0] == 'z' || p[0] == 'Z' {
			return filepath.Clean(p[2:])
		}
		environ, _ := os.ReadFile(filepath.Join(procDir, "environ"))
		for _, entry := range bytes.Split(environ, []byte{0}) {
			if prefix, ok := strings.CutPrefix(string(entry), "WINEPREFIX="); ok {
				drive := filepath.Join(prefix, "dosdevices", strings.ToLower(p[:2]))
				if root, err := filepath.EvalSymlinks(drive); err == nil {
					return filepath.Join(root, p[3:])
				}
			}
		}
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if cwd, err := os.Readlink(filepath.Join(procDir, "cwd")); err == nil {
		return filepath.Join(cwd, p)
	}
	return ""
}

// steamClientRunning reports whether the Steam client runs (09 §2 "Without Steam").
func steamClientRunning() bool { return steamRunningAt("/proc") }

// steamRunningAt also accepts a synthetic proc tree: a process whose executable is named
// steam (native, Flatpak and the steam.sh-started client alike).
func steamRunningAt(proc string) bool {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return true // unknown: keep launching through Steam
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		exe, err := os.Readlink(filepath.Join(proc, entry.Name(), "exe"))
		if err != nil {
			continue
		}
		if filepath.Base(strings.TrimSuffix(exe, " (deleted)")) == "steam" {
			return true
		}
	}
	return false
}
