// Package procmem reports the current process's resident memory on Windows.
package procmem

import (
	"syscall"
	"unsafe"
)

type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

var (
	psapi    = syscall.NewLazyDLL("psapi.dll")
	getPMI   = psapi.NewProc("GetProcessMemoryInfo")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	getCur   = kernel32.NewProc("GetCurrentProcess")
)

// MB returns (working set, private bytes, peak working set) in MiB.
func MB() (ws, private, peak float64) {
	h, _, _ := getCur.Call()
	var c processMemoryCounters
	c.cb = uint32(unsafe.Sizeof(c))
	getPMI.Call(h, uintptr(unsafe.Pointer(&c)), uintptr(c.cb))
	return float64(c.WorkingSetSize) / (1 << 20), float64(c.PrivateUsage) / (1 << 20), float64(c.PeakWorkingSetSize) / (1 << 20)
}
