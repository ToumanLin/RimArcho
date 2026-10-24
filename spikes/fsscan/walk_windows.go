//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var findFirstFileEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("FindFirstFileExW")

func isReparse(info os.FileInfo) bool {
	if a, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return a.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	}
	return info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}

// Extended paths also allow the native modes to enumerate trees beyond MAX_PATH.
func nativePath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + path
}

func directoryHandle(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(nativePath(path))
	if err != nil {
		return windows.InvalidHandle, err
	}
	// OPEN_REPARSE_POINT keeps a directory replaced with a junction from being followed.
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return h, fmt.Errorf("open directory %s: %w", path, err)
	}
	return h, nil
}
func openDirectory(path string) (*os.File, error) {
	h, err := directoryHandle(path)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if f == nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("os.NewFile failed for %s", path)
	}
	return f, nil
}

// Arithmetic intentionally wraps, matching uint64(info.ModTime().UnixNano()).
func unixNano(ft uint64) uint64 { return (ft - 116444736000000000) * 100 }

func nativeWalk(root string, big bool) totals {
	var t totals
	stack := []string{root}
	// One reusable, 8-byte-aligned 256 KiB buffer per active mod job.
	var buffer []uint64
	if big {
		buffer = make([]uint64, 256*1024/8)
	}
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		visit := func(name string, attrs uint32, size, mtime uint64) {
			if name == "." || name == ".." || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
				return
			}
			if attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
				if !skipDir(name) {
					stack = append(stack, filepath.Join(dir, name))
				}
			} else if attrs&windows.FILE_ATTRIBUTE_DEVICE == 0 {
				t.file(size, unixNano(mtime))
			}
		}
		var err error
		if big {
			err = enumerateBig(dir, buffer, visit)
		} else {
			err = enumerateFind(dir, visit)
		}
		if err == nil {
			t.dirs++
		} else {
			t.fail(fmt.Errorf("enumerate %s: %w", dir, err))
		}
	}
	return t
}

type entryVisitor func(string, uint32, uint64, uint64)

func enumerateFind(dir string, visit entryVisitor) error {
	pattern, err := windows.UTF16PtrFromString(nativePath(filepath.Join(dir, "*")))
	if err != nil {
		return err
	}
	var data windows.Win32finddata
	// FindExInfoBasic=1, FindExSearchNameMatch=0, FIND_FIRST_EX_LARGE_FETCH=2.
	raw, _, callErr := findFirstFileEx.Call(uintptr(unsafe.Pointer(pattern)), 1, uintptr(unsafe.Pointer(&data)), 0, 0, 2)
	h := windows.Handle(raw)
	if h == windows.InvalidHandle {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return callErr
	}
	defer windows.FindClose(h)
	for {
		visit(windows.UTF16ToString(data.FileName[:]), data.FileAttributes,
			uint64(data.FileSizeHigh)<<32|uint64(data.FileSizeLow),
			uint64(data.LastWriteTime.HighDateTime)<<32|uint64(data.LastWriteTime.LowDateTime))
		err := windows.FindNextFile(h, &data)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func enumerateBig(dir string, storage []uint64, visit entryVisitor) error {
	h, err := directoryHandle(dir)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), len(storage)*8)
	for {
		err := windows.GetFileInformationByHandleEx(h, windows.FileFullDirectoryInfo, &buf[0], uint32(len(buf)))
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return nil
		}
		if err != nil {
			return err
		}
		// FILE_FULL_DIR_INFO has a 68-byte header, followed by FileNameLength
		// bytes of UTF-16. NextEntryOffset is zero on the last record in a batch.
		for offset := 0; ; {
			if offset > len(buf)-68 {
				return fmt.Errorf("invalid FILE_FULL_DIR_INFO header at %d", offset)
			}
			record := buf[offset:]
			next := int(binary.LittleEndian.Uint32(record[0:4]))
			nameLen := int(binary.LittleEndian.Uint32(record[60:64]))
			if nameLen%2 != 0 || nameLen > len(record)-68 {
				return fmt.Errorf("invalid directory filename length %d", nameLen)
			}
			name := make([]uint16, nameLen/2)
			for i := range name {
				name[i] = binary.LittleEndian.Uint16(record[68+2*i:])
			}
			visit(string(utf16.Decode(name)), binary.LittleEndian.Uint32(record[56:60]),
				binary.LittleEndian.Uint64(record[40:48]), binary.LittleEndian.Uint64(record[24:32]))
			if next == 0 {
				break
			}
			if next%8 != 0 || next < 68+nameLen || next > len(record)-68 {
				return fmt.Errorf("invalid NextEntryOffset %d", next)
			}
			offset += next
		}
	}
}

func queryUSN(mods []string) {
	volumes := []string{}
	seen := map[string]bool{}
	for _, mod := range mods {
		vol := strings.ToUpper(filepath.VolumeName(mod))
		if len(vol) == 2 && vol[1] == ':' && !seen[vol] {
			volumes = append(volumes, vol)
			seen[vol] = true
		}
	}
	if len(volumes) == 0 {
		volumes = []string{"D:"}
	}
	for _, vol := range volumes {
		path := `\\.\` + vol
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			fmt.Printf("usn %s: %v\n", path, err)
			continue
		}
		h, err := windows.CreateFile(p, windows.GENERIC_READ,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			fmt.Printf("usn %s: open read-only: %v\n", path, err)
			continue
		}
		var buf [80]byte
		var returned uint32
		const fsctlQueryUSNJournal = 0x000900f4
		err = windows.DeviceIoControl(h, fsctlQueryUSNJournal, nil, 0, &buf[0], uint32(len(buf)), &returned, nil)
		windows.CloseHandle(h)
		if err != nil {
			fmt.Printf("usn %s: FSCTL_QUERY_USN_JOURNAL: %v\n", path, err)
			continue
		}
		if returned < 56 {
			fmt.Printf("usn %s: unexpected response length %d\n", path, returned)
			continue
		}
		fmt.Printf("usn %s: usable; journal_id=%016x first_usn=%d next_usn=%d lowest_valid_usn=%d max_usn=%d maximum_size=%d allocation_delta=%d response_bytes=%d\n",
			path, binary.LittleEndian.Uint64(buf[0:8]), int64(binary.LittleEndian.Uint64(buf[8:16])),
			int64(binary.LittleEndian.Uint64(buf[16:24])), int64(binary.LittleEndian.Uint64(buf[24:32])),
			int64(binary.LittleEndian.Uint64(buf[32:40])), binary.LittleEndian.Uint64(buf[40:48]), binary.LittleEndian.Uint64(buf[48:56]), returned)
	}
}

// Native directory records provide attributes without opening individual files.
func listColdDirectory(dir string, storage []uint64, visit func(string, bool)) error {
	return enumerateBig(dir, storage, func(name string, attrs uint32, _, _ uint64) {
		if name == "." || name == ".." || attrs&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DEVICE) != 0 {
			return
		}
		visit(name, attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0)
	})
}

func openColdFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(nativePath(path))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if f == nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("os.NewFile failed for %s", path)
	}
	return f, nil
}
