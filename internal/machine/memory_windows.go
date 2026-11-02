package machine

import (
	"fmt"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	memoryStatus  = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	createFactory = windows.NewLazySystemDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
)

type memoryStatusEx struct {
	Length, Load                                                                     uint32
	TotalPhys, AvailPhys, TotalPage, AvailPage, TotalVirtual, AvailVirtual, Extended uint64
}

type adapterDesc struct {
	Description                             [128]uint16
	Vendor, Device, SubSystem, Revision     uint32
	DedicatedVideo, DedicatedSystem, Shared uintptr
	Luid                                    windows.LUID
	Flags                                   uint32
}

// comObject is a COM interface pointer: its first word is the method table.
type comObject struct{ table *[64]uintptr }

func (o *comObject) call(slot int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.table[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

// Read calls only OS hardware APIs. Dedicated memory uses SIZE_T, not WMI's
// 32-bit AdapterRAM. DXGI software adapters never represent game VRAM.
func Read() Memory {
	m := Memory{SampledAt: time.Now()}
	status := memoryStatusEx{}
	status.Length = uint32(unsafe.Sizeof(status))
	ok, _, err := memoryStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		m.Error = fmt.Sprint(err)
	} else {
		m.TotalRAM = int64(status.TotalPhys)
		m.AvailableRAM = int64(status.AvailPhys)
		m.CommitAvailable = int64(status.AvailPage)
	}
	// Prefer IDXGIFactory6's OS high-performance order. Older Windows falls back
	// to hardware adapters sorted by dedicated capacity, explicitly an assumption.
	iid := windows.GUID{Data1: 0xc1b6694f, Data2: 0xff09, Data3: 0x44a9, Data4: [8]byte{0xb0, 0x3c, 0x77, 0x90, 0x0a, 0x0a, 0x1d, 0x17}}
	var factory *comObject
	if err := createFactory.Find(); err != nil {
		m.Error += " DXGI unavailable"
		return m
	}
	hr, _, _ := createFactory.Call(uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory)))
	highPerformance := int32(hr) >= 0
	if !highPerformance {
		iid = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
		hr, _, _ = createFactory.Call(uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory)))
	}
	if int32(hr) < 0 || factory == nil {
		m.Error += fmt.Sprintf(" DXGI unavailable (0x%x)", uint32(hr))
		return m
	}
	defer factory.call(2)
	adapterIID := windows.GUID{Data1: 0x29038f61, Data2: 0x3839, Data3: 0x4626, Data4: [8]byte{0x91, 0xfd, 0x08, 0x68, 0x79, 0x01, 0x1a, 0x05}}
	for i := uintptr(0); i < 32; i++ {
		var adapter *comObject
		if highPerformance {
			hr = factory.call(29, i, 2, uintptr(unsafe.Pointer(&adapterIID)), uintptr(unsafe.Pointer(&adapter)))
		} else {
			hr = factory.call(12, i, uintptr(unsafe.Pointer(&adapter)))
		}
		if int32(hr) < 0 || adapter == nil {
			break
		}
		var desc adapterDesc
		hr = adapter.call(10, uintptr(unsafe.Pointer(&desc)))
		adapter.call(2)
		if int32(hr) >= 0 && desc.Flags&2 == 0 {
			m.Adapters = append(m.Adapters, Adapter{ID: fmt.Sprintf("%08x:%08x", uint32(desc.Luid.HighPart), desc.Luid.LowPart), Name: windows.UTF16ToString(desc.Description[:]), Dedicated: int64(desc.DedicatedVideo)})
		}
	}
	if !highPerformance {
		sort.SliceStable(m.Adapters, func(i, j int) bool { return m.Adapters[i].Dedicated > m.Adapters[j].Dedicated })
	}
	return m
}
