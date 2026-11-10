//go:build windows

package secrets

import (
	"errors"
	"syscall"
	"unsafe"
)

// Windows keeps secrets in the Credential Manager. omega stores each key as
// a generic credential of the current user, through the same three calls the
// system's own tools use (advapi32: CredWriteW, CredReadW, CredDeleteW). The
// secret is protected by the user's login, like the macOS Keychain.

var (
	advapi32       = syscall.NewLazyDLL("advapi32.dll")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errorNotFound           = syscall.Errno(1168) // ERROR_NOT_FOUND
)

// credential mirrors CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func credTarget(provider string) string { return service + ":" + provider }

// credAvailable reports whether the Credential Manager can be used.
func credAvailable() bool { return procCredWrite.Find() == nil }

func credWrite(provider, key string) error {
	target, err := syscall.UTF16PtrFromString(credTarget(provider))
	if err != nil {
		return err
	}
	user, _ := syscall.UTF16PtrFromString(provider)
	comment, _ := syscall.UTF16PtrFromString("omega (" + provider + ")")
	blob := []byte(key)
	if len(blob) == 0 {
		return errors.New("empty key")
	}
	cred := credential{
		Type:               credTypeGeneric,
		TargetName:         target,
		Comment:            comment,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
		UserName:           user,
	}
	if r, _, e := procCredWrite.Call(uintptr(unsafe.Pointer(&cred)), 0); r == 0 {
		return e
	}
	return nil
}

// credRead returns the stored key, or "" when there is none.
func credRead(provider string) (string, error) {
	target, err := syscall.UTF16PtrFromString(credTarget(provider))
	if err != nil {
		return "", err
	}
	var p *credential
	if r, _, e := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p))); r == 0 {
		if errors.Is(e, errorNotFound) {
			return "", nil
		}
		return "", e
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(p)))
	if p.CredentialBlobSize == 0 || p.CredentialBlob == nil {
		return "", nil
	}
	return string(unsafe.Slice(p.CredentialBlob, p.CredentialBlobSize)), nil
}

// credDelete removes the stored key. found is false when there was none.
func credDelete(provider string) (found bool, err error) {
	target, err := syscall.UTF16PtrFromString(credTarget(provider))
	if err != nil {
		return false, err
	}
	if r, _, e := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0); r == 0 {
		if errors.Is(e, errorNotFound) {
			return false, nil
		}
		return false, e
	}
	return true, nil
}
