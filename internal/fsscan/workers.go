package fsscan

import "runtime"

type diskKind uint8

const (
	diskUnknown diskKind = iota
	diskSSD
	diskRotating
)

// Workers chooses listing concurrency for the disk holding path: up to 16
// workers for SSDs, 2 for rotating disks, and up to 8 when the disk is unknown.
func Workers(path string) int {
	switch storageKind(path) {
	case diskRotating:
		return 2
	case diskSSD:
		return min(16, runtime.GOMAXPROCS(0))
	default:
		return min(8, runtime.GOMAXPROCS(0))
	}
}

// DiskType is "ssd" or "hdd" for the disk holding path, "" when Windows does not say (or
// on other systems): the slow-check report's disk (16 §2.14).
func DiskType(path string) string {
	switch storageKind(path) {
	case diskSSD:
		return "ssd"
	case diskRotating:
		return "hdd"
	}
	return ""
}
