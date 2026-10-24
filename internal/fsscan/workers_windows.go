package fsscan

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

var volumeKinds sync.Map

func storageKind(path string) diskKind {
	path, err := filepath.Abs(path)
	if err != nil {
		return diskUnknown
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return diskUnknown
	}
	var mount [32768]uint16
	if windows.GetVolumePathName(p, &mount[0], uint32(len(mount))) != nil {
		return diskUnknown
	}
	volume := windows.UTF16ToString(mount[:])
	key := strings.ToLower(volume)
	if cached, ok := volumeKinds.Load(key); ok {
		return cached.(diskKind)
	}
	device := ""
	if len(volume) == 3 && volume[1] == ':' {
		device = `\\.\` + volume[:2]
	} else {
		var name [64]uint16
		if windows.GetVolumeNameForVolumeMountPoint(&mount[0], &name[0], uint32(len(name))) == nil {
			device = strings.TrimSuffix(windows.UTF16ToString(name[:]), `\`)
		}
	}
	kind := diskUnknown
	if device != "" {
		kind = querySeekPenalty(device)
	}
	cached, _ := volumeKinds.LoadOrStore(key, kind)
	return cached.(diskKind)
}

func querySeekPenalty(device string) diskKind {
	p, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return diskUnknown
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return diskUnknown
	}
	defer windows.CloseHandle(h)

	const ioctlStorageQueryProperty = 0x002d1400
	var query [12]byte                          // STORAGE_PROPERTY_QUERY, including alignment padding.
	binary.LittleEndian.PutUint32(query[:4], 7) // StorageDeviceSeekPenaltyProperty.
	var descriptor [12]byte
	var returned uint32
	err = windows.DeviceIoControl(h, ioctlStorageQueryProperty, &query[0], uint32(len(query)),
		&descriptor[0], uint32(len(descriptor)), &returned, nil)
	if err != nil || returned < 9 || binary.LittleEndian.Uint32(descriptor[4:8]) < 9 {
		return diskUnknown
	}
	if descriptor[8] != 0 {
		return diskRotating
	}
	return diskSSD
}
