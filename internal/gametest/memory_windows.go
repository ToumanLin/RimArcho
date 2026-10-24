//go:build windows

package gametest

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var getProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

type processMemoryCounters struct {
	Size, PageFaultCount                               uint32
	PeakWorkingSetSize, WorkingSetSize                 uintptr
	QuotaPeakPagedPoolUsage, QuotaPagedPoolUsage       uintptr
	QuotaPeakNonPagedPoolUsage, QuotaNonPagedPoolUsage uintptr
	PagefileUsage, PeakPagefileUsage                   uintptr
}

func peakMemoryMB(pid int) int64 {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var counters processMemoryCounters
	counters.Size = uint32(unsafe.Sizeof(counters))
	ok, _, _ := getProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	if ok == 0 {
		return 0
	}
	return int64(counters.PeakWorkingSetSize / (1024 * 1024))
}
