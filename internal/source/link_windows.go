package source

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// IsLink reports whether p is a reparse point (junction or symbolic link).
func IsLink(p string) bool {
	fi, err := os.Lstat(p)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	}
	return false
}

// RealPath resolves junctions, symbolic links and 8.3 short names to the final path, using
// GetFinalPathNameByHandle on a directory handle (02 §3.3).
func RealPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	u, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(u, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", &os.PathError{Op: "open", Path: p, Err: err}
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", &os.PathError{Op: "realpath", Path: p, Err: err}
		}
		if int(n) < len(buf) {
			s := windows.UTF16ToString(buf[:n])
			switch {
			case strings.HasPrefix(s, `\\?\UNC\`):
				s = `\\` + s[len(`\\?\UNC\`):]
			case strings.HasPrefix(s, `\\?\`):
				s = s[4:]
			}
			return s, nil
		}
		buf = make([]uint16, n+1)
	}
}

// CreateLink makes link a directory junction pointing at target (09 §8). A junction needs no
// privileges; volumes without reparse points (FAT, exFAT) fail with an error.
func CreateLink(link, target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if err := os.Mkdir(link, 0o755); err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			os.Remove(link)
		}
	}()
	sub := utf16.Encode([]rune(`\??\` + abs))
	prn := utf16.Encode([]rune(abs))
	paths := (len(sub) + 1 + len(prn) + 1) * 2
	buf := make([]byte, 16+paths)
	le := binary.LittleEndian
	le.PutUint32(buf[0:], windows.IO_REPARSE_TAG_MOUNT_POINT)
	le.PutUint16(buf[4:], uint16(8+paths))
	le.PutUint16(buf[8:], 0)                       // substitute name offset
	le.PutUint16(buf[10:], uint16(len(sub)*2))     // substitute name length
	le.PutUint16(buf[12:], uint16((len(sub)+1)*2)) // print name offset
	le.PutUint16(buf[14:], uint16(len(prn)*2))     // print name length
	for i, c := range sub {
		le.PutUint16(buf[16+2*i:], c)
	}
	for i, c := range prn {
		le.PutUint16(buf[16+2*(len(sub)+1+i):], c)
	}
	u, err := windows.UTF16PtrFromString(link)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(u, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: link, Err: err}
	}
	defer windows.CloseHandle(h)
	var n uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &buf[0], uint32(len(buf)), nil, 0, &n, nil); err != nil {
		return &os.PathError{Op: "junction", Path: link, Err: err}
	}
	ok = true
	return nil
}

// RemoveLink deletes a junction or symbolic link itself, never its target. It refuses
// anything that is not a link.
func RemoveLink(link string) error {
	if !IsLink(link) {
		return fmt.Errorf("%s is not a link; refusing to delete it", link)
	}
	return os.Remove(link) // RemoveDirectory on a junction removes only the reparse point
}
