package app

import (
	"errors"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SteamCMD's console, read and typed into (11 §3 "Password prompt"). A process without a
// console of its own (the window) attaches to SteamCMD's console for each look; a process
// that shares its console with SteamCMD (the CLI) uses its own. A process that owns a
// console it does not share (the TUI) never detaches from it: AttachConsole fails with
// ERROR_ACCESS_DENIED and RimArcho leaves the prompt to the player.

var (
	kernel32Console        = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole      = kernel32Console.NewProc("AttachConsole")
	procFreeConsole        = kernel32Console.NewProc("FreeConsole")
	procReadConsoleOutput  = kernel32Console.NewProc("ReadConsoleOutputCharacterW")
	procWriteConsoleInputW = kernel32Console.NewProc("WriteConsoleInputW")
)

// consoleMu: attaching is process-wide, one console at a time.
var consoleMu sync.Mutex

// errOwnConsole: this process has a console of its own; it is not given up.
var errOwnConsole = errors.New("this process has its own console")

// inConsole runs f with SteamCMD's console (pid) as this process's console.
func inConsole(pid uint32, shared bool, f func() error) error {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	if shared {
		return f()
	}
	if r, _, err := procAttachConsole.Call(uintptr(pid)); r == 0 {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return errOwnConsole
		}
		return err
	}
	defer procFreeConsole.Call()
	return f()
}

func openConsole(name string) (windows.Handle, error) {
	return windows.CreateFile(windows.StringToUTF16Ptr(name), windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
}

// steamConsoleRead returns the last lines of SteamCMD's screen, the cursor's line last.
func steamConsoleRead(pid uint32, shared bool) (lines []string, cursorY int, err error) {
	err = inConsole(pid, shared, func() error {
		h, err := openConsole("CONOUT$")
		if err != nil {
			return err
		}
		defer windows.CloseHandle(h)
		var info windows.ConsoleScreenBufferInfo
		if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
			return err
		}
		w, y := int(info.Size.X), int(info.CursorPosition.Y)
		first := max(0, y-40)
		n := w*(y-first) + int(info.CursorPosition.X)
		if w <= 0 || n <= 0 {
			cursorY = y
			return nil
		}
		buf := make([]uint16, n)
		var got uint32
		coord := uintptr(uint32(first) << 16) // COORD{X: 0, Y: first}
		if r, _, err := procReadConsoleOutput.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(n), coord, uintptr(unsafe.Pointer(&got))); r == 0 {
			return err
		}
		buf = buf[:got]
		for len(buf) > 0 {
			row := buf[:min(w, len(buf))]
			lines = append(lines, strings.TrimRight(string(utf16.Decode(row)), " \x00"))
			buf = buf[len(row):]
		}
		cursorY = y
		return nil
	})
	return lines, cursorY, err
}

// keyRecord is INPUT_RECORD holding a KEY_EVENT_RECORD.
type keyRecord struct {
	EventType       uint16
	_               uint16
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

// steamConsoleType types s and Enter into SteamCMD's console input, then wipes the copies.
func steamConsoleType(pid uint32, shared bool, s *SteamSecret) error {
	if s.Empty() {
		return nil
	}
	return inConsole(pid, shared, func() error {
		h, err := openConsole("CONIN$")
		if err != nil {
			return err
		}
		defer windows.CloseHandle(h)
		evs := make([]keyRecord, 0, 2*len(s.b)+2)
		defer clear(evs)
		key := func(c, vk uint16) {
			evs = append(evs, keyRecord{EventType: 1, KeyDown: 1, RepeatCount: 1, VirtualKeyCode: vk, UnicodeChar: c},
				keyRecord{EventType: 1, KeyDown: 0, RepeatCount: 1, VirtualKeyCode: vk, UnicodeChar: c})
		}
		for _, c := range s.b {
			key(c, 0)
		}
		key('\r', windows.VK_RETURN)
		var n uint32
		if r, _, err := procWriteConsoleInputW.Call(uintptr(h), uintptr(unsafe.Pointer(&evs[0])), uintptr(len(evs)), uintptr(unsafe.Pointer(&n))); r == 0 {
			return err
		}
		return nil
	})
}

// Pid is the console process's ID (steamPrompts attaches to its console).
func (p *consoleProcess) Pid() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return 0
	}
	id, _ := windows.GetProcessId(p.handle)
	return id
}
