package app

import "golang.org/x/sys/windows"

func sourceLockBusy(err error) bool {
	return err == windows.ERROR_SHARING_VIOLATION || err == windows.ERROR_LOCK_VIOLATION
}

func lockSourceLibraryFile(path string, exclusive bool) (func(), error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	share := uint32(windows.FILE_SHARE_READ)
	if exclusive {
		share = 0
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, share, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}
