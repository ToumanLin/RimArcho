package app

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// logCreationTime is the file's birth time from statx (zero when the file system or
// kernel does not record it; logStart then uses the mtime).
func logCreationTime(path string, _ os.FileInfo) time.Time {
	var st unix.Statx_t
	if unix.Statx(unix.AT_FDCWD, path, 0, unix.STATX_BTIME, &st) != nil || st.Mask&unix.STATX_BTIME == 0 || st.Btime.Sec == 0 {
		return time.Time{}
	}
	return time.Unix(st.Btime.Sec, int64(st.Btime.Nsec))
}
