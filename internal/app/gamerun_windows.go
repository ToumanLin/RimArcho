package app

import (
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// gameProcess finds a running RimWorld whose executable is in gameRoot (10 §3.1).
func gameProcess(gameRoot string) *GameRun {
	if gameRoot == "" {
		return nil
	}
	root := env.PathKey(gameRoot)
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		name := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if !strings.HasPrefix(name, "rimworld") {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		var exe string
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
			exe = windows.UTF16ToString(buf[:n])
		}
		var c, x, k, u windows.Filetime
		started := time.Time{}
		if windows.GetProcessTimes(h, &c, &x, &k, &u) == nil {
			started = time.Unix(0, c.Nanoseconds())
		}
		cmdline, known := processCommandLine(h)
		windows.CloseHandle(h)
		if exe != "" && strings.HasPrefix(env.PathKey(filepath.Dir(exe)), root) {
			r := &GameRun{PID: int(pe.ProcessID), Exe: exe, Started: started, ArgsKnown: known}
			if known {
				r.SaveData = SaveDataArg(cmdline)
			}
			return r
		}
	}
	return nil
}

// processCommandLine reads a process's command line (Windows 8.1 and later allow it with
// PROCESS_QUERY_LIMITED_INFORMATION); ok is false when it cannot be read.
func processCommandLine(h windows.Handle) (string, bool) {
	buf := make([]byte, 4096)
	for i := 0; i < 3; i++ {
		var n uint32
		err := windows.NtQueryInformationProcess(h, windows.ProcessCommandLineInformation, unsafe.Pointer(&buf[0]), uint32(len(buf)), &n)
		if err == nil {
			us := (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0]))
			if us.Buffer == nil || us.Length == 0 {
				return "", true
			}
			return windows.UTF16ToString(unsafe.Slice(us.Buffer, us.Length/2)), true
		}
		if n <= uint32(len(buf)) {
			return "", false
		}
		buf = make([]byte, n)
	}
	return "", false
}
