//go:build windows

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// systemFree is the free space of the system drive, in bytes; ok is false
// when it cannot be read.
func systemFree() (free uint64, ok bool) {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	p, err := syscall.UTF16PtrFromString(drive + `\`)
	if err != nil {
		return 0, false
	}
	var avail, total, totalFree uint64
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW").Call(uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
	if r == 0 {
		return 0, false
	}
	return avail, true
}
