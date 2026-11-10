package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func gameProcesses() []*GameRun { return gameProcessesAt("/proc") }

// gameProcessesAt also accepts a synthetic proc tree. Wine's host executable
// identifies the loader; its argv identifies the game and its installation.
func gameProcessesAt(proc string) []*GameRun {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return nil
	}
	var boot int64
	stat, _ := os.ReadFile(filepath.Join(proc, "stat"))
	for _, line := range strings.Split(string(stat), "\n") {
		if s, ok := strings.CutPrefix(line, "btime "); ok {
			boot, _ = strconv.ParseInt(s, 10, 64)
		}
	}
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
		stat, _ := os.ReadFile(filepath.Join(dir, "stat"))
		// comm may contain spaces and ')'; starttime is field 22, after it.
		if i := strings.LastIndexByte(string(stat), ')'); i >= 0 && boot > 0 {
			fields := strings.Fields(string(stat)[i+1:])
			if len(fields) > 19 {
				if ticks, err := strconv.ParseInt(fields[19], 10, 64); err == nil {
					// Linux exposes USER_HZ=100 on the supported amd64 platform.
					r.Started = time.Unix(boot+ticks/100, (ticks%100)*int64(time.Second)/100)
				}
			}
		}
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
