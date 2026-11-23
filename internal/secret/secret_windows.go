package secret

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW = advapi32.NewProc("CredWriteW")
	procCredReadW  = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2 // this user on this computer; not roamed
	errorNotFound           = windows.Errno(1168)
	maxBlob                 = 2560 // CRED_MAX_CREDENTIAL_BLOB_SIZE (5 * 512)
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func set(target, value string) error {
	if len(value) > maxBlob {
		return errors.New("secret too long")
	}
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	user, _ := windows.UTF16PtrFromString("RimArcho")
	blob := []byte(value)
	c := credential{Type: credTypeGeneric, TargetName: name, Persist: credPersistLocalMachine, UserName: user, CredentialBlobSize: uint32(len(blob))}
	if len(blob) > 0 {
		c.CredentialBlob = &blob[0]
	}
	r, _, e := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0)
	if r == 0 {
		return e
	}
	return nil
}

func get(target string) (string, error) {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return "", err
	}
	var p *credential
	r, _, e := procCredReadW.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		if errors.Is(e, errorNotFound) {
			return "", ErrNotFound
		}
		return "", e
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(p)))
	if p.CredentialBlobSize == 0 || p.CredentialBlob == nil {
		return "", nil
	}
	return string(unsafe.Slice(p.CredentialBlob, p.CredentialBlobSize)), nil
}

func del(target string) error {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	r, _, e := procCredDelete.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0)
	if r == 0 && !errors.Is(e, errorNotFound) {
		return e
	}
	return nil
}
