package app

import (
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startAgentConsole opens Archo Agent in a console window of its own. Not
// os/exec: it hands a nil stream NUL with STARTF_USESTDHANDLES, so Pi would
// read NUL and exit at once (see startConsole).
func startAgentConsole(exe string, args []string, dir string, env []string) error {
	application, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{exe}, args...)))
	if err != nil {
		return err
	}
	wdir, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	var block []uint16
	for _, kv := range env {
		if strings.IndexByte(kv, 0) >= 0 {
			continue
		}
		block = append(block, utf16.Encode([]rune(kv))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var info windows.ProcessInformation
	flags := uint32(windows.CREATE_NEW_CONSOLE | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(application, command, nil, nil, false, flags, &block[0], wdir, &startup, &info); err != nil {
		return err
	}
	windows.CloseHandle(info.Thread)
	windows.CloseHandle(info.Process)
	return nil
}
