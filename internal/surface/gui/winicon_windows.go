package gui

import (
	"syscall"

	"github.com/ToumanLin/RimArcho/internal/icon"
	"github.com/ToumanLin/RimArcho/internal/winres"
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	findResource = kernel32.NewProc("FindResourceW")
	getModule    = kernel32.NewProc("GetModuleHandleW")
	getMetrics   = user32.NewProc("GetSystemMetrics")
)

const (
	rtGroupIcon = 14 // RT_GROUP_ICON
	smCxIcon    = 11 // SM_CXICON
)

// windowIcon is the PNG Wails turns into the window's icon when the exe has no icon group 3
// (a development build; release builds carry it, 06 §8.6): nil when the exe has one, else
// the icon drawn at the system's large-icon size (32 px at 100 %), the only size Wails
// uses. Drawing at 256 px cost 43 MB of start-up heap for a 32 px result.
func windowIcon() []byte {
	h, _, _ := getModule.Call(0)
	if r, _, _ := findResource.Call(h, uintptr(winres.IconGroupID), rtGroupIcon); r != 0 {
		return nil
	}
	size, _, _ := getMetrics.Call(smCxIcon)
	if size < 16 || size > 256 {
		size = 32
	}
	return icon.PNG(int(size))
}
