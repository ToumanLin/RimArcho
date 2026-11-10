package machine

import (
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

var wineOnce = sync.OnceValue(func() bool {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	return ntdll.NewProc("wine_get_version").Find() == nil
})

// Wine reports whether this Windows program runs under Wine or Proton: only Wine's
// ntdll exports wine_get_version.
func Wine() bool { return wineOnce() }

// TerminalDrawsTUI reports whether stdout is a console that understands the escape
// sequences the TUI draws with. Windows 10 and later turn them on with
// ENABLE_VIRTUAL_TERMINAL_PROCESSING. Wine accepts the flag, but its own console window
// prints the sequences as text ("[K"); a Linux terminal (TERM is set) draws them.
func TerminalDrawsTUI() bool {
	out := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(out, &mode) != nil {
		return true // not a console (a pipe, or a terminal that is not one): not ours to judge
	}
	if windows.SetConsoleMode(out, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
		return false
	}
	if Wine() && os.Getenv("TERM") == "" {
		return false
	}
	return true
}

// OwnConsole reports whether this process is the only one attached to its console, which
// is what a double click on archo.exe gives: the console closes when the process exits.
func OwnConsole() bool {
	ids := make([]uint32, 4)
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	return n == 1
}
