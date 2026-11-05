package source

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// VolumeFS names the file system of the volume holding dir ("NTFS", "exFAT", "FAT32" …;
// "" when it cannot be read). Directory junctions need NTFS or ReFS (09 §3).
func VolumeFS(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	p, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return ""
	}
	root := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return ""
	}
	name := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeInformation(&root[0], nil, 0, nil, nil, nil, &name[0], uint32(len(name))); err != nil {
		return ""
	}
	return windows.UTF16ToString(name)
}
