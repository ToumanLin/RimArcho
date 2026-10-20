package source

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSHFileOperationW = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// shFileOpStruct is SHFILEOPSTRUCTW.
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400
	fofWantNukeWarn   = 0x4000 // fail instead of deleting permanently when there is no recycle bin
)

// Trash moves a file or folder to the recycle bin (10 §2.4). It never deletes permanently:
// where the volume has no recycle bin the shell reports an error instead.
func Trash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	from, err := syscall.UTF16FromString(abs)
	if err != nil {
		return err
	}
	from = append(from, 0) // the list is double-NUL terminated
	op := shFileOpStruct{wFunc: foDelete, pFrom: &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofWantNukeWarn}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return fmt.Errorf("cannot move %s to the recycle bin (shell error 0x%x)", abs, r)
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("moving %s to the recycle bin was cancelled", abs)
	}
	return nil
}
