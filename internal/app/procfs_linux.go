package app

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// procBoot is the boot time in Unix seconds from <proc>/stat (0: unknown).
func procBoot(proc string) int64 {
	stat, _ := os.ReadFile(filepath.Join(proc, "stat"))
	for _, line := range strings.Split(string(stat), "\n") {
		if s, ok := strings.CutPrefix(line, "btime "); ok {
			boot, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			return boot
		}
	}
	return 0
}

// procStat reads a process's state letter and start time in clock ticks after boot from
// <dir>/stat. comm may contain spaces and ')'; the state is field 3 and starttime field 22,
// both after the last ')'.
func procStat(dir string) (state byte, ticks int64, ok bool) {
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return 0, 0, false
	}
	i := strings.LastIndexByte(string(stat), ')')
	if i < 0 {
		return 0, 0, false
	}
	fields := strings.Fields(string(stat)[i+1:])
	if len(fields) < 20 || len(fields[0]) != 1 {
		return 0, 0, false
	}
	ticks, err = strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return fields[0][0], ticks, true
}

// procStarted is when the process in dir started (zero: unknown).
func procStarted(dir string, boot int64) time.Time {
	_, ticks, ok := procStat(dir)
	if !ok || boot <= 0 {
		return time.Time{}
	}
	// Linux exposes USER_HZ=100 on the supported amd64 platform.
	return time.Unix(boot+ticks/100, (ticks%100)*int64(time.Second)/100)
}

// archoProcesses lists the running archo and rimarcho processes.
func archoProcesses() []*GameRun { return archoProcessesAt("/proc") }

// archoProcessesAt also accepts a synthetic proc tree. The executable link names the
// program and its folder; a deleted program (uninstalled while it runs) still counts.
// Processes whose link cannot be read (other users') are not listed, as on Windows.
func archoProcessesAt(proc string) []*GameRun {
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
			continue
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		if name := filepath.Base(exe); name != "archo" && name != "rimarcho" {
			continue
		}
		if state, _, ok := procStat(dir); ok && (state == 'Z' || state == 'X') {
			continue
		}
		out = append(out, &GameRun{PID: pid, Exe: exe, Started: procStarted(dir, boot)})
	}
	return out
}

// processStart is when process pid started, in clock ticks after boot (0: unknown), and
// whether it still runs. Ticks, not wall time: /proc/stat's btime moves with clock
// adjustments, and a task owner compares the value exactly (16 §2.16).
func processStart(pid uint32) (int64, bool) { return processStartAt("/proc", pid) }

func processStartAt(proc string, pid uint32) (int64, bool) {
	if pid == 0 {
		return 0, false
	}
	state, ticks, ok := procStat(filepath.Join(proc, strconv.FormatUint(uint64(pid), 10)))
	if ok {
		return ticks, state != 'Z' && state != 'X'
	}
	if proc != "/proc" {
		return 0, false
	}
	err := unix.Kill(int(pid), 0)
	return 0, err == nil || errors.Is(err, unix.EPERM)
}
