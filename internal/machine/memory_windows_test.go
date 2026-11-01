package machine

import (
	"testing"
	"unsafe"
)

func TestMemoryNativeLayouts(t *testing.T) {
	if unsafe.Sizeof(memoryStatusEx{}) != 64 {
		t.Fatalf("MEMORYSTATUSEX size %d", unsafe.Sizeof(memoryStatusEx{}))
	}
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(adapterDesc{}) != 312 || unsafe.Offsetof(adapterDesc{}.DedicatedVideo) != 272 || unsafe.Offsetof(adapterDesc{}.Luid) != 296) {
		t.Fatalf("DXGI_ADAPTER_DESC1 layout size %d", unsafe.Sizeof(adapterDesc{}))
	}
	m := Memory{Adapters: []Adapter{{Name: "synthetic discrete", Dedicated: 24 << 30}, {Name: "synthetic integrated", Dedicated: 512 << 20}}}
	if m.VRAM() != 24<<30 {
		t.Fatalf("VRAM truncated or summed: %d", m.VRAM())
	}
}
