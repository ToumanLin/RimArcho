package app

import (
	"os"
	"syscall"
	"time"
)

func logCreationTime(fi os.FileInfo) time.Time {
	if data, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok && data.CreationTime != (syscall.Filetime{}) {
		return time.Unix(0, data.CreationTime.Nanoseconds())
	}
	return time.Time{}
}
