//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func dataVolume() string {
	if d := os.Getenv("SystemDrive"); d != "" {
		return d + `\`
	}
	return filepath.VolumeName(os.Getenv("USERPROFILE")) + `\`
}

func diskSpace(path string) (total, free uint64, ok bool) {
	k := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var avail, tot, totFree uint64
	r, _, _ := k.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&tot)), uintptr(unsafe.Pointer(&totFree)))
	if r == 0 {
		return 0, 0, false
	}
	return tot, avail, true
}

func hardwareModel() string { return "" }
